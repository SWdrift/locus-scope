package pkg

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"deps.dev/util/resolve"
	"deps.dev/util/resolve/dep"
	"github.com/natefinch/atomic"

	locusnpm "locus-scope/internal/npm"
	"locus-scope/internal/packageenv"
	"locus-scope/internal/scope"
)

type mutationKind byte

const (
	installMutation mutationKind = iota
	uninstallMutation
	updateMutation
)

type requestedDependency struct {
	bare bool
}

// Install resolves and installs package specs. With no specs it synchronizes
// the declared package.json dependencies.
func Install(ctx context.Context, root string, specs []string, options Options) (InstallResult, error) {
	return mutate(ctx, root, installMutation, specs, options)
}

// Uninstall removes direct dependencies and their now-unreachable lock nodes.
func Uninstall(ctx context.Context, root string, names []string, options Options) (InstallResult, error) {
	if len(names) == 0 {
		return InstallResult{}, fmt.Errorf("uninstall requires at least one package name")
	}
	return mutate(ctx, root, uninstallMutation, names, options)
}

// Update re-resolves all direct dependencies, or only the named root closures.
func Update(ctx context.Context, root string, names []string, options Options) (InstallResult, error) {
	return mutate(ctx, root, updateMutation, names, options)
}

type mutationPlan struct {
	desired       map[string]string
	unlock        map[string]bool
	requested     map[string]requestedDependency
	writeManifest bool
}

func prepareMutation(kind mutationKind, arguments []string, options Options, dependencies map[string]string, lock lockFile) (mutationPlan, error) {
	plan := mutationPlan{
		desired:   make(map[string]string, len(dependencies)),
		unlock:    map[string]bool{},
		requested: map[string]requestedDependency{},
	}
	for name, constraint := range dependencies {
		plan.desired[name] = constraint
	}
	switch kind {
	case installMutation:
		if len(arguments) != 0 && options.FrozenLockfile {
			return mutationPlan{}, fmt.Errorf("explicit install cannot be used with frozen lockfile")
		}
		plan.writeManifest = len(arguments) != 0
		for _, value := range arguments {
			spec, err := locusnpm.ParsePackageSpec(value)
			if err != nil {
				return mutationPlan{}, err
			}
			if _, duplicate := plan.requested[spec.Name]; duplicate {
				return mutationPlan{}, fmt.Errorf("package %s was specified more than once", spec.Name)
			}
			plan.requested[spec.Name] = requestedDependency{bare: value == spec.Name}
			plan.desired[spec.Name] = spec.Constraint
		}
		for name, constraint := range plan.desired {
			old, exists := lock.Importers["."].Dependencies[name]
			if !exists || old.Specifier != constraint {
				plan.unlock[name] = true
			}
		}
	case uninstallMutation:
		if options.FrozenLockfile {
			return mutationPlan{}, fmt.Errorf("uninstall cannot be used with frozen lockfile")
		}
		if !lockMatchesDependencies(lock, dependencies) {
			return mutationPlan{}, fmt.Errorf("locus.lock does not match package.json; run locus-pkg install")
		}
		plan.writeManifest = true
		seen := map[string]bool{}
		for _, value := range arguments {
			name, err := locusnpm.ParsePackageName(value)
			if err != nil || name != value {
				return mutationPlan{}, fmt.Errorf("invalid direct dependency name %q", value)
			}
			if seen[name] {
				return mutationPlan{}, fmt.Errorf("package %s was specified more than once", name)
			}
			seen[name] = true
			if _, exists := plan.desired[name]; !exists {
				return mutationPlan{}, fmt.Errorf("package %s is not a direct dependency", name)
			}
			delete(plan.desired, name)
		}
	case updateMutation:
		if options.FrozenLockfile {
			return mutationPlan{}, fmt.Errorf("update cannot be used with frozen lockfile")
		}
		if !lockMatchesDependencies(lock, dependencies) {
			return mutationPlan{}, fmt.Errorf("locus.lock does not match package.json; run locus-pkg install")
		}
		if len(arguments) == 0 {
			for name := range plan.desired {
				plan.unlock[name] = true
			}
			break
		}
		for _, value := range arguments {
			name, err := locusnpm.ParsePackageName(value)
			if err != nil || name != value {
				return mutationPlan{}, fmt.Errorf("invalid direct dependency name %q", value)
			}
			if _, exists := plan.desired[name]; !exists {
				return mutationPlan{}, fmt.Errorf("package %s is not a direct dependency", name)
			}
			if plan.unlock[name] {
				return mutationPlan{}, fmt.Errorf("package %s was specified more than once", name)
			}
			plan.unlock[name] = true
		}
	}
	return plan, nil
}

func mutate(ctx context.Context, root string, kind mutationKind, arguments []string, options Options) (InstallResult, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return InstallResult{}, fmt.Errorf("resolve project root: %w", err)
	}
	manifest, originalManifest, err := readProjectManifest(root)
	if err != nil {
		return InstallResult{}, err
	}
	lock, originalLock, err := readLock(root, options.FrozenLockfile || options.Offline)
	if err != nil {
		return InstallResult{}, err
	}

	plan, err := prepareMutation(kind, arguments, options, manifest.dependencies, lock)
	if err != nil {
		return InstallResult{}, err
	}
	desired := plan.desired
	unlock := plan.unlock
	requested := plan.requested
	writeManifest := plan.writeManifest

	compatible := lockMatchesDependencies(lock, desired)
	if options.FrozenLockfile && !compatible {
		return InstallResult{}, fmt.Errorf("frozen lockfile does not match package.json dependencies")
	}
	newLock := lock
	var client *locusnpm.Client
	needsGraphResolution := !compatible || len(unlock) != 0
	if options.Offline {
		if len(unlock) != 0 && kind == updateMutation {
			return InstallResult{}, fmt.Errorf("update cannot resolve unlocked dependencies offline")
		}
		if !compatible {
			newLock, err = resolveOfflineLock(desired, lock)
			if err != nil {
				return InstallResult{}, err
			}
		}
	} else if needsGraphResolution {
		if len(unlock) != 0 {
			config, configErr := locusnpm.LoadConfig(root, options.Registry)
			if configErr != nil {
				return InstallResult{}, configErr
			}
			client = locusnpm.NewClient(config, http.DefaultClient)
		}
		newLock, err = resolveLock(ctx, desired, unlock, lock, client)
		if err != nil {
			return InstallResult{}, err
		}
	}
	if needsGraphResolution {
		normalizeBareSpecifiers(desired, requested, &newLock)
		if err := validateLock(newLock); err != nil {
			return InstallResult{}, fmt.Errorf("resolved package graph is invalid: %w", err)
		}
	}
	effectiveOffline := options.Offline || options.FrozenLockfile
	if client == nil && !effectiveOffline && len(newLock.Packages) != 0 {
		config, configErr := locusnpm.LoadConfig(root, options.Registry)
		if configErr != nil {
			return InstallResult{}, configErr
		}
		client = locusnpm.NewClient(config, http.DefaultClient)
	}

	materialized, err := materialize(ctx, root, newLock, client, effectiveOffline)
	if err != nil {
		return InstallResult{}, err
	}
	defer materialized.close()
	workspace, err := loadEnvironment(root, newLock, materialized.packages)
	if err != nil {
		return InstallResult{}, err
	}
	if err := materialized.publish(); err != nil {
		materialized.rollback()
		return InstallResult{}, err
	}
	if !options.FrozenLockfile {
		if err := stagePrune(root, newLock, materialized); err != nil {
			materialized.rollback()
			return InstallResult{}, err
		}
	}

	var manifestData []byte
	if writeManifest {
		manifest = manifest.withDependencies(desired)
		manifestData, err = encodeProjectManifest(manifest)
		if err != nil {
			materialized.rollback()
			return InstallResult{}, err
		}
	}
	lockData, err := encodeLock(newLock)
	if err != nil {
		materialized.rollback()
		return InstallResult{}, err
	}
	if !options.FrozenLockfile {
		if err := commitProjectFiles(root, manifestData, writeManifest, lockData, originalManifest, originalLock); err != nil {
			materialized.rollback()
			return InstallResult{}, err
		}
	}
	materialized.close()
	packageChanges := graphChanges(lock, newLock)
	scopes, entities, relations := workspaceCounts(workspace)
	return InstallResult{
		Valid:     true,
		Root:      workspace.Root,
		Added:     packageChanges.added,
		Removed:   packageChanges.removed,
		Updated:   packageChanges.updated,
		Reused:    materialized.reused,
		Fetched:   materialized.fetched,
		Installed: materialized.installed,
		Packages:  len(newLock.Packages),
		Scopes:    scopes,
		Entities:  entities,
		Relations: relations,
	}, nil
}

func lockMatchesDependencies(lock lockFile, dependencies map[string]string) bool {
	root, ok := lock.Importers["."]
	if !ok || len(root.Dependencies) != len(dependencies) {
		return false
	}
	for name, constraint := range dependencies {
		edge, exists := root.Dependencies[name]
		if !exists || edge.Specifier != constraint {
			return false
		}
	}
	return true
}

func resolveOfflineLock(desired map[string]string, old lockFile) (lockFile, error) {
	resolved := emptyLock()
	root := resolved.Importers["."]
	for _, name := range sortedNames(desired) {
		edge, ok := old.Importers["."].Dependencies[name]
		if !ok {
			return lockFile{}, fmt.Errorf("offline package %s is not present in the compatible lockfile", name)
		}
		_, version, _ := locusnpm.ParseIdentity(edge.Package)
		matches, err := locusnpm.Matches(version, desired[name])
		if err != nil || !matches {
			return lockFile{}, fmt.Errorf("offline locked package %s does not satisfy %q", edge.Package, desired[name])
		}
		edge.Specifier = desired[name]
		root.Dependencies[name] = edge
		if err := copyLockedClosure(edge.Package, old.Packages, resolved.Packages); err != nil {
			return lockFile{}, err
		}
	}
	resolved.Importers["."] = root
	return resolved, nil
}

func normalizeBareSpecifiers(desired map[string]string, requested map[string]requestedDependency, lock *lockFile) {
	root := lock.Importers["."]
	for name, request := range requested {
		if !request.bare {
			continue
		}
		_, version, _ := locusnpm.ParseIdentity(root.Dependencies[name].Package)
		desired[name] = "^" + version
		edge := root.Dependencies[name]
		edge.Specifier = desired[name]
		root.Dependencies[name] = edge
	}
	lock.Importers["."] = root
}

func resolveLock(ctx context.Context, desired map[string]string, unlock map[string]bool, old lockFile, client *locusnpm.Client) (lockFile, error) {
	resolved := emptyLock()
	root := resolved.Importers["."]
	for _, name := range sortedNames(desired) {
		constraint := desired[name]
		if !unlock[name] {
			if oldEdge, ok := old.Importers["."].Dependencies[name]; ok && oldEdge.Specifier == constraint {
				root.Dependencies[name] = oldEdge
				if err := copyLockedClosure(oldEdge.Package, old.Packages, resolved.Packages); err != nil {
					return lockFile{}, err
				}
				continue
			}
		}
		packument, err := client.Packument(ctx, name)
		if err != nil {
			return lockFile{}, fmt.Errorf("resolve %s: %w", name, err)
		}
		versions := make([]string, 0, len(packument.Versions))
		for version := range packument.Versions {
			versions = append(versions, version)
		}
		version, err := locusnpm.HighestMatching(versions, constraint)
		if err != nil {
			return lockFile{}, fmt.Errorf("resolve %s@%s: %w", name, constraint, err)
		}
		identity, _ := locusnpm.Identity(name, version)
		root.Dependencies[name] = lockEdge{Specifier: constraint, Package: identity}
		if err := resolveClosure(ctx, client, name, version, resolved.Packages); err != nil {
			return lockFile{}, err
		}
	}
	resolved.Importers["."] = root
	if err := validateLock(resolved); err != nil {
		return lockFile{}, fmt.Errorf("resolved package graph is invalid: %w", err)
	}
	return resolved, nil
}

func resolveClosure(ctx context.Context, client *locusnpm.Client, name, version string, packages map[string]lockPackage) error {
	resolver := locusnpm.NewResolver(client)
	graph, err := resolver.Resolve(ctx, resolve.VersionKey{
		PackageKey:  resolve.PackageKey{System: resolve.NPM, Name: name},
		VersionType: resolve.Concrete,
		Version:     version,
	})
	if err != nil {
		return fmt.Errorf("resolve dependency graph for %s@%s: %w", name, version, err)
	}
	if graph.Error != "" {
		return fmt.Errorf("resolve dependency graph for %s@%s: %s", name, version, graph.Error)
	}
	outgoing := make(map[resolve.NodeID][]resolve.Edge, len(graph.Nodes))
	for _, edge := range graph.Edges {
		if !edge.Type.IsRegular() && !edge.Type.Equal(dep.NewType(dep.Selector)) {
			return fmt.Errorf("resolved graph contains unsupported %s dependency", edge.Type)
		}
		outgoing[edge.From] = append(outgoing[edge.From], edge)
	}
	for nodeID, node := range graph.Nodes {
		if len(node.Errors) != 0 {
			return fmt.Errorf("resolved graph node %s has errors: %v", node.Version, node.Errors)
		}
		if node.Version.System != resolve.NPM || node.Version.VersionType != resolve.Concrete {
			return fmt.Errorf("resolved graph contains non-concrete npm node %s", node.Version)
		}
		metadata, err := client.VersionMetadata(ctx, node.Version.Name, node.Version.Version)
		if err != nil {
			return fmt.Errorf("read metadata for %s@%s: %w", node.Version.Name, node.Version.Version, err)
		}
		identity, _ := locusnpm.Identity(node.Version.Name, node.Version.Version)
		registry, err := client.RegistryFor(node.Version.Name)
		if err != nil {
			return err
		}
		record := lockPackage{
			Registry:     registry.String(),
			Resolved:     metadata.Dist.Tarball,
			Integrity:    metadata.Dist.Integrity,
			Dependencies: map[string]lockEdge{},
		}
		for _, edge := range outgoing[resolve.NodeID(nodeID)] {
			target := graph.Nodes[edge.To].Version
			targetIdentity, identityErr := locusnpm.Identity(target.Name, target.Version)
			if identityErr != nil {
				return identityErr
			}
			if _, duplicate := record.Dependencies[target.Name]; duplicate {
				return fmt.Errorf("package %s has duplicate dependency %s", identity, target.Name)
			}
			record.Dependencies[target.Name] = lockEdge{Specifier: edge.Requirement, Package: targetIdentity}
		}
		if len(record.Dependencies) != len(metadata.Dependencies) {
			return fmt.Errorf("resolved edges for %s differ from declared dependencies", identity)
		}
		for dependencyName, constraint := range metadata.Dependencies {
			if edge, ok := record.Dependencies[dependencyName]; !ok || edge.Specifier != constraint {
				return fmt.Errorf("resolved edge %s of %s differs from declared dependency", dependencyName, identity)
			}
		}
		if err := mergeLockedPackage(identity, record, packages); err != nil {
			return err
		}
	}
	return nil
}

func copyLockedClosure(identity string, source, destination map[string]lockPackage) error {
	if existing, ok := destination[identity]; ok {
		return mergeLockedPackage(identity, source[identity], map[string]lockPackage{identity: existing})
	}
	record, ok := source[identity]
	if !ok {
		return fmt.Errorf("locked package %s is missing", identity)
	}
	if err := mergeLockedPackage(identity, record, destination); err != nil {
		return err
	}
	for _, edge := range record.Dependencies {
		if err := copyLockedClosure(edge.Package, source, destination); err != nil {
			return err
		}
	}
	return nil
}

func mergeLockedPackage(identity string, record lockPackage, packages map[string]lockPackage) error {
	if existing, ok := packages[identity]; ok {
		if !equalLockedPackage(existing, record) {
			return fmt.Errorf("conflicting package environment for semantic identity %s", identity)
		}
		return nil
	}
	packages[identity] = record
	return nil
}

func equalLockedPackage(left, right lockPackage) bool {
	if left.Registry != right.Registry || left.Resolved != right.Resolved || left.Integrity != right.Integrity || len(left.Dependencies) != len(right.Dependencies) {
		return false
	}
	for name, edge := range left.Dependencies {
		if right.Dependencies[name] != edge {
			return false
		}
	}
	return true
}

type dependencyChanges struct {
	added   []string
	removed []string
	updated []string
}

func graphChanges(old, current lockFile) dependencyChanges {
	changes := dependencyChanges{added: []string{}, removed: []string{}, updated: []string{}}
	for identity, record := range current.Packages {
		oldRecord, existed := old.Packages[identity]
		if !existed {
			changes.added = append(changes.added, identity)
		} else if !equalLockedPackage(oldRecord, record) {
			changes.updated = append(changes.updated, identity)
		}
	}
	for identity := range old.Packages {
		if _, exists := current.Packages[identity]; !exists {
			changes.removed = append(changes.removed, identity)
		}
	}
	sort.Strings(changes.added)
	sort.Strings(changes.removed)
	sort.Strings(changes.updated)
	return changes
}

func commitProjectFiles(root string, manifestData []byte, writeManifest bool, lockData, oldManifest, oldLock []byte) error {
	manifestPath := filepath.Join(root, "package.json")
	lockPath := filepath.Join(root, "locus.lock")
	if writeManifest && !bytes.Equal(manifestData, oldManifest) {
		if err := atomic.WriteFile(manifestPath, bytes.NewReader(manifestData)); err != nil {
			return fmt.Errorf("write package.json: %w", err)
		}
	}
	if bytes.Equal(lockData, oldLock) {
		return nil
	}
	if err := atomic.WriteFile(lockPath, bytes.NewReader(lockData)); err != nil {
		writeErr := fmt.Errorf("write locus.lock: %w", err)
		var rollbackErrors []error
		if writeManifest {
			if restoreErr := atomic.WriteFile(manifestPath, bytes.NewReader(oldManifest)); restoreErr != nil {
				rollbackErrors = append(rollbackErrors, fmt.Errorf("restore package.json: %w", restoreErr))
			}
		}
		if oldLock == nil {
			if removeErr := os.Remove(lockPath); removeErr != nil && !os.IsNotExist(removeErr) {
				rollbackErrors = append(rollbackErrors, fmt.Errorf("remove uncommitted locus.lock: %w", removeErr))
			}
		} else if restoreErr := atomic.WriteFile(lockPath, bytes.NewReader(oldLock)); restoreErr != nil {
			rollbackErrors = append(rollbackErrors, fmt.Errorf("restore locus.lock: %w", restoreErr))
		}
		if len(rollbackErrors) != 0 {
			return errors.Join(writeErr, fmt.Errorf("project file rollback incomplete: %w", errors.Join(rollbackErrors...)))
		}
		return writeErr
	}
	return nil
}

func stagePrune(root string, lock lockFile, materialized *materialization) error {
	directory := filepath.Join(root, ".locus", "packages")
	entries, err := os.ReadDir(directory)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect package store for pruning: %w", err)
	}
	retained := map[string]bool{}
	for _, record := range lock.Packages {
		integrity, _ := locusnpm.ParseIntegrity(record.Integrity)
		retained[integrity.Algorithm()+"-"+integrity.Hex()] = true
	}
	prunedRoot := filepath.Join(materialized.operationRoot, "pruned")
	materialized.prunedStores = map[string]string{}
	for _, entry := range entries {
		if !entry.IsDir() || retained[entry.Name()] || !isStoreDirectoryName(entry.Name()) {
			continue
		}
		if err := os.MkdirAll(prunedRoot, 0o755); err != nil {
			return err
		}
		original := filepath.Join(directory, entry.Name())
		staged := filepath.Join(prunedRoot, entry.Name())
		if err := os.Rename(original, staged); err != nil {
			return fmt.Errorf("stage stale package store %s for pruning: %w", entry.Name(), err)
		}
		materialized.prunedStores[original] = staged
	}
	return nil
}

func isStoreDirectoryName(name string) bool {
	algorithm, digest, ok := strings.Cut(name, "-")
	if !ok || (algorithm != "sha256" && algorithm != "sha512") {
		return false
	}
	expected := 64
	if algorithm == "sha512" {
		expected = 128
	}
	if len(digest) != expected {
		return false
	}
	for _, character := range digest {
		if !strings.ContainsRune("0123456789abcdef", character) {
			return false
		}
	}
	return true
}

// LoadWorkspace assembles a package environment using only lock and store data.
func LoadWorkspace(root string) (*scope.Workspace, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	_, manifestErr := os.Stat(filepath.Join(root, "package.json"))
	_, lockErr := os.Stat(filepath.Join(root, "locus.lock"))
	if os.IsNotExist(manifestErr) && os.IsNotExist(lockErr) {
		return packageenv.Load(root, packageenv.Environment{RootDependencies: map[string]packageenv.Identity{}, Packages: map[packageenv.Identity]packageenv.Package{}})
	}
	manifest, _, err := readProjectManifest(root)
	if err != nil {
		return nil, err
	}
	lock, _, err := readLock(root, true)
	if err != nil {
		return nil, err
	}
	if !lockMatchesDependencies(lock, manifest.dependencies) {
		return nil, fmt.Errorf("locus.lock does not match package.json; run locus-pkg install")
	}
	materialized, err := materialize(context.Background(), root, lock, nil, true)
	if err != nil {
		return nil, fmt.Errorf("package environment is incomplete; run locus-pkg install: %w", err)
	}
	defer materialized.close()
	workspace, err := loadEnvironment(root, lock, materialized.packages)
	if err != nil {
		return nil, err
	}
	if err := materialized.publish(); err != nil {
		materialized.rollback()
		return nil, err
	}
	return workspace, nil
}
