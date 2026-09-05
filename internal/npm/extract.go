package npm

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const (
	MaxPackumentSize   = 16 << 20
	MaxTarballSize     = 256 << 20
	MaxExtractedSize   = int64(1 << 30)
	MaxArchiveEntries  = 100_000
	MaxArchiveFileSize = int64(256 << 20)
)

// ExtractTarball safely extracts an npm tgz into destination. Every member
// must be a canonical path beneath the standard package/ archive root.
func ExtractTarball(content []byte, destination string) error {
	if len(content) > MaxTarballSize {
		return fmt.Errorf("package tarball exceeds %d bytes", MaxTarballSize)
	}
	gzipReader, err := gzip.NewReader(bytes.NewReader(content))
	if err != nil {
		return fmt.Errorf("read package tarball: %w", err)
	}
	defer gzipReader.Close()

	destination, err = filepath.Abs(destination)
	if err != nil {
		return fmt.Errorf("resolve extraction destination: %w", err)
	}
	if info, statErr := os.Lstat(destination); statErr == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("package extraction destination must be a directory, not a symlink or file")
		}
		entries, readErr := os.ReadDir(destination)
		if readErr != nil {
			return fmt.Errorf("read extraction destination: %w", readErr)
		}
		if len(entries) != 0 {
			return fmt.Errorf("package extraction destination must be empty")
		}
	} else if !os.IsNotExist(statErr) {
		return fmt.Errorf("inspect extraction destination: %w", statErr)
	}
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return fmt.Errorf("create extraction destination: %w", err)
	}
	resolvedDestination, err := filepath.EvalSymlinks(destination)
	if err != nil {
		return fmt.Errorf("resolve extraction destination symlinks: %w", err)
	}
	destination = resolvedDestination

	seen := make(map[string]bool)
	archive := tar.NewReader(gzipReader)
	var totalSize int64
	entryCount := 0
	for {
		header, err := archive.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read package tarball: %w", err)
		}
		entryCount++
		if entryCount > MaxArchiveEntries {
			return fmt.Errorf("package tarball exceeds %d entries", MaxArchiveEntries)
		}
		name, err := safeArchiveName(header.Name)
		if err != nil {
			return err
		}
		key := strings.ToLower(name)
		if _, exists := seen[key]; exists {
			return fmt.Errorf("package tarball contains duplicate path %q", name)
		}
		isDirectory := header.Typeflag == tar.TypeDir
		seen[key] = isDirectory
		for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
			if directory, exists := seen[strings.ToLower(parent)]; exists && !directory {
				return fmt.Errorf("package tarball path %q descends from a file", name)
			}
		}
		if header.Typeflag == tar.TypeGNUSparse || containsSparseMetadata(header.PAXRecords) {
			return fmt.Errorf("package tarball contains sparse entry %q", name)
		}

		target := filepath.Join(destination, filepath.FromSlash(name))
		relative, err := filepath.Rel(destination, target)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return fmt.Errorf("package tarball path %q escapes destination", name)
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return fmt.Errorf("create package directory %q: %w", name, err)
			}
		case tar.TypeReg, tar.TypeRegA:
			if header.Size < 0 || header.Size > MaxArchiveFileSize {
				return fmt.Errorf("package tarball file %q exceeds %d bytes", name, MaxArchiveFileSize)
			}
			totalSize += header.Size
			if totalSize > MaxExtractedSize {
				return fmt.Errorf("package tarball exceeds %d extracted bytes", MaxExtractedSize)
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return fmt.Errorf("create package directory for %q: %w", name, err)
			}
			mode := os.FileMode(0o644)
			if header.Mode&0o111 != 0 {
				mode = 0o755
			}
			file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
			if err != nil {
				return fmt.Errorf("create package file %q: %w", name, err)
			}
			written, copyErr := io.CopyN(file, archive, header.Size)
			closeErr := file.Close()
			if copyErr != nil || written != header.Size {
				return fmt.Errorf("extract package file %q: %w", name, copyErr)
			}
			if closeErr != nil {
				return fmt.Errorf("close package file %q: %w", name, closeErr)
			}
		default:
			return fmt.Errorf("package tarball contains unsupported entry %q", name)
		}
	}
	return nil
}

func safeArchiveName(value string) (string, error) {
	if value == "" || strings.Contains(value, "\\") || strings.Contains(value, ":") || strings.HasPrefix(value, "/") {
		return "", fmt.Errorf("package tarball contains unsafe path %q", value)
	}
	name := strings.TrimSuffix(value, "/")
	if name == "" || path.Clean(name) != name || (name != "package" && !strings.HasPrefix(name, "package/")) {
		return "", fmt.Errorf("package tarball contains unsafe path %q", value)
	}
	return name, nil
}

func containsSparseMetadata(records map[string]string) bool {
	for key := range records {
		lower := strings.ToLower(key)
		if strings.HasPrefix(lower, "gnu.sparse") || lower == "schily.realsize" {
			return true
		}
	}
	return false
}
