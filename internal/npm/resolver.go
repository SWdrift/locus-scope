package npm

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"deps.dev/util/resolve"
	resolvernpm "deps.dev/util/resolve/npm"
	"deps.dev/util/resolve/version"
)

// ResolverClient implements resolve.Client from Registry packuments without
// calling the hosted deps.dev service.
type ResolverClient struct {
	registry *Client
	mutex    sync.Mutex
	cache    map[string]Packument
}

func NewResolverClient(registry *Client) *ResolverClient {
	if registry == nil {
		panic("npm.NewResolverClient: nil Client")
	}
	return &ResolverClient{registry: registry, cache: make(map[string]Packument)}
}

// NewResolver returns the deps.dev npm resolver backed by Registry metadata.
func NewResolver(registry *Client) resolve.Resolver {
	return resolvernpm.NewResolver(NewResolverClient(registry))
}

func (c *ResolverClient) Version(ctx context.Context, key resolve.VersionKey) (resolve.Version, error) {
	if err := validateConcreteKey(key); err != nil {
		return resolve.Version{}, err
	}
	packument, err := c.packument(ctx, key.Name)
	if err != nil {
		return resolve.Version{}, err
	}
	metadata, exists := packument.Versions[key.Version]
	if !exists {
		return resolve.Version{}, fmt.Errorf("version %v: %w", key, resolve.ErrNotFound)
	}
	if err := validateVersionMetadata(key.Name, key.Version, metadata, false); err != nil {
		return resolve.Version{}, err
	}
	return makeResolverVersion(packument, key), nil
}

func (c *ResolverClient) Versions(ctx context.Context, key resolve.PackageKey) ([]resolve.Version, error) {
	if key.System != resolve.NPM {
		return nil, fmt.Errorf("expected npm package key, got %v", key)
	}
	if _, err := ParsePackageName(key.Name); err != nil {
		return nil, err
	}
	packument, err := c.packument(ctx, key.Name)
	if err != nil {
		return nil, err
	}
	versions := make([]resolve.Version, 0, len(packument.Versions))
	for version := range packument.Versions {
		versions = append(versions, makeResolverVersion(packument, resolve.VersionKey{
			PackageKey:  key,
			VersionType: resolve.Concrete,
			Version:     version,
		}))
	}
	resolve.SortVersions(versions)
	return versions, nil
}

func makeResolverVersion(packument Packument, key resolve.VersionKey) resolve.Version {
	result := resolve.Version{VersionKey: key}
	tags := make([]string, 0, len(packument.DistTags))
	for tag, target := range packument.DistTags {
		if target == key.Version {
			tags = append(tags, tag)
		}
	}
	if len(tags) != 0 {
		sort.Strings(tags)
		result.SetAttr(version.Tags, strings.Join(tags, ","))
	}
	return result
}

func (c *ResolverClient) Requirements(ctx context.Context, key resolve.VersionKey) ([]resolve.RequirementVersion, error) {
	if err := validateConcreteKey(key); err != nil {
		return nil, err
	}
	packument, err := c.packument(ctx, key.Name)
	if err != nil {
		return nil, err
	}
	metadata, exists := packument.Versions[key.Version]
	if !exists {
		return nil, fmt.Errorf("version %v: %w", key, resolve.ErrNotFound)
	}
	if err := validateVersionMetadata(key.Name, key.Version, metadata, true); err != nil {
		return nil, err
	}
	requirements := make([]resolve.RequirementVersion, 0, len(metadata.Dependencies))
	for name, constraint := range metadata.Dependencies {
		requirements = append(requirements, resolve.RequirementVersion{VersionKey: resolve.VersionKey{
			PackageKey:  resolve.PackageKey{System: resolve.NPM, Name: name},
			VersionType: resolve.Requirement,
			Version:     constraint,
		}})
	}
	resolve.SortDependencies(requirements)
	return requirements, nil
}

func (c *ResolverClient) MatchingVersions(ctx context.Context, key resolve.VersionKey) ([]resolve.Version, error) {
	if key.System != resolve.NPM || key.VersionType != resolve.Requirement {
		return nil, fmt.Errorf("expected npm requirement key, got %v", key)
	}
	if err := ValidateConstraint(key.Version); err != nil {
		return nil, err
	}
	versions, err := c.Versions(ctx, key.PackageKey)
	if err != nil {
		return nil, err
	}
	return resolve.MatchRequirement(key, versions), nil
}

func validateConcreteKey(key resolve.VersionKey) error {
	if key.System != resolve.NPM || key.VersionType != resolve.Concrete {
		return fmt.Errorf("expected concrete npm version key, got %v", key)
	}
	if _, err := ParsePackageName(key.Name); err != nil {
		return err
	}
	return ValidateVersion(key.Version)
}

func (c *ResolverClient) packument(ctx context.Context, name string) (Packument, error) {
	c.mutex.Lock()
	cached, exists := c.cache[name]
	c.mutex.Unlock()
	if exists {
		return cached, nil
	}
	packument, err := c.registry.Packument(ctx, name)
	if err != nil {
		return Packument{}, err
	}
	c.mutex.Lock()
	if cached, exists := c.cache[name]; exists {
		c.mutex.Unlock()
		return cached, nil
	}
	c.cache[name] = packument
	c.mutex.Unlock()
	return packument, nil
}
