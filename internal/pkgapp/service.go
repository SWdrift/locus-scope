// Package pkgapp exposes transport-independent Locus Package operations.
package pkgapp

import (
	"context"

	"locus-scope/internal/pkg"
	"locus-scope/internal/scope"
)

// Options controls package resolution and persistence.
type Options struct {
	Registry       string
	FrozenLockfile bool
	Offline        bool
}

// Service operates on one Locus project or package root.
type Service struct {
	root    string
	options Options
}

// InstallResult describes the committed package environment.
type InstallResult struct {
	Valid     bool           `json:"valid"`
	Root      scope.ScopeKey `json:"root"`
	Added     []string       `json:"added"`
	Removed   []string       `json:"removed"`
	Updated   []string       `json:"updated"`
	Reused    int            `json:"reused"`
	Fetched   int            `json:"fetched"`
	Installed int            `json:"installed"`
	Packages  int            `json:"packages"`
	Scopes    int            `json:"scopes"`
	Entities  int            `json:"entities"`
	Relations int            `json:"relations"`
}

// ListDependency is one importer-relative node in ListResult.
type ListDependency struct {
	Name         string           `json:"name"`
	Version      string           `json:"version"`
	Identity     string           `json:"identity"`
	Dependencies []ListDependency `json:"dependencies"`
}

// ListResult describes the direct dependency trees recorded by the lock.
type ListResult struct {
	Root         string           `json:"root"`
	Dependencies []ListDependency `json:"dependencies"`
}

// PackResult describes a deterministic npm-compatible tarball.
type PackResult struct {
	Name      string   `json:"name"`
	Version   string   `json:"version"`
	Filename  string   `json:"filename"`
	Integrity string   `json:"integrity"`
	Files     []string `json:"files"`
}

// PublishResult describes a successfully published immutable package version.
type PublishResult struct {
	Name      string `json:"name"`
	Version   string `json:"version"`
	Registry  string `json:"registry"`
	Integrity string `json:"integrity"`
}

// New creates an application service for root.
func New(root string, options Options) *Service {
	return &Service{root: root, options: options}
}

// Install resolves and installs package specs.
func (s *Service) Install(ctx context.Context, specs []string) (InstallResult, error) {
	result, err := pkg.Install(ctx, s.root, specs, s.pkgOptions())
	return makeInstallResult(result), err
}

// Uninstall removes direct dependencies.
func (s *Service) Uninstall(ctx context.Context, names []string) (InstallResult, error) {
	result, err := pkg.Uninstall(ctx, s.root, names, s.pkgOptions())
	return makeInstallResult(result), err
}

// Update re-resolves all or selected direct dependencies.
func (s *Service) Update(ctx context.Context, names []string) (InstallResult, error) {
	result, err := pkg.Update(ctx, s.root, names, s.pkgOptions())
	return makeInstallResult(result), err
}

// List returns importer-relative dependency trees.
func (s *Service) List() (ListResult, error) {
	result, err := pkg.List(s.root)
	if err != nil {
		return ListResult{}, err
	}
	return ListResult{Root: result.Root, Dependencies: makeDependencies(result.Dependencies)}, nil
}

// Pack writes a deterministic npm-compatible tarball.
func (s *Service) Pack() (PackResult, error) {
	result, err := pkg.Pack(s.root)
	return PackResult(result), err
}

// Publish packs and publishes the current immutable package version.
func (s *Service) Publish(ctx context.Context) (PublishResult, error) {
	result, err := pkg.Publish(ctx, s.root, s.pkgOptions())
	return PublishResult(result), err
}

// LoadWorkspace assembles the installed Scope workspace without network access.
func (s *Service) LoadWorkspace() (*scope.Workspace, error) {
	return pkg.LoadWorkspace(s.root)
}

func (s *Service) pkgOptions() pkg.Options {
	return pkg.Options{
		Registry:       s.options.Registry,
		FrozenLockfile: s.options.FrozenLockfile,
		Offline:        s.options.Offline,
	}
}

func makeInstallResult(result pkg.InstallResult) InstallResult {
	return InstallResult(result)
}

func makeDependencies(source []pkg.ListDependency) []ListDependency {
	result := make([]ListDependency, len(source))
	for index, dependency := range source {
		result[index] = ListDependency{
			Name: dependency.Name, Version: dependency.Version, Identity: dependency.Identity,
			Dependencies: makeDependencies(dependency.Dependencies),
		}
	}
	return result
}
