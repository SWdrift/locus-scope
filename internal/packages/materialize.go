package packages

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"locus-scope/internal/scope"
	"oras.land/oras-go/v2/content"
)

func materializePackage(ctx context.Context, rootDirectory string, reference packageReference, cached cachedPackage) (string, bool, error) {
	target, err := packageDirectory(rootDirectory, reference)
	if err != nil {
		return "", false, err
	}
	source := scope.Source{Key: scope.ScopeKey(reference.Canonical), LocalPath: target}
	if _, err := os.Stat(target); err == nil {
		if err := scope.CheckSource(source); err != nil {
			return "", false, fmt.Errorf("existing materialized package %q is invalid: %w", reference.Canonical, err)
		}
		return target, false, nil
	} else if !os.IsNotExist(err) {
		return "", false, fmt.Errorf("inspect materialized package %q: %w", reference.Canonical, err)
	}

	packagesRoot := filepath.Dir(target)
	if err := os.MkdirAll(packagesRoot, 0o755); err != nil {
		return "", false, fmt.Errorf("create package materialization root: %w", err)
	}
	temporary, err := os.MkdirTemp(packagesRoot, ".extract-*")
	if err != nil {
		return "", false, fmt.Errorf("create package extraction directory: %w", err)
	}
	defer func() {
		if temporary != "" {
			_ = os.RemoveAll(temporary)
		}
	}()

	if err := extractPackageLayer(ctx, cached.store, cached.layer, temporary); err != nil {
		return "", false, fmt.Errorf("extract package %q: %w", reference.Canonical, err)
	}
	if err := scope.CheckSource(scope.Source{Key: scope.ScopeKey(reference.Canonical), LocalPath: temporary}); err != nil {
		return "", false, fmt.Errorf("validate package %q: %w", reference.Canonical, err)
	}
	if err := os.Rename(temporary, target); err != nil {
		if checkErr := scope.CheckSource(source); checkErr == nil {
			return target, false, nil
		}
		return "", false, fmt.Errorf("publish package %q to %s: %w", reference.Canonical, target, err)
	}
	temporary = ""
	return target, true, nil
}

func packageDirectory(rootDirectory string, reference packageReference) (string, error) {
	if reference.Mutable {
		return "", fmt.Errorf("materialization requires an immutable package reference, got %q", reference.Canonical)
	}
	dgst, err := digest.Parse(reference.Reference)
	if err != nil {
		return "", fmt.Errorf("parse package digest %q: %w", reference.Reference, err)
	}
	name := encodeCacheSegment("", dgst.Algorithm().String()) + "-" + encodeCacheSegment("", dgst.Encoded())
	return filepath.Join(rootDirectory, ".locus", "packages", name), nil
}

func extractPackageLayer(ctx context.Context, fetcher content.Fetcher, descriptor ocispec.Descriptor, destination string) error {
	layerReader, err := fetcher.Fetch(ctx, descriptor)
	if err != nil {
		return fmt.Errorf("fetch layer %s: %w", descriptor.Digest, err)
	}
	defer layerReader.Close()

	verifier := content.NewVerifyReader(layerReader, descriptor)
	gzipReader, err := gzip.NewReader(verifier)
	if err != nil {
		return fmt.Errorf("open gzip layer: %w", err)
	}
	gzipClosed := false
	defer func() {
		if !gzipClosed {
			_ = gzipReader.Close()
		}
	}()
	tarReader := tar.NewReader(gzipReader)
	seen := make(map[string]struct{})
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read tar header: %w", err)
		}
		cleaned, err := safeArchivePath(header)
		if err != nil {
			return err
		}
		if _, duplicate := seen[cleaned]; duplicate {
			return fmt.Errorf("duplicate archive path %q", cleaned)
		}
		seen[cleaned] = struct{}{}

		target := filepath.Join(destination, filepath.FromSlash(cleaned))
		relative, err := filepath.Rel(destination, target)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return fmt.Errorf("archive path %q escapes extraction root", header.Name)
		}
		mode := header.FileInfo().Mode().Perm()
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, mode); err != nil {
				return fmt.Errorf("create archive directory %q: %w", cleaned, err)
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return fmt.Errorf("create parent for archive file %q: %w", cleaned, err)
			}
			file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
			if err != nil {
				return fmt.Errorf("create archive file %q: %w", cleaned, err)
			}
			_, copyErr := io.Copy(file, tarReader)
			closeErr := file.Close()
			if copyErr != nil {
				return fmt.Errorf("write archive file %q: %w", cleaned, copyErr)
			}
			if closeErr != nil {
				return fmt.Errorf("close archive file %q: %w", cleaned, closeErr)
			}
		default:
			return fmt.Errorf("archive path %q uses unsupported type %d", header.Name, header.Typeflag)
		}
	}
	if _, err := io.Copy(io.Discard, gzipReader); err != nil {
		return fmt.Errorf("finish gzip layer: %w", err)
	}
	if err := gzipReader.Close(); err != nil {
		return fmt.Errorf("close gzip layer: %w", err)
	}
	gzipClosed = true
	if _, err := io.Copy(io.Discard, verifier); err != nil {
		return fmt.Errorf("finish layer verification: %w", err)
	}
	if err := verifier.Verify(); err != nil {
		return fmt.Errorf("verify layer: %w", err)
	}
	if err := layerReader.Close(); err != nil {
		return fmt.Errorf("close layer: %w", err)
	}
	return nil
}

func safeArchivePath(header *tar.Header) (string, error) {
	if header.Name == "" {
		return "", fmt.Errorf("archive contains an empty path")
	}
	if strings.Contains(header.Name, `\`) {
		return "", fmt.Errorf("archive path %q contains a backslash", header.Name)
	}
	cleaned := path.Clean(header.Name)
	if path.IsAbs(header.Name) || cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("archive path %q is not a safe relative path", header.Name)
	}
	native := filepath.FromSlash(cleaned)
	if filepath.IsAbs(native) || filepath.VolumeName(native) != "" {
		return "", fmt.Errorf("archive path %q is not a safe relative path", header.Name)
	}
	if header.Typeflag == tar.TypeGNUSparse {
		return "", fmt.Errorf("archive path %q is sparse", header.Name)
	}
	for key := range header.PAXRecords {
		if strings.HasPrefix(key, "GNU.sparse.") || key == "SCHILY.filetype" && header.PAXRecords[key] == "sparse" {
			return "", fmt.Errorf("archive path %q is sparse", header.Name)
		}
	}
	return cleaned, nil
}
