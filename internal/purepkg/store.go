package purepkg

import (
	"context"
	"crypto/rand"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	locusnpm "locus-scope/internal/npm"
	"locus-scope/internal/packageenv"
)

type materializedPackage struct {
	root     string
	metadata packageenv.PackageMetadata
	isLocus  bool
}

type materialization struct {
	packages      map[string]materializedPackage
	stagedCache   map[string]string
	stagedStores  map[string]string
	prunedStores  map[string]string
	createdStores []string
	reused        int
	fetched       int
	installed     int
	operationRoot string
}

func newOperationRoot(root string) (string, error) {
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", fmt.Errorf("create operation id: %w", err)
	}
	operationRoot := filepath.Join(root, ".locus", "tmp", hex.EncodeToString(nonce[:]))
	if err := os.MkdirAll(operationRoot, 0o755); err != nil {
		return "", fmt.Errorf("create operation directory: %w", err)
	}
	return operationRoot, nil
}

func materialize(ctx context.Context, root string, lock lockFile, client *locusnpm.Client, offline bool) (*materialization, error) {
	operationRoot, err := newOperationRoot(root)
	if err != nil {
		return nil, err
	}
	result := &materialization{
		packages:      make(map[string]materializedPackage, len(lock.Packages)),
		stagedCache:   map[string]string{},
		stagedStores:  map[string]string{},
		operationRoot: operationRoot,
	}
	failed := true
	defer func() {
		if failed {
			_ = os.RemoveAll(operationRoot)
		}
	}()
	identities := make([]string, 0, len(lock.Packages))
	for identity := range lock.Packages {
		identities = append(identities, identity)
	}
	sort.Strings(identities)
	for _, identity := range identities {
		record := lock.Packages[identity]
		integrity, _ := locusnpm.ParseIntegrity(record.Integrity)
		stem := integrity.Algorithm() + "-" + integrity.Hex()
		cachePath := filepath.Join(root, ".locus", "cache", stem+".tgz")
		storeRoot := filepath.Join(root, ".locus", "packages", stem)
		packageRoot := filepath.Join(storeRoot, "package")
		storeExists := false
		if info, statErr := os.Stat(storeRoot); statErr == nil {
			if !info.IsDir() {
				return nil, fmt.Errorf("generated package store %s is not a directory", storeRoot)
			}
			storeExists = true
		} else if !os.IsNotExist(statErr) {
			return nil, fmt.Errorf("inspect package store %s: %w", storeRoot, statErr)
		}

		tarball, readErr := os.ReadFile(cachePath)
		if readErr == nil {
			if err := integrity.Verify(tarball); err != nil {
				return nil, fmt.Errorf("invalid cached tarball for %s: %w", identity, err)
			}
		} else if !os.IsNotExist(readErr) {
			return nil, fmt.Errorf("read cached tarball for %s: %w", identity, readErr)
		} else {
			if offline {
				return nil, fmt.Errorf("offline package %s is missing verified cache and store content", identity)
			}
			if client == nil {
				return nil, fmt.Errorf("no Registry client available for %s", identity)
			}
			tarball, err = client.Download(ctx, record.Resolved, record.Integrity)
			if err != nil {
				return nil, fmt.Errorf("download %s: %w", identity, err)
			}
			stagedCache := filepath.Join(operationRoot, "cache", stem+".tgz")
			if err := os.MkdirAll(filepath.Dir(stagedCache), 0o755); err != nil {
				return nil, fmt.Errorf("stage cache for %s: %w", identity, err)
			}
			if err := os.WriteFile(stagedCache, tarball, 0o644); err != nil {
				return nil, fmt.Errorf("stage cache for %s: %w", identity, err)
			}
			result.stagedCache[cachePath] = stagedCache
			result.fetched++
		}
		if storeExists {
			verifiedStore := filepath.Join(operationRoot, "verified", stem)
			if err := os.MkdirAll(verifiedStore, 0o755); err != nil {
				return nil, fmt.Errorf("stage store verification for %s: %w", identity, err)
			}
			if err := locusnpm.ExtractTarball(tarball, verifiedStore); err != nil {
				return nil, fmt.Errorf("extract verified content for %s: %w", identity, err)
			}
			verifiedPackageRoot := filepath.Join(verifiedStore, "package")
			metadata, isLocus, err := validateStoredPackage(identity, verifiedPackageRoot, record)
			if err != nil {
				return nil, fmt.Errorf("validate verified content for %s: %w", identity, err)
			}
			if err := comparePackageTrees(verifiedPackageRoot, packageRoot); err != nil {
				return nil, fmt.Errorf("existing package store for %s does not match its integrity-verified tarball: %w", identity, err)
			}
			result.packages[identity] = materializedPackage{root: verifiedPackageRoot, metadata: metadata, isLocus: isLocus}
			result.reused++
			continue
		}

		stagedStore := filepath.Join(operationRoot, "packages", stem)
		if err := os.MkdirAll(stagedStore, 0o755); err != nil {
			return nil, fmt.Errorf("stage package %s: %w", identity, err)
		}
		if err := locusnpm.ExtractTarball(tarball, stagedStore); err != nil {
			return nil, fmt.Errorf("extract %s: %w", identity, err)
		}
		stagedPackageRoot := filepath.Join(stagedStore, "package")
		metadata, isLocus, err := validateStoredPackage(identity, stagedPackageRoot, record)
		if err != nil {
			return nil, fmt.Errorf("validate extracted package %s: %w", identity, err)
		}
		result.packages[identity] = materializedPackage{root: stagedPackageRoot, metadata: metadata, isLocus: isLocus}
		result.stagedStores[storeRoot] = stagedStore
		result.installed++
	}
	failed = false
	return result, nil
}

func comparePackageTrees(expected, actual string) error {
	expectedDigest, err := packageTreeDigest(expected)
	if err != nil {
		return fmt.Errorf("read verified package tree: %w", err)
	}
	actualDigest, err := packageTreeDigest(actual)
	if err != nil {
		return fmt.Errorf("read stored package tree: %w", err)
	}
	if expectedDigest != actualDigest {
		return fmt.Errorf("package tree content differs")
	}
	return nil
}

func packageTreeDigest(root string) ([sha512.Size]byte, error) {
	digest := sha512.New()
	err := filepath.WalkDir(root, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(root, current)
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symbolic link %q is not allowed", filepath.ToSlash(relative))
		}
		switch {
		case entry.IsDir():
			_, err = io.WriteString(digest, "d\x00"+filepath.ToSlash(relative)+"\x00")
			return err
		case entry.Type().IsRegular():
			if _, err := io.WriteString(digest, "f\x00"+filepath.ToSlash(relative)+"\x00"); err != nil {
				return err
			}
			file, err := os.Open(current)
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(digest, file)
			closeErr := file.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
			_, err = io.WriteString(digest, "\x00")
			return err
		default:
			return fmt.Errorf("unsupported package tree entry %q", filepath.ToSlash(relative))
		}
	})
	if err != nil {
		return [sha512.Size]byte{}, err
	}
	var sum [sha512.Size]byte
	copy(sum[:], digest.Sum(nil))
	return sum, nil
}

func validateStoredPackage(identity, root string, record lockPackage) (packageenv.PackageMetadata, bool, error) {
	metadata, isLocus, err := packageenv.ValidatePackage(root)
	if err != nil {
		return packageenv.PackageMetadata{}, false, err
	}
	name, version, _ := locusnpm.ParseIdentity(identity)
	if metadata.Name != name || metadata.Version != version {
		return packageenv.PackageMetadata{}, false, fmt.Errorf("metadata is %s@%s, expected %s@%s", metadata.Name, metadata.Version, name, version)
	}
	if len(metadata.Dependencies) != len(record.Dependencies) {
		return packageenv.PackageMetadata{}, false, fmt.Errorf("dependency metadata differs from lock")
	}
	for dependencyName, edge := range record.Dependencies {
		if metadata.Dependencies[dependencyName] != edge.Specifier {
			return packageenv.PackageMetadata{}, false, fmt.Errorf("dependency %s metadata differs from lock", dependencyName)
		}
	}
	return metadata, isLocus, nil
}

func (m *materialization) publish() error {
	for target, staged := range m.stagedCache {
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.Rename(staged, target); err != nil {
			if _, statErr := os.Stat(target); statErr != nil {
				return fmt.Errorf("publish cached tarball: %w", err)
			}
		}
	}
	for target, staged := range m.stagedStores {
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.Rename(staged, target); err != nil {
			if _, statErr := os.Stat(target); statErr != nil {
				return fmt.Errorf("publish package store: %w", err)
			}
		} else {
			m.createdStores = append(m.createdStores, target)
		}
		packageRoot := filepath.Join(target, "package")
		for identity, materialized := range m.packages {
			if materialized.root == filepath.Join(staged, "package") {
				materialized.root = packageRoot
				m.packages[identity] = materialized
			}
		}
	}
	return nil
}

func (m *materialization) rollback() {
	for _, path := range m.createdStores {
		_ = os.RemoveAll(path)
	}
	for original, staged := range m.prunedStores {
		_ = os.Rename(staged, original)
	}
	_ = os.RemoveAll(m.operationRoot)
}

func (m *materialization) close() {
	_ = os.RemoveAll(m.operationRoot)
}
