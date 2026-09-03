package scope

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Source identifies a scope independently from the local directory used to read it.
type Source struct {
	Key       ScopeKey
	LocalPath string
}

// Resolver resolves one manifest import from its owning source.
type Resolver interface {
	Resolve(from Source, reference string) (Source, error)
}

// LocalResolver resolves filesystem imports from file sources.
type LocalResolver struct{}

type pendingScope struct {
	source      Source
	importer    ScopeKey
	alias       string
	sourceValue string
}

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

// NewLocalSource validates a local scope directory and assigns its canonical file URI key.
func NewLocalSource(path string) (Source, error) {
	localPath, err := canonicalScopeDirectory(path)
	if err != nil {
		return Source{}, err
	}
	return Source{Key: localScopeKey(localPath), LocalPath: localPath}, nil
}

// Resolve resolves relative and absolute filesystem imports from a file source.
func (LocalResolver) Resolve(from Source, reference string) (Source, error) {
	parsed, err := url.Parse(string(from.Key))
	if err != nil || parsed.Scheme != "file" || !strings.HasPrefix(string(from.Key), "file://") {
		return Source{}, fmt.Errorf("resolve local import %q from unsupported source %q", reference, from.Key)
	}
	if separator := strings.Index(reference, "://"); separator >= 0 {
		return Source{}, fmt.Errorf("resolve local import %q from %q: unsupported source scheme %q", reference, from.Key, reference[:separator])
	}

	target := reference
	if !filepath.IsAbs(target) {
		target = filepath.Join(from.LocalPath, target)
	}
	resolved, err := NewLocalSource(target)
	if err != nil {
		return Source{}, err
	}
	return resolved, nil
}

// ReadSource decodes and validates one source without resolving imports.
func ReadSource(source Source) (*Scope, error) {
	normalized, err := normalizeSource(source)
	if err != nil {
		return nil, err
	}
	return decodeScope(normalized)
}

// CheckSource validates the local files for one source without resolving imports.
func CheckSource(source Source) error {
	_, err := ReadSource(source)
	return err
}

// Load discovers every reachable import and returns a fully validated workspace.
func Load(root Source, resolve Resolver) (*Workspace, error) {
	if resolve == nil {
		return nil, fmt.Errorf("load root scope: resolver is required")
	}
	root, err := normalizeSource(root)
	if err != nil {
		return nil, fmt.Errorf("load root scope: %w", err)
	}

	workspace := &Workspace{
		Root:   root.Key,
		Scopes: make(map[ScopeKey]*Scope),
	}
	sourcePaths := map[ScopeKey]string{root.Key: root.LocalPath}
	queue := []pendingScope{{source: root}}

	for len(queue) > 0 {
		pending := queue[0]
		queue = queue[1:]
		source, err := normalizeSource(pending.source)
		if err != nil {
			return nil, wrapImportError(workspace, pending, err)
		}
		if previous, exists := sourcePaths[source.Key]; exists && previous != source.LocalPath {
			return nil, wrapImportError(workspace, pending,
				fmt.Errorf("scope key %q resolves to both %q and %q", source.Key, previous, source.LocalPath))
		}
		sourcePaths[source.Key] = source.LocalPath
		if _, loaded := workspace.Scopes[source.Key]; loaded {
			continue
		}

		loaded, err := decodeScope(source)
		if err != nil {
			return nil, wrapImportError(workspace, pending, err)
		}

		// Register before discovering children so cyclic imports are ordinary.
		workspace.Scopes[source.Key] = loaded

		aliases := sortedStringKeys(loaded.Manifest.Imports)
		for _, alias := range aliases {
			sourceValue := loaded.Manifest.Imports[alias]
			target, err := resolve.Resolve(source, sourceValue)
			if err != nil {
				return nil, fmt.Errorf("%s: import %q (%q): %w", loaded.manifestPath, alias, sourceValue, err)
			}
			target, err = normalizeSource(target)
			if err != nil {
				return nil, fmt.Errorf("%s: import %q (%q): %w", loaded.manifestPath, alias, sourceValue, err)
			}
			if previous, exists := sourcePaths[target.Key]; exists && previous != target.LocalPath {
				return nil, fmt.Errorf("%s: import %q (%q): scope key %q resolves to both %q and %q",
					loaded.manifestPath, alias, sourceValue, target.Key, previous, target.LocalPath)
			}
			sourcePaths[target.Key] = target.LocalPath
			loaded.Imports[alias] = target.Key
			if _, alreadyLoaded := workspace.Scopes[target.Key]; !alreadyLoaded {
				queue = append(queue, pendingScope{
					source: target, importer: loaded.Key, alias: alias, sourceValue: sourceValue,
				})
			}
		}
	}

	if err := workspace.validate(); err != nil {
		return nil, err
	}
	return workspace, nil
}

func wrapImportError(workspace *Workspace, pending pendingScope, err error) error {
	if pending.importer == "" {
		return err
	}
	importer := workspace.Scopes[pending.importer]
	return fmt.Errorf("%s: import %q (%q): %w", importer.manifestPath, pending.alias, pending.sourceValue, err)
}

func normalizeSource(source Source) (Source, error) {
	if source.Key == "" {
		return Source{}, fmt.Errorf("scope source key is required")
	}
	if strings.TrimSpace(source.LocalPath) == "" {
		return Source{}, fmt.Errorf("scope source %q local path is required", source.Key)
	}
	localPath, err := canonicalScopeDirectory(source.LocalPath)
	if err != nil {
		return Source{}, err
	}
	source.LocalPath = localPath
	return source, nil
}

func canonicalScopeDirectory(path string) (string, error) {
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
	return filepath.Clean(resolved), nil
}

func localScopeKey(path string) ScopeKey {
	slashed := filepath.ToSlash(path)
	volume := filepath.VolumeName(path)
	if strings.HasPrefix(volume, `\\`) {
		authorityPath := strings.TrimPrefix(slashed, "//")
		host, rest, _ := strings.Cut(authorityPath, "/")
		return ScopeKey((&url.URL{Scheme: "file", Host: host, Path: "/" + rest}).String())
	}
	if len(volume) == 2 && volume[1] == ':' {
		slashed = "/" + slashed
	}
	return ScopeKey((&url.URL{Scheme: "file", Path: slashed}).String())
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
