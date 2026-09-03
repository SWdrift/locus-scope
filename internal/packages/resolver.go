package packages

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"locus-scope/internal/scope"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/retry"
)

type packageSource struct {
	digest      packageReference
	packageRoot string
	relative    string
}

type resolverBase struct {
	rootDirectory  string
	local          scope.LocalResolver
	packageSources map[scope.ScopeKey]packageSource
	requestSources map[string]scope.Source
}

func newResolverBase(rootDirectory string) resolverBase {
	return resolverBase{
		rootDirectory:  rootDirectory,
		packageSources: make(map[scope.ScopeKey]packageSource),
		requestSources: make(map[string]scope.Source),
	}
}

func (r *resolverBase) resolveLocalOrPackage(from scope.Source, reference string) (scope.Source, error) {
	metadata, packaged := r.packageSources[from.Key]
	if !packaged {
		return r.local.Resolve(from, reference)
	}
	if filepath.IsAbs(reference) || filepath.VolumeName(reference) != "" || strings.HasPrefix(reference, "/") || strings.Contains(reference, `\`) {
		return scope.Source{}, fmt.Errorf("package scope %q must not import absolute or platform-specific path %q", from.Key, reference)
	}
	relative := path.Clean(path.Join(metadata.relative, reference))
	if relative == ".." || strings.HasPrefix(relative, "../") || path.IsAbs(relative) {
		return scope.Source{}, fmt.Errorf("package-relative import %q from %q escapes package root", reference, from.Key)
	}

	target, err := scope.NewLocalSource(filepath.Join(metadata.packageRoot, filepath.FromSlash(relative)))
	if err != nil {
		return scope.Source{}, err
	}
	checkedRelative, err := filepath.Rel(metadata.packageRoot, target.LocalPath)
	if err != nil {
		return scope.Source{}, fmt.Errorf("resolve package-relative import %q from %q: %w", reference, from.Key, err)
	}
	if checkedRelative == ".." || strings.HasPrefix(checkedRelative, ".."+string(filepath.Separator)) || filepath.IsAbs(checkedRelative) {
		return scope.Source{}, fmt.Errorf("package-relative import %q from %q escapes package root", reference, from.Key)
	}

	key := scope.ScopeKey(metadata.digest.Canonical)
	if relative != "." {
		escaped := (&url.URL{Path: "/" + relative}).EscapedPath()
		key = scope.ScopeKey(metadata.digest.Canonical + "#" + escaped)
	}
	if previous, exists := r.packageSources[key]; exists && previous.packageRoot != metadata.packageRoot {
		return scope.Source{}, fmt.Errorf("package source key %q maps to both %q and %q", key, previous.packageRoot, metadata.packageRoot)
	}
	r.packageSources[key] = packageSource{digest: metadata.digest, packageRoot: metadata.packageRoot, relative: relative}
	return scope.Source{Key: key, LocalPath: target.LocalPath}, nil
}

func (r *resolverBase) packageRootSource(reference packageReference, path string) (scope.Source, error) {
	key := scope.ScopeKey(reference.Canonical)
	if previous, exists := r.packageSources[key]; exists && previous.packageRoot != path {
		return scope.Source{}, fmt.Errorf("package source key %q maps to both %q and %q", key, previous.packageRoot, path)
	}
	r.packageSources[key] = packageSource{digest: reference, packageRoot: path, relative: "."}
	return scope.Source{Key: key, LocalPath: path}, nil
}

type installingResolver struct {
	resolverBase
	ctx             context.Context
	options         InstallOptions
	lock            Lock
	reachable       map[string]LockedPackage
	resolvedDigests map[string]packageReference
	resolved        int
	reused          int
	fetched         int
	materialized    int
}

func newInstallingResolver(ctx context.Context, rootDirectory string, options InstallOptions, lock Lock) *installingResolver {
	return &installingResolver{
		resolverBase:    newResolverBase(rootDirectory),
		ctx:             ctx,
		options:         options,
		lock:            lock,
		reachable:       make(map[string]LockedPackage),
		resolvedDigests: make(map[string]packageReference),
	}
}

func (r *installingResolver) Resolve(from scope.Source, reference string) (scope.Source, error) {
	if !strings.Contains(reference, "://") {
		return r.resolveLocalOrPackage(from, reference)
	}
	requested, err := parsePackageReference(reference)
	if err != nil {
		return scope.Source{}, err
	}
	if source, exists := r.requestSources[requested.Canonical]; exists {
		return source, nil
	}

	resolved, err := r.resolveDigest(requested)
	if err != nil {
		return scope.Source{}, err
	}
	source, err := r.preparePackage(resolved)
	if err != nil {
		return scope.Source{}, err
	}
	r.requestSources[requested.Canonical] = source
	return source, nil
}

func (r *installingResolver) resolveDigest(requested packageReference) (packageReference, error) {
	if !requested.Mutable {
		return requested, nil
	}
	if known, exists := r.resolvedDigests[requested.Canonical]; exists {
		return known, nil
	}

	var resolved packageReference
	locked, exists := r.lock.Packages[requested.Canonical]
	if exists {
		var err error
		resolved, err = parsePackageReference(locked.Resolved)
		if err != nil {
			return packageReference{}, fmt.Errorf("resolve locked package %q: %w", requested.Canonical, err)
		}
		r.reused++
	} else {
		if r.options.Frozen {
			return packageReference{}, fmt.Errorf("frozen lock is missing %q", requested.Canonical)
		}
		repository, err := newRemoteRepository(requested, r.options.Credential)
		if err != nil {
			return packageReference{}, err
		}
		descriptor, err := repository.Resolve(r.ctx, requested.Reference)
		if err != nil {
			return packageReference{}, fmt.Errorf("resolve package %q: %w", requested.Canonical, err)
		}
		resolved, err = digestReference(requested.Registry, requested.Repository, descriptor.Digest.String())
		if err != nil {
			return packageReference{}, fmt.Errorf("validate resolved package %q: %w", requested.Canonical, err)
		}
		r.resolved++
	}
	if resolved.Mutable || resolved.Registry != requested.Registry || resolved.Repository != requested.Repository {
		return packageReference{}, fmt.Errorf("package %q resolved outside its registry or repository to %q", requested.Canonical, resolved.Canonical)
	}
	r.resolvedDigests[requested.Canonical] = resolved
	r.reachable[requested.Canonical] = LockedPackage{Resolved: resolved.Canonical}
	return resolved, nil
}

func (r *installingResolver) preparePackage(resolved packageReference) (scope.Source, error) {
	target, err := packageDirectory(r.rootDirectory, resolved)
	if err != nil {
		return scope.Source{}, err
	}
	if _, err := os.Stat(target); err == nil {
		source, err := r.packageRootSource(resolved, target)
		if err != nil {
			return scope.Source{}, err
		}
		if err := scope.CheckSource(source); err != nil {
			return scope.Source{}, fmt.Errorf("existing materialized package %q is invalid: %w", resolved.Canonical, err)
		}
		return source, nil
	} else if !os.IsNotExist(err) {
		return scope.Source{}, fmt.Errorf("inspect materialized package %q: %w", resolved.Canonical, err)
	}

	repository, err := newRemoteRepository(resolved, r.options.Credential)
	if err != nil {
		return scope.Source{}, err
	}
	cached, fetched, err := ensureCached(r.ctx, r.options.CacheRoot, resolved, repository)
	if err != nil {
		return scope.Source{}, err
	}
	if fetched {
		r.fetched++
	}
	target, materialized, err := materializePackage(r.ctx, r.rootDirectory, resolved, cached)
	if err != nil {
		return scope.Source{}, err
	}
	if materialized {
		r.materialized++
	}
	return r.packageRootSource(resolved, target)
}

type offlineResolver struct {
	resolverBase
	lock Lock
}

func newOfflineResolver(rootDirectory string, lock Lock) *offlineResolver {
	return &offlineResolver{resolverBase: newResolverBase(rootDirectory), lock: lock}
}

func (r *offlineResolver) Resolve(from scope.Source, reference string) (scope.Source, error) {
	if !strings.Contains(reference, "://") {
		source, err := r.resolveLocalOrPackage(from, reference)
		if err != nil && r.packageSources[from.Key].packageRoot != "" {
			return scope.Source{}, installRequired("resolve package-relative import %q from %q: %v", reference, from.Key, err)
		}
		return source, err
	}
	requested, err := parsePackageReference(reference)
	if err != nil {
		return scope.Source{}, err
	}
	if source, exists := r.requestSources[requested.Canonical]; exists {
		return source, nil
	}
	resolved := requested
	if requested.Mutable {
		locked, exists := r.lock.Packages[requested.Canonical]
		if !exists {
			return scope.Source{}, installRequired("lock entry for package %q is missing", requested.Canonical)
		}
		resolved, err = parsePackageReference(locked.Resolved)
		if err != nil {
			return scope.Source{}, installRequired("lock entry for package %q is invalid: %v", requested.Canonical, err)
		}
	}
	target, err := packageDirectory(r.rootDirectory, resolved)
	if err != nil {
		return scope.Source{}, installRequired("resolve package %q: %v", resolved.Canonical, err)
	}
	source, err := r.packageRootSource(resolved, target)
	if err != nil {
		return scope.Source{}, err
	}
	if err := scope.CheckSource(source); err != nil {
		return scope.Source{}, installRequired("materialized package %q is unavailable: %v", resolved.Canonical, err)
	}
	r.requestSources[requested.Canonical] = source
	return source, nil
}

func newRemoteRepository(reference packageReference, credential auth.CredentialFunc) (*remote.Repository, error) {
	repository, err := remote.NewRepository(reference.Registry + "/" + reference.Repository)
	if err != nil {
		return nil, fmt.Errorf("create repository client for %q: %w", reference.Canonical, err)
	}
	repository.Client = &auth.Client{Client: retry.DefaultClient, Cache: auth.NewCache(), Credential: credential}
	repository.PlainHTTP = isLoopbackRegistry(reference.Registry)
	return repository, nil
}

func installRequired(format string, arguments ...any) error {
	return fmt.Errorf(format+"; run locus-pkg install", arguments...)
}
