package packageenv

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"locus-scope/internal/scope"
)

// Identity is the stable identity assigned by the Node package adapter.
type Identity string

// PackageDescriptor describes one installed Locus package. Each dependency
// edge is resolved by npm or pnpm in this package's importer context.
type PackageDescriptor struct {
	Root         string              `json:"root"`
	Entry        string              `json:"entry"`
	Dependencies map[string]Identity `json:"dependencies"`
}

// Environment is the exact installed package graph resolved by the Node adapter.
type Environment struct {
	RootScope        string                         `json:"rootScope"`
	RootDependencies map[string]Identity            `json:"rootDependencies"`
	Packages         map[Identity]PackageDescriptor `json:"packages"`
}

type resolver struct {
	local            scope.LocalResolver
	rootDependencies map[string]Identity
	packages         map[Identity]PackageDescriptor
	packageSources   map[Identity]scope.Source
}

// Load adapts an already-resolved npm package graph to the Scope loader.
func Load(environment Environment) (*scope.Workspace, error) {
	root, err := scope.NewLocalSource(environment.RootScope)
	if err != nil {
		return nil, fmt.Errorf("load package environment root: %w", err)
	}
	resolve, err := newResolver(environment)
	if err != nil {
		return nil, err
	}
	return scope.Load(root, resolve)
}

func newResolver(environment Environment) (*resolver, error) {
	result := &resolver{
		rootDependencies: copyDependencies(environment.RootDependencies),
		packages:         make(map[Identity]PackageDescriptor, len(environment.Packages)),
		packageSources:   make(map[Identity]scope.Source, len(environment.Packages)),
	}

	identities := make([]Identity, 0, len(environment.Packages))
	for identity := range environment.Packages {
		identities = append(identities, identity)
	}
	sort.Slice(identities, func(i, j int) bool { return identities[i] < identities[j] })
	for _, identity := range identities {
		if strings.TrimSpace(string(identity)) == "" {
			return nil, fmt.Errorf("package identity must not be empty")
		}
		pkg := environment.Packages[identity]
		root, entry, err := secureEntry(pkg.Root, pkg.Entry)
		if err != nil {
			return nil, fmt.Errorf("package %q: %w", identity, err)
		}
		pkg.Root = root
		pkg.Entry = entry
		pkg.Dependencies = copyDependencies(pkg.Dependencies)
		source := scope.Source{Key: scope.ScopeKey(identity), LocalPath: filepath.Dir(filepath.Join(root, filepath.FromSlash(entry)))}
		if err := scope.CheckSource(source); err != nil {
			return nil, fmt.Errorf("package %q Scope source: %w", identity, err)
		}
		result.packages[identity] = pkg
		result.packageSources[identity] = source
	}

	if err := result.validateEdges("root", result.rootDependencies); err != nil {
		return nil, err
	}
	for _, identity := range identities {
		if err := result.validateEdges(fmt.Sprintf("package %q", identity), result.packages[identity].Dependencies); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (r *resolver) validateEdges(importer string, dependencies map[string]Identity) error {
	names := make([]string, 0, len(dependencies))
	for name := range dependencies {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if name == "" {
			return fmt.Errorf("%s dependency name must not be empty", importer)
		}
		identity := dependencies[name]
		if _, exists := r.packages[identity]; !exists {
			return fmt.Errorf("%s dependency %q targets missing package %q", importer, name, identity)
		}
	}
	return nil
}

func (r *resolver) Resolve(from scope.Source, reference string) (scope.Source, error) {
	pkg, distributed := r.packages[Identity(from.Key)]
	if isLocalImport(reference) {
		if distributed {
			return scope.Source{}, fmt.Errorf("distributed package %q cannot import local Scope %q", from.Key, reference)
		}
		return r.local.Resolve(from, reference)
	}

	// A package import is resolved only through the declaring package's edges.
	// Root dependencies are not a global fallback for distributed packages.
	dependencies := r.rootDependencies
	importer := "local root"
	if distributed {
		dependencies = pkg.Dependencies
		importer = fmt.Sprintf("package %q", from.Key)
	}
	identity, exists := dependencies[reference]
	if !exists {
		return scope.Source{}, fmt.Errorf("%s does not declare package import %q in the resolved environment", importer, reference)
	}
	target, exists := r.packageSources[identity]
	if !exists {
		return scope.Source{}, fmt.Errorf("%s package import %q targets unavailable package %q", importer, reference, identity)
	}
	return target, nil
}

func secureEntry(root, entry string) (string, string, error) {
	if root == "" || entry == "" || filepath.IsAbs(entry) || strings.Contains(entry, "\\") {
		return "", "", fmt.Errorf("entry %q must be a relative slash path", entry)
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return "", "", fmt.Errorf("resolve package root: %w", err)
	}
	canonicalRoot, err := filepath.EvalSymlinks(absoluteRoot)
	if err != nil {
		return "", "", fmt.Errorf("resolve package root: %w", err)
	}
	cleanEntry := filepath.ToSlash(filepath.Clean(filepath.FromSlash(entry)))
	if cleanEntry == "." || cleanEntry == ".." || strings.HasPrefix(cleanEntry, "../") || cleanEntry != entry {
		return "", "", fmt.Errorf("entry %q must be normalized and remain inside the package root", entry)
	}
	switch filepath.Base(filepath.FromSlash(cleanEntry)) {
	case "locus.yaml", "locus.yml", "locus.json":
	default:
		return "", "", fmt.Errorf("entry %q must name a Scope manifest", entry)
	}
	candidate, err := filepath.EvalSymlinks(filepath.Join(canonicalRoot, filepath.FromSlash(cleanEntry)))
	if err != nil {
		return "", "", fmt.Errorf("resolve entry %q: %w", entry, err)
	}
	relative, err := filepath.Rel(canonicalRoot, candidate)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return "", "", fmt.Errorf("entry %q escapes package root", entry)
	}
	info, err := os.Stat(candidate)
	if err != nil {
		return "", "", fmt.Errorf("inspect entry %q: %w", entry, err)
	}
	if !info.Mode().IsRegular() {
		return "", "", fmt.Errorf("entry %q is not a readable file", entry)
	}
	return filepath.Clean(canonicalRoot), cleanEntry, nil
}

func isLocalImport(reference string) bool {
	return strings.HasPrefix(reference, "./") || strings.HasPrefix(reference, "../") || filepath.IsAbs(reference)
}

func copyDependencies(source map[string]Identity) map[string]Identity {
	result := make(map[string]Identity, len(source))
	for name, identity := range source {
		result[name] = identity
	}
	return result
}
