package purepkg

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"

	"github.com/natefinch/atomic"

	locusnpm "locus-scope/internal/npm"
)

// List returns importer-relative dependency trees after validating lock/store.
func List(root string) (ListResult, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return ListResult{}, err
	}
	lock, _, err := readLock(root, true)
	if err != nil {
		return ListResult{}, err
	}
	materialized, err := materialize(context.Background(), root, lock, nil, true)
	if err != nil {
		return ListResult{}, fmt.Errorf("invalid package store: %w", err)
	}
	defer materialized.close()
	if len(materialized.stagedStores) != 0 {
		return ListResult{}, fmt.Errorf("package store is incomplete; run locus-pkg install")
	}
	result := ListResult{Root: root, Dependencies: []ListDependency{}}
	rootEdges := lock.Importers["."].Dependencies
	for _, name := range sortedEdgeNames(rootEdges) {
		result.Dependencies = append(result.Dependencies, listDependency(name, rootEdges[name].Package, lock, map[string]bool{}))
	}
	return result, nil
}

func listDependency(name, identity string, lock lockFile, ancestors map[string]bool) ListDependency {
	_, version, _ := locusnpm.ParseIdentity(identity)
	node := ListDependency{Name: name, Version: version, Identity: identity, Dependencies: []ListDependency{}}
	if ancestors[identity] {
		return node
	}
	nextAncestors := make(map[string]bool, len(ancestors)+1)
	for ancestor := range ancestors {
		nextAncestors[ancestor] = true
	}
	nextAncestors[identity] = true
	edges := lock.Packages[identity].Dependencies
	for _, dependencyName := range sortedEdgeNames(edges) {
		node.Dependencies = append(node.Dependencies, listDependency(dependencyName, edges[dependencyName].Package, lock, nextAncestors))
	}
	return node
}

func sortedEdgeNames(edges map[string]lockEdge) []string {
	names := make([]string, 0, len(edges))
	for name := range edges {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Pack writes the deterministic npm-compatible tarball in root.
func Pack(root string) (PackResult, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return PackResult{}, err
	}
	packed, err := locusnpm.Pack(root)
	if err != nil {
		return PackResult{}, err
	}
	filename := filepath.Join(root, packed.Filename)
	if err := atomic.WriteFile(filename, bytes.NewReader(packed.Tarball)); err != nil {
		return PackResult{}, fmt.Errorf("write package archive: %w", err)
	}
	return PackResult{Name: packed.Name, Version: packed.Version, Filename: packed.Filename, Integrity: packed.Integrity, Files: packed.Files}, nil
}

// Publish packs and publishes latest without leaving a package archive behind.
func Publish(ctx context.Context, root string, options Options) (PublishResult, error) {
	if options.Offline {
		return PublishResult{}, fmt.Errorf("publish cannot be used offline")
	}
	if options.FrozenLockfile {
		return PublishResult{}, fmt.Errorf("publish does not use a lockfile")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return PublishResult{}, err
	}
	packed, err := locusnpm.Pack(root)
	if err != nil {
		return PublishResult{}, err
	}
	operationRoot, err := newOperationRoot(root)
	if err != nil {
		return PublishResult{}, err
	}
	defer os.RemoveAll(operationRoot)
	stagedArchive := filepath.Join(operationRoot, packed.Filename)
	if err := os.WriteFile(stagedArchive, packed.Tarball, 0o644); err != nil {
		return PublishResult{}, fmt.Errorf("stage package archive: %w", err)
	}
	config, err := locusnpm.LoadConfig(root, options.Registry)
	if err != nil {
		return PublishResult{}, err
	}
	client := locusnpm.NewClient(config, http.DefaultClient)
	if err := client.Publish(ctx, packed); err != nil {
		return PublishResult{}, err
	}
	registry, err := client.RegistryFor(packed.Name)
	if err != nil {
		return PublishResult{}, err
	}
	return PublishResult{Name: packed.Name, Version: packed.Version, Registry: registry.String(), Integrity: packed.Integrity}, nil
}
