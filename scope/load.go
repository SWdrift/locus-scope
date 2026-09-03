package scope

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// FindScope walks from start toward the filesystem root and returns the nearest
// directory containing a locus manifest.
func FindScope(start string) (string, error) {
	current, err := filepath.Abs(start)
	if err != nil {
		return "", fmt.Errorf("resolve start directory %q: %w", start, err)
	}
	info, err := os.Stat(current)
	if err != nil {
		return "", fmt.Errorf("inspect start path %q: %w", current, err)
	}
	if !info.IsDir() {
		current = filepath.Dir(current)
	}

	for {
		for _, name := range manifestFileNames {
			info, err := os.Stat(filepath.Join(current, name))
			if err == nil && !info.IsDir() {
				return current, nil
			}
			if err != nil && !os.IsNotExist(err) {
				return "", fmt.Errorf("inspect %s: %w", filepath.Join(current, name), err)
			}
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return "", fmt.Errorf("no locus manifest found from %q to the filesystem root", start)
}

// Load reads a local root scope, discovers every reachable import, and returns
// a fully validated workspace.
func Load(rootDirectory string) (*Workspace, error) {
	root, err := canonicalScopeDirectory(rootDirectory)
	if err != nil {
		return nil, fmt.Errorf("load root scope: %w", err)
	}

	type pendingScope struct {
		key         ScopeKey
		importer    ScopeKey
		alias       string
		sourceValue string
	}

	workspace := &Workspace{
		Root:   root,
		Scopes: make(map[ScopeKey]*Scope),
	}
	queue := []pendingScope{{key: root}}

	for len(queue) > 0 {
		pending := queue[0]
		queue = queue[1:]
		if _, loaded := workspace.Scopes[pending.key]; loaded {
			continue
		}

		loaded, err := decodeScope(string(pending.key))
		if err != nil {
			if pending.importer != "" {
				importer := workspace.Scopes[pending.importer]
				return nil, fmt.Errorf("%s: import %q (%q): %w", importer.manifestPath, pending.alias, pending.sourceValue, err)
			}
			return nil, err
		}

		// Register before discovering children. This makes cyclic imports ordinary.
		workspace.Scopes[pending.key] = loaded

		aliases := sortedStringKeys(loaded.Manifest.Imports)
		for _, alias := range aliases {
			sourceValue := loaded.Manifest.Imports[alias]
			targetPath := sourceValue
			if !filepath.IsAbs(targetPath) {
				targetPath = filepath.Join(string(loaded.Key), targetPath)
			}
			target, err := canonicalScopeDirectory(targetPath)
			if err != nil {
				return nil, fmt.Errorf("%s: import %q (%q): %w", loaded.manifestPath, alias, sourceValue, err)
			}
			loaded.Imports[alias] = target
			if _, alreadyLoaded := workspace.Scopes[target]; !alreadyLoaded {
				queue = append(queue, pendingScope{
					key: target, importer: loaded.Key, alias: alias, sourceValue: sourceValue,
				})
			}
		}
	}

	if err := workspace.validate(); err != nil {
		return nil, err
	}
	return workspace, nil
}

func canonicalScopeDirectory(path string) (ScopeKey, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve scope path %q: %w", path, err)
	}
	absolute = filepath.Clean(absolute)
	info, err := os.Stat(absolute)
	if err != nil {
		return "", fmt.Errorf("scope source %q: %w", absolute, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("scope source %q is not a directory", absolute)
	}

	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("resolve scope source %q: %w", absolute, err)
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return "", fmt.Errorf("normalize scope source %q: %w", resolved, err)
	}
	return ScopeKey(filepath.Clean(resolved)), nil
}

func sortedStringKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func (w *Workspace) sortedScopeKeys() []ScopeKey {
	keys := make([]ScopeKey, 0, len(w.Scopes))
	for key := range w.Scopes {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	return keys
}
