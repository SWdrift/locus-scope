package packageenv

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"locus-scope/internal/scope"
)

// Identity is the semantic identity of one npm package Scope.
type Identity string

// Package describes one Locus package made available by a package environment.
type Package struct {
	Identity     Identity
	Root         string
	Entry        string
	Dependencies map[string]Identity
}

// Environment describes the importer-relative package graph visible to a root Scope.
type Environment struct {
	RootDependencies map[string]Identity
	Packages         map[Identity]Package
}

type resolver struct {
	local            scope.LocalResolver
	rootDependencies map[string]Identity
	packages         map[Identity]Package
	packageSources   map[Identity]scope.Source
}

// Load adapts a resolved package graph to the Scope loader.
func Load(rootDirectory string, environment Environment) (*scope.Workspace, error) {
	root, err := scope.NewLocalSource(rootDirectory)
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
		packages:         make(map[Identity]Package, len(environment.Packages)),
		packageSources:   make(map[Identity]scope.Source, len(environment.Packages)),
	}

	identities := make([]Identity, 0, len(environment.Packages))
	for identity := range environment.Packages {
		identities = append(identities, identity)
	}
	sort.Slice(identities, func(i, j int) bool { return identities[i] < identities[j] })
	for _, identity := range identities {
		pkg := environment.Packages[identity]
		if pkg.Identity != identity {
			return nil, fmt.Errorf("package map key %q does not match package identity %q", identity, pkg.Identity)
		}
		if _, _, err := parseIdentity(identity); err != nil {
			return nil, err
		}
		root, entry, err := secureEntry(pkg.Root, pkg.Entry)
		if err != nil {
			return nil, fmt.Errorf("package %q: %w", identity, err)
		}
		pkg.Root = root
		pkg.Entry = entry
		pkg.Dependencies = copyDependencies(pkg.Dependencies)
		source := scope.Source{Key: scope.ScopeKey(identity), LocalPath: filepath.Dir(filepath.Join(root, filepath.FromSlash(entry)))}
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
		if err := validatePackageName(name); err != nil {
			return fmt.Errorf("%s dependency %q: %w", importer, name, err)
		}
		identity := dependencies[name]
		identityName, _, err := parseIdentity(identity)
		if err != nil {
			return fmt.Errorf("%s dependency %q: %w", importer, name, err)
		}
		if identityName != name {
			return fmt.Errorf("%s dependency %q targets identity %q for package %q", importer, name, identity, identityName)
		}
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
	if err := validatePackageName(reference); err != nil {
		return scope.Source{}, fmt.Errorf("invalid package import %q: bare imports must be npm package names without versions or subpaths", reference)
	}

	dependencies := r.rootDependencies
	importer := "local root"
	if distributed {
		dependencies = pkg.Dependencies
		importer = fmt.Sprintf("package %q", pkg.Identity)
	}
	identity, exists := dependencies[reference]
	if !exists {
		if distributed {
			return scope.Source{}, fmt.Errorf("%s does not declare package import %q in dependencies", importer, reference)
		}
		return scope.Source{}, fmt.Errorf("local root has no installed package edge for %q; run locus-pkg install", reference)
	}
	target, exists := r.packageSources[identity]
	if !exists {
		return scope.Source{}, fmt.Errorf("%s package import %q targets unavailable package %q", importer, reference, identity)
	}
	return target, nil
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
