package packages

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/natefinch/atomic"
	"gopkg.in/yaml.v3"
)

const lockVersion = 1

type Lock struct {
	Version  int                      `yaml:"version"`
	Packages map[string]LockedPackage `yaml:"packages"`
}

type LockedPackage struct {
	Resolved string `yaml:"resolved"`
}

func emptyLock() Lock {
	return Lock{Version: lockVersion, Packages: make(map[string]LockedPackage)}
}

func readLock(rootDirectory string) (Lock, error) {
	path := filepath.Join(rootDirectory, "locus.lock")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return emptyLock(), nil
	}
	if err != nil {
		return Lock{}, fmt.Errorf("read lock file %s: %w", path, err)
	}

	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	var lock Lock
	if err := decoder.Decode(&lock); err != nil {
		return Lock{}, fmt.Errorf("decode lock file %s: %w", path, err)
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); err != io.EOF {
		if err != nil {
			return Lock{}, fmt.Errorf("decode lock file %s: %w", path, err)
		}
		return Lock{}, fmt.Errorf("decode lock file %s: multiple YAML documents are not supported", path)
	}
	if lock.Packages == nil {
		return Lock{}, fmt.Errorf("decode lock file %s: packages must be a mapping", path)
	}
	if err := validateLock(lock); err != nil {
		return Lock{}, fmt.Errorf("decode lock file %s: %w", path, err)
	}
	return lock, nil
}

func validateLock(lock Lock) error {
	if lock.Version != lockVersion {
		return fmt.Errorf("unsupported lock version %d; expected %d", lock.Version, lockVersion)
	}
	for key, locked := range lock.Packages {
		if key == "" {
			return fmt.Errorf("lock package key must not be empty")
		}
		if locked.Resolved == "" {
			return fmt.Errorf("lock package %q has an empty resolved value", key)
		}
		requested, err := parsePackageReference(key)
		if err != nil {
			return fmt.Errorf("lock package key %q: %w", key, err)
		}
		if !requested.Mutable {
			return fmt.Errorf("lock package key %q must be a mutable tag reference", key)
		}
		if requested.Canonical != key {
			return fmt.Errorf("lock package key %q is not canonical; use %q", key, requested.Canonical)
		}
		resolved, err := parsePackageReference(locked.Resolved)
		if err != nil {
			return fmt.Errorf("lock package %q resolved value: %w", key, err)
		}
		if resolved.Mutable {
			return fmt.Errorf("lock package %q resolved value must be a digest reference", key)
		}
		if resolved.Canonical != locked.Resolved {
			return fmt.Errorf("lock package %q resolved value %q is not canonical", key, locked.Resolved)
		}
		if requested.Registry != resolved.Registry || requested.Repository != resolved.Repository {
			return fmt.Errorf("lock package %q resolves to a different registry or repository %q", key, locked.Resolved)
		}
	}
	return nil
}

func encodeLock(lock Lock) ([]byte, error) {
	if lock.Packages == nil {
		lock.Packages = make(map[string]LockedPackage)
	}
	if err := validateLock(lock); err != nil {
		return nil, err
	}

	root := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	root.Content = append(root.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "version"},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: "1"},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "packages"},
	)
	packages := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	keys := make([]string, 0, len(lock.Packages))
	for key := range lock.Packages {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		entry := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{
			{Kind: yaml.ScalarNode, Tag: "!!str", Value: "resolved"},
			{Kind: yaml.ScalarNode, Tag: "!!str", Value: lock.Packages[key].Resolved},
		}}
		packages.Content = append(packages.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, entry)
	}
	root.Content = append(root.Content, packages)

	document := &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{root}}
	var output bytes.Buffer
	encoder := yaml.NewEncoder(&output)
	encoder.SetIndent(2)
	if err := encoder.Encode(document); err != nil {
		return nil, fmt.Errorf("encode lock file: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return nil, fmt.Errorf("encode lock file: %w", err)
	}
	return output.Bytes(), nil
}

func writeLockIfChanged(rootDirectory string, lock Lock) (bool, error) {
	data, err := encodeLock(lock)
	if err != nil {
		return false, err
	}
	path := filepath.Join(rootDirectory, "locus.lock")
	existing, err := os.ReadFile(path)
	if err == nil && bytes.Equal(existing, data) {
		return false, nil
	}
	if err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("read existing lock file %s: %w", path, err)
	}
	if err := atomic.WriteFile(path, bytes.NewReader(data)); err != nil {
		return false, fmt.Errorf("write lock file %s: %w", path, err)
	}
	return true, nil
}
