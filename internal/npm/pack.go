package npm

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// PackedPackage is a deterministic npm-compatible package archive.
type PackedPackage struct {
	Name        string
	Version     string
	Filename    string
	Integrity   string
	Files       []string
	Tarball     []byte
	PackageJSON json.RawMessage
}

type packManifest struct {
	Name                string                     `json:"name"`
	Version             string                     `json:"version"`
	Files               []string                   `json:"files"`
	Locus               map[string]json.RawMessage `json:"locus"`
	Scripts             map[string]string          `json:"scripts"`
	BundledDependencies json.RawMessage            `json:"bundledDependencies"`
	BundleDependencies  json.RawMessage            `json:"bundleDependencies"`
}

type packEntry struct {
	source    string
	directory bool
	mode      int64
	size      int64
}

// Pack applies the reduced first-release packlist and creates a deterministic
// gzip tar archive with entries rooted at package/.
func Pack(root string) (PackedPackage, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return PackedPackage{}, fmt.Errorf("resolve package root: %w", err)
	}
	packageJSONPath := filepath.Join(root, "package.json")
	packageJSON, err := os.ReadFile(packageJSONPath)
	if err != nil {
		return PackedPackage{}, fmt.Errorf("read package.json: %w", err)
	}
	if err := rejectDuplicateJSONKeys(packageJSON); err != nil {
		return PackedPackage{}, fmt.Errorf("decode package.json: %w", err)
	}
	var manifest packManifest
	if err := decodeOneJSON(packageJSON, &manifest); err != nil {
		return PackedPackage{}, fmt.Errorf("decode package.json: %w", err)
	}
	if _, err := ParsePackageName(manifest.Name); err != nil {
		return PackedPackage{}, err
	}
	if err := ValidateVersion(manifest.Version); err != nil {
		return PackedPackage{}, err
	}
	if len(manifest.Files) == 0 {
		return PackedPackage{}, fmt.Errorf("package.json files must be a non-empty array")
	}
	if manifest.BundledDependencies != nil || manifest.BundleDependencies != nil {
		return PackedPackage{}, fmt.Errorf("bundled dependencies are not supported by locus-pkg pack")
	}
	for _, script := range []string{"prepublish", "prepare", "prepublishOnly", "prepack", "postpack"} {
		if strings.TrimSpace(manifest.Scripts[script]) != "" {
			return PackedPackage{}, fmt.Errorf("npm lifecycle script %q is not supported by locus-pkg pack", script)
		}
	}
	entry, err := parseLocusEntry(manifest.Locus)
	if err != nil {
		return PackedPackage{}, err
	}
	if err := rejectNPMIgnore(root); err != nil {
		return PackedPackage{}, err
	}

	entries := make(map[string]packEntry)
	if err := addPackPath(entries, "package.json", packageJSONPath); err != nil {
		return PackedPackage{}, err
	}
	rootEntries, err := os.ReadDir(root)
	if err != nil {
		return PackedPackage{}, fmt.Errorf("read package root: %w", err)
	}
	for _, candidate := range rootEntries {
		if candidate.IsDir() || !alwaysIncludedDocument(candidate.Name()) {
			continue
		}
		if err := addPackPath(entries, candidate.Name(), filepath.Join(root, candidate.Name())); err != nil {
			return PackedPackage{}, err
		}
	}
	for _, declared := range manifest.Files {
		relative, err := cleanPackPath(declared)
		if err != nil {
			return PackedPackage{}, fmt.Errorf("invalid package.json files entry %q: %w", declared, err)
		}
		source := root
		if relative != "." {
			source = filepath.Join(root, filepath.FromSlash(relative))
		}
		if err := addPackTree(entries, root, relative, source); err != nil {
			return PackedPackage{}, err
		}
	}

	entry = strings.TrimPrefix(path.Clean(strings.ReplaceAll(entry, "\\", "/")), "./")
	manifestCount := 0
	for name, candidate := range entries {
		if candidate.directory || !isScopeManifest(name) {
			continue
		}
		manifestCount++
	}
	selectedEntry, exists := entries[entry]
	if !exists || selectedEntry.directory || manifestCount != 1 || !isScopeManifest(entry) {
		return PackedPackage{}, fmt.Errorf("locus.entry must be the sole Scope manifest in the packed package")
	}

	names := make([]string, 0, len(entries))
	files := make([]string, 0, len(entries))
	for name, candidate := range entries {
		names = append(names, name)
		if !candidate.directory {
			files = append(files, name)
		}
	}
	sort.Strings(names)
	sort.Strings(files)

	var output bytes.Buffer
	gzipWriter, err := gzip.NewWriterLevel(&output, gzip.BestCompression)
	if err != nil {
		return PackedPackage{}, fmt.Errorf("create package gzip: %w", err)
	}
	gzipWriter.Header.ModTime = time.Unix(0, 0).UTC()
	gzipWriter.Header.OS = 255
	tarWriter := tar.NewWriter(gzipWriter)
	if err := tarWriter.WriteHeader(&tar.Header{Name: "package/", Typeflag: tar.TypeDir, Mode: 0o755, ModTime: time.Unix(0, 0).UTC()}); err != nil {
		return PackedPackage{}, fmt.Errorf("write package archive root: %w", err)
	}
	for _, name := range names {
		candidate := entries[name]
		header := &tar.Header{
			Name:     "package/" + name,
			Mode:     candidate.mode,
			Size:     candidate.size,
			ModTime:  time.Unix(0, 0).UTC(),
			Typeflag: tar.TypeReg,
		}
		if candidate.directory {
			header.Name += "/"
			header.Typeflag = tar.TypeDir
			header.Size = 0
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			return PackedPackage{}, fmt.Errorf("write package archive header %q: %w", name, err)
		}
		if candidate.directory {
			continue
		}
		file, err := os.Open(candidate.source)
		if err != nil {
			return PackedPackage{}, fmt.Errorf("read package file %q: %w", name, err)
		}
		written, copyErr := io.CopyN(tarWriter, file, candidate.size)
		var extra [1]byte
		extraCount, extraErr := file.Read(extra[:])
		closeErr := file.Close()
		if copyErr != nil || written != candidate.size {
			return PackedPackage{}, fmt.Errorf("read package file %q: %w", name, copyErr)
		}
		if extraCount != 0 || (extraErr != nil && extraErr != io.EOF) {
			return PackedPackage{}, fmt.Errorf("package file %q changed while packing", name)
		}
		if closeErr != nil {
			return PackedPackage{}, fmt.Errorf("close package file %q: %w", name, closeErr)
		}
	}
	if err := tarWriter.Close(); err != nil {
		return PackedPackage{}, fmt.Errorf("finish package tar: %w", err)
	}
	if err := gzipWriter.Close(); err != nil {
		return PackedPackage{}, fmt.Errorf("finish package gzip: %w", err)
	}
	if output.Len() > MaxTarballSize {
		return PackedPackage{}, fmt.Errorf("package tarball exceeds %d bytes", MaxTarballSize)
	}

	archive := append([]byte(nil), output.Bytes()...)
	integrity := IntegrityFor(archive)
	return PackedPackage{
		Name:        manifest.Name,
		Version:     manifest.Version,
		Filename:    TarballFilename(manifest.Name, manifest.Version),
		Integrity:   integrity.String(),
		Files:       files,
		Tarball:     archive,
		PackageJSON: append(json.RawMessage(nil), packageJSON...),
	}, nil
}

func parseLocusEntry(locus map[string]json.RawMessage) (string, error) {
	if len(locus) != 1 {
		return "", fmt.Errorf("package.json locus must contain only entry")
	}
	raw, ok := locus["entry"]
	if !ok {
		return "", fmt.Errorf("package.json locus.entry is required")
	}
	var entry string
	if err := json.Unmarshal(raw, &entry); err != nil || entry == "" {
		return "", fmt.Errorf("package.json locus.entry must be a non-empty relative path")
	}
	if strings.Contains(entry, "\\") || strings.Contains(entry, ":") || path.IsAbs(entry) || filepath.IsAbs(entry) || filepath.VolumeName(entry) != "" || path.Clean(entry) == ".." || strings.HasPrefix(path.Clean(entry), "../") {
		return "", fmt.Errorf("package.json locus.entry must remain inside the package")
	}
	return entry, nil
}

func cleanPackPath(value string) (string, error) {
	if value == "" || strings.ContainsAny(value, "*?[]{}!") || strings.Contains(value, "\\") || path.IsAbs(value) {
		return "", fmt.Errorf("must be a relative literal file or directory")
	}
	clean := path.Clean(value)
	if clean == ".." || strings.HasPrefix(clean, "../") || filepath.VolumeName(value) != "" {
		return "", fmt.Errorf("must remain inside the package")
	}
	return strings.TrimPrefix(clean, "./"), nil
}

func rejectNPMIgnore(root string) error {
	return filepath.WalkDir(root, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if current != root {
			relative, err := filepath.Rel(root, current)
			if err != nil {
				return err
			}
			slash := filepath.ToSlash(relative)
			if entry.IsDir() && excludedPackPath(slash, true) {
				return filepath.SkipDir
			}
		}
		if strings.EqualFold(entry.Name(), ".npmignore") {
			return fmt.Errorf(".npmignore is not supported by locus-pkg pack")
		}
		return nil
	})
}

func addPackTree(entries map[string]packEntry, root, relative, source string) error {
	info, err := os.Lstat(source)
	if err != nil {
		return fmt.Errorf("read package.json files path %q: %w", relative, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("package path %q is a symlink", relative)
	}
	if !info.IsDir() {
		return addPackPath(entries, relative, source)
	}
	return filepath.WalkDir(source, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		name, err := filepath.Rel(root, current)
		if err != nil {
			return err
		}
		name = filepath.ToSlash(name)
		if name == "." {
			return nil
		}
		if excludedPackPath(name, entry.IsDir()) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		return addPackPath(entries, name, current)
	})
}

func addPackPath(entries map[string]packEntry, name, source string) error {
	name = filepath.ToSlash(name)
	if excludedPackPath(name, false) {
		return nil
	}
	info, err := os.Lstat(source)
	if err != nil {
		return fmt.Errorf("read package path %q: %w", name, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("package path %q is a symlink", name)
	}
	if !info.Mode().IsRegular() && !info.IsDir() {
		return fmt.Errorf("package path %q is not a regular file or directory", name)
	}
	mode := int64(0o644)
	if info.IsDir() {
		mode = 0o755
	} else if info.Mode()&0o111 != 0 {
		mode = 0o755
	}
	entries[name] = packEntry{source: source, directory: info.IsDir(), mode: mode, size: info.Size()}
	return nil
}

func excludedPackPath(name string, directory bool) bool {
	parts := strings.Split(strings.ToLower(filepath.ToSlash(name)), "/")
	for _, part := range parts {
		if part == ".git" || part == ".locus" || part == "node_modules" {
			return true
		}
	}
	base := parts[len(parts)-1]
	return (!directory && (base == "locus.lock" || strings.HasSuffix(base, ".tgz")))
}

func alwaysIncludedDocument(name string) bool {
	upper := strings.ToUpper(name)
	for _, prefix := range []string{"README", "LICENSE", "LICENCE", "NOTICE"} {
		if upper == prefix || strings.HasPrefix(upper, prefix+".") {
			return true
		}
	}
	return false
}

func isScopeManifest(name string) bool {
	switch strings.ToLower(path.Base(name)) {
	case "locus.yaml", "locus.yml", "locus.json":
		return true
	default:
		return false
	}
}

// TarballFilename returns npm's conventional archive filename.
func TarballFilename(name, version string) string {
	return strings.ReplaceAll(strings.TrimPrefix(name, "@"), "/", "-") + "-" + version + ".tgz"
}

func rejectDuplicateJSONKeys(content []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(content))
	var visit func() error
	visit = func() error {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		delimiter, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delimiter {
		case '{':
			keys := make(map[string]struct{})
			for decoder.More() {
				keyToken, err := decoder.Token()
				if err != nil {
					return err
				}
				key := keyToken.(string)
				if _, exists := keys[key]; exists {
					return fmt.Errorf("duplicate JSON field %q", key)
				}
				keys[key] = struct{}{}
				if err := visit(); err != nil {
					return err
				}
			}
			_, err = decoder.Token()
			return err
		case '[':
			for decoder.More() {
				if err := visit(); err != nil {
					return err
				}
			}
			_, err = decoder.Token()
			return err
		default:
			return fmt.Errorf("unexpected JSON delimiter")
		}
	}
	if err := visit(); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return fmt.Errorf("unexpected trailing JSON value")
		}
		return err
	}
	return nil
}
