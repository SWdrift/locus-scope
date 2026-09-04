package packages

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"locus-scope/internal/scope"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/content/memory"
	"oras.land/oras-go/v2/errdef"
	"oras.land/oras-go/v2/registry/remote/auth"
)

const reproducibleCreated = "1970-01-01T00:00:00Z"

type PublishOptions struct {
	Credential auth.CredentialFunc
}

type PublishResult struct {
	Target string `json:"target"`
	Digest string `json:"digest"`
}

func Publish(ctx context.Context, rootDirectory, target string, options PublishOptions) (PublishResult, error) {
	reference, err := parsePackageReference(target)
	if err != nil {
		return PublishResult{}, err
	}
	if !reference.Mutable {
		return PublishResult{}, fmt.Errorf("publish target %q must use a tag, not a digest", reference.Canonical)
	}

	root, err := scope.NewLocalSource(rootDirectory)
	if err != nil {
		return PublishResult{}, fmt.Errorf("load package root: %w", err)
	}
	if err := validatePackageSourceTree(root); err != nil {
		return PublishResult{}, err
	}
	layerFile, layer, err := buildPackageLayer(root.LocalPath)
	if err != nil {
		return PublishResult{}, err
	}
	layerPath := layerFile.Name()
	defer func() {
		_ = layerFile.Close()
		_ = os.Remove(layerPath)
	}()

	manifestStore := memory.New()
	manifest, err := oras.PackManifest(ctx, manifestStore, oras.PackManifestVersion1_1, ArtifactType, oras.PackManifestOptions{
		Layers: []ocispec.Descriptor{layer},
		ManifestAnnotations: map[string]string{
			ocispec.AnnotationCreated: reproducibleCreated,
		},
	})
	if err != nil {
		return PublishResult{}, fmt.Errorf("build package manifest: %w", err)
	}

	repository, err := newRemoteRepository(reference, options.Credential)
	if err != nil {
		return PublishResult{}, err
	}
	if err := pushStoredContent(ctx, repository, manifestStore, ocispec.DescriptorEmptyJSON); err != nil {
		return PublishResult{}, fmt.Errorf("push package config: %w", err)
	}
	if _, err := layerFile.Seek(0, io.SeekStart); err != nil {
		return PublishResult{}, fmt.Errorf("rewind package layer: %w", err)
	}
	if err := repository.Push(ctx, layer, layerFile); err != nil && !errors.Is(err, errdef.ErrAlreadyExists) {
		return PublishResult{}, fmt.Errorf("push package layer: %w", err)
	}
	if err := pushStoredContent(ctx, repository, manifestStore, manifest); err != nil {
		return PublishResult{}, fmt.Errorf("push package manifest: %w", err)
	}
	if err := repository.Tag(ctx, manifest, reference.Reference); err != nil {
		return PublishResult{}, fmt.Errorf("tag package %q: %w", reference.Canonical, err)
	}
	return PublishResult{Target: reference.Canonical, Digest: manifest.Digest.String()}, nil
}

func validatePackageSourceTree(root scope.Source) error {
	pending := []scope.Source{root}
	seen := make(map[string]struct{})
	for len(pending) != 0 {
		current := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if _, exists := seen[current.LocalPath]; exists {
			continue
		}
		seen[current.LocalPath] = struct{}{}

		loaded, err := scope.ReadSource(current)
		if err != nil {
			return fmt.Errorf("validate package scope %q: %w", current.LocalPath, err)
		}
		aliases := make([]string, 0, len(loaded.Manifest.Imports))
		for alias := range loaded.Manifest.Imports {
			aliases = append(aliases, alias)
		}
		sort.Strings(aliases)
		for _, alias := range aliases {
			reference := loaded.Manifest.Imports[alias]
			if strings.Contains(reference, "://") {
				if _, err := parsePackageReference(reference); err != nil {
					return fmt.Errorf("package scope %q import %q: %w", current.LocalPath, alias, err)
				}
				continue
			}
			if !portablePackageImport(reference) {
				return fmt.Errorf("package scope %q import %q must use a portable relative path, got %q", current.LocalPath, alias, reference)
			}
			target, err := scope.NewLocalSource(filepath.Join(current.LocalPath, filepath.FromSlash(path.Clean(reference))))
			if err != nil {
				return fmt.Errorf("package scope %q import %q (%q): %w", current.LocalPath, alias, reference, err)
			}
			relative, err := filepath.Rel(root.LocalPath, target.LocalPath)
			if err != nil {
				return fmt.Errorf("check package scope %q import %q (%q): %w", current.LocalPath, alias, reference, err)
			}
			if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
				return fmt.Errorf("package scope %q import %q (%q) escapes package root %q", current.LocalPath, alias, reference, root.LocalPath)
			}
			pending = append(pending, target)
		}
	}
	return nil
}

func portablePackageImport(reference string) bool {
	if reference == "" || strings.Contains(reference, `\`) || path.IsAbs(reference) || filepath.IsAbs(reference) || filepath.VolumeName(reference) != "" {
		return false
	}
	if len(reference) >= 2 && reference[1] == ':' &&
		(reference[0] >= 'A' && reference[0] <= 'Z' || reference[0] >= 'a' && reference[0] <= 'z') {
		return false
	}
	return true
}

type packageArchiveEntry struct {
	path     string
	relative string
	info     fs.FileInfo
}

func buildPackageLayer(root string) (*os.File, ocispec.Descriptor, error) {
	entries, err := collectPackageArchiveEntries(root)
	if err != nil {
		return nil, ocispec.Descriptor{}, err
	}
	file, err := os.CreateTemp(root, ".locus-publish-*.tar.gz")
	if err != nil {
		return nil, ocispec.Descriptor{}, fmt.Errorf("create temporary package layer: %w", err)
	}
	failed := true
	defer func() {
		if failed {
			_ = file.Close()
			_ = os.Remove(file.Name())
		}
	}()

	digester := digest.Canonical.Digester()
	normalizedTime := time.Unix(0, 0).UTC()
	gzipWriter, err := gzip.NewWriterLevel(io.MultiWriter(file, digester.Hash()), gzip.BestCompression)
	if err != nil {
		return nil, ocispec.Descriptor{}, fmt.Errorf("create package gzip stream: %w", err)
	}
	gzipWriter.Header.ModTime = normalizedTime
	gzipWriter.Header.OS = 255
	tarWriter := tar.NewWriter(gzipWriter)
	for _, entry := range entries {
		header := &tar.Header{
			Name:       entry.relative,
			Mode:       int64(entry.info.Mode().Perm()),
			ModTime:    normalizedTime,
			AccessTime: time.Time{},
			ChangeTime: time.Time{},
			Format:     tar.FormatPAX,
		}
		if entry.info.IsDir() {
			header.Name += "/"
			header.Typeflag = tar.TypeDir
		} else {
			header.Typeflag = tar.TypeReg
			header.Size = entry.info.Size()
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			return nil, ocispec.Descriptor{}, fmt.Errorf("write package entry %q: %w", entry.relative, err)
		}
		if !entry.info.Mode().IsRegular() {
			continue
		}
		source, err := os.Open(entry.path)
		if err != nil {
			return nil, ocispec.Descriptor{}, fmt.Errorf("open package entry %q: %w", entry.relative, err)
		}
		_, copyErr := io.CopyN(tarWriter, source, entry.info.Size())
		closeErr := source.Close()
		if copyErr != nil {
			return nil, ocispec.Descriptor{}, fmt.Errorf("read package entry %q: %w", entry.relative, copyErr)
		}
		if closeErr != nil {
			return nil, ocispec.Descriptor{}, fmt.Errorf("close package entry %q: %w", entry.relative, closeErr)
		}
	}
	if err := tarWriter.Close(); err != nil {
		return nil, ocispec.Descriptor{}, fmt.Errorf("finish package tar stream: %w", err)
	}
	if err := gzipWriter.Close(); err != nil {
		return nil, ocispec.Descriptor{}, fmt.Errorf("finish package gzip stream: %w", err)
	}
	info, err := file.Stat()
	if err != nil {
		return nil, ocispec.Descriptor{}, fmt.Errorf("inspect package layer: %w", err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, ocispec.Descriptor{}, fmt.Errorf("rewind package layer: %w", err)
	}
	failed = false
	return file, ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageLayerGzip,
		Digest:    digester.Digest(),
		Size:      info.Size(),
	}, nil
}

func collectPackageArchiveEntries(root string) ([]packageArchiveEntry, error) {
	var entries []packageArchiveEntry
	err := filepath.WalkDir(root, func(filePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if filePath == root {
			return nil
		}
		relative, err := filepath.Rel(root, filePath)
		if err != nil {
			return err
		}
		if entry.Name() == "locus.lock" || entry.Name() == ".locus" {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("package source contains symlink %q", relative)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			return fmt.Errorf("package source entry %q has unsupported mode %s", relative, info.Mode())
		}
		relative = filepath.ToSlash(relative)
		if strings.Contains(relative, `\`) {
			return fmt.Errorf("package source entry %q contains a backslash", relative)
		}
		entries = append(entries, packageArchiveEntry{path: filePath, relative: relative, info: info})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk package source %q: %w", root, err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].relative < entries[j].relative })
	return entries, nil
}

func pushStoredContent(ctx context.Context, repository content.Pusher, store content.Fetcher, descriptor ocispec.Descriptor) error {
	reader, err := store.Fetch(ctx, descriptor)
	if err != nil {
		return err
	}
	pushErr := repository.Push(ctx, descriptor, reader)
	closeErr := reader.Close()
	if pushErr != nil && !errors.Is(pushErr, errdef.ErrAlreadyExists) {
		return pushErr
	}
	return closeErr
}
