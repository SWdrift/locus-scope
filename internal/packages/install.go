package packages

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"locus-scope/internal/scope"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/credentials"
)

type InstallOptions struct {
	Frozen     bool
	CacheRoot  string
	Credential auth.CredentialFunc
}

type InstallResult struct {
	Valid        bool           `json:"valid"`
	Root         scope.ScopeKey `json:"root"`
	Resolved     int            `json:"resolved"`
	Reused       int            `json:"reused"`
	Fetched      int            `json:"fetched"`
	Materialized int            `json:"materialized"`
	Scopes       int            `json:"scopes"`
	Entities     int            `json:"entities"`
	Relations    int            `json:"relations"`
}

func Install(ctx context.Context, rootDirectory string, options InstallOptions) (InstallResult, error) {
	if strings.TrimSpace(options.CacheRoot) == "" {
		return InstallResult{}, fmt.Errorf("OCI cache root is required")
	}
	root, err := scope.NewLocalSource(rootDirectory)
	if err != nil {
		return InstallResult{}, fmt.Errorf("load root scope: %w", err)
	}
	lock, err := readLock(root.LocalPath)
	if err != nil {
		return InstallResult{}, err
	}
	resolver := newInstallingResolver(ctx, root.LocalPath, options, lock)
	workspace, err := scope.Load(root, resolver)
	if err != nil {
		return InstallResult{}, err
	}

	if options.Frozen {
		missing, stale := lockDifference(lock.Packages, resolver.reachable)
		if len(missing) != 0 || len(stale) != 0 {
			return InstallResult{}, fmt.Errorf("frozen lock mismatch: missing [%s]; stale [%s]", strings.Join(missing, ", "), strings.Join(stale, ", "))
		}
	} else {
		if _, err := writeLockIfChanged(root.LocalPath, Lock{Version: lockVersion, Packages: resolver.reachable}); err != nil {
			return InstallResult{}, err
		}
	}

	entities := 0
	for _, loaded := range workspace.Scopes {
		entities += len(loaded.Entities)
	}
	return InstallResult{
		Valid:        true,
		Root:         workspace.Root,
		Resolved:     resolver.resolved,
		Reused:       resolver.reused,
		Fetched:      resolver.fetched,
		Materialized: resolver.materialized,
		Scopes:       len(workspace.Scopes),
		Entities:     entities,
		Relations:    len(workspace.Relations),
	}, nil
}

func LoadWorkspace(rootDirectory string) (*scope.Workspace, error) {
	root, err := scope.NewLocalSource(rootDirectory)
	if err != nil {
		return nil, fmt.Errorf("load root scope: %w", err)
	}
	lock, err := readLock(root.LocalPath)
	if err != nil {
		return nil, err
	}
	return scope.Load(root, newOfflineResolver(root.LocalPath, lock))
}

func DefaultCacheRoot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home directory: %w", err)
	}
	return filepath.Join(home, ".locus", "oci"), nil
}

func DockerCredential() (auth.CredentialFunc, error) {
	store, err := credentials.NewStoreFromDocker(credentials.StoreOptions{})
	if err != nil {
		return nil, fmt.Errorf("load Docker credentials: %w", err)
	}
	return credentials.Credential(store), nil
}

func lockDifference(existing, reachable map[string]LockedPackage) (missing, stale []string) {
	for key := range reachable {
		if _, exists := existing[key]; !exists {
			missing = append(missing, key)
		}
	}
	for key := range existing {
		if _, exists := reachable[key]; !exists {
			stale = append(stale, key)
		}
	}
	sort.Strings(missing)
	sort.Strings(stale)
	return missing, stale
}

func isLoopbackRegistry(registryName string) bool {
	host := registryName
	if splitHost, _, err := net.SplitHostPort(registryName); err == nil {
		host = splitHost
	}
	host = strings.Trim(host, "[]")
	return strings.EqualFold(host, "localhost") || net.ParseIP(host) != nil && net.ParseIP(host).IsLoopback()
}
