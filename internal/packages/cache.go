package packages

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/content/oci"
	"oras.land/oras-go/v2/errdef"
)

type cachedPackage struct {
	store    *oci.Store
	manifest ocispec.Descriptor
	layer    ocispec.Descriptor
}

func ensureCached(ctx context.Context, cacheRoot string, reference packageReference, source oras.ReadOnlyTarget) (cachedPackage, bool, error) {
	if reference.Mutable {
		return cachedPackage{}, false, fmt.Errorf("cache requires an immutable package reference, got %q", reference.Canonical)
	}
	store, err := oci.New(cacheRepositoryPath(cacheRoot, reference))
	if err != nil {
		return cachedPackage{}, false, fmt.Errorf("open OCI cache for %q: %w", reference.Canonical, err)
	}

	manifest, err := store.Resolve(ctx, reference.Reference)
	if err == nil {
		cached, err := validateCachedPackage(ctx, store, manifest)
		if err != nil {
			return cachedPackage{}, false, fmt.Errorf("invalid cached package %q: %w", reference.Canonical, err)
		}
		cached.store = store
		return cached, false, nil
	}
	if !errors.Is(err, errdef.ErrNotFound) {
		return cachedPackage{}, false, fmt.Errorf("resolve cached package %q: %w", reference.Canonical, err)
	}
	if source == nil {
		return cachedPackage{}, false, fmt.Errorf("package %q is not present in the OCI cache", reference.Canonical)
	}

	copied, err := oras.Copy(ctx, source, reference.Reference, store, reference.Reference, oras.DefaultCopyOptions)
	if err != nil {
		return cachedPackage{}, false, fmt.Errorf("cache package %q: %w", reference.Canonical, err)
	}
	if copied.Digest.String() != reference.Reference {
		return cachedPackage{}, false, fmt.Errorf("registry returned digest %q for requested package %q", copied.Digest, reference.Canonical)
	}
	manifest, err = store.Resolve(ctx, reference.Reference)
	if err != nil {
		return cachedPackage{}, false, fmt.Errorf("resolve copied package %q: %w", reference.Canonical, err)
	}
	cached, err := validateCachedPackage(ctx, store, manifest)
	if err != nil {
		return cachedPackage{}, false, fmt.Errorf("invalid copied package %q: %w", reference.Canonical, err)
	}
	cached.store = store
	return cached, true, nil
}

func validateCachedPackage(ctx context.Context, store content.ReadOnlyStorage, manifestDescriptor ocispec.Descriptor) (cachedPackage, error) {
	if manifestDescriptor.MediaType != ocispec.MediaTypeImageManifest {
		return cachedPackage{}, fmt.Errorf("manifest media type is %q, expected %q", manifestDescriptor.MediaType, ocispec.MediaTypeImageManifest)
	}
	manifestReader, err := store.Fetch(ctx, manifestDescriptor)
	if err != nil {
		return cachedPackage{}, fmt.Errorf("fetch manifest %s: %w", manifestDescriptor.Digest, err)
	}
	manifestData, readErr := content.ReadAll(manifestReader, manifestDescriptor)
	closeErr := manifestReader.Close()
	if readErr != nil {
		return cachedPackage{}, fmt.Errorf("read manifest %s: %w", manifestDescriptor.Digest, readErr)
	}
	if closeErr != nil {
		return cachedPackage{}, fmt.Errorf("close manifest %s: %w", manifestDescriptor.Digest, closeErr)
	}

	var manifest ocispec.Manifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return cachedPackage{}, fmt.Errorf("decode manifest %s: %w", manifestDescriptor.Digest, err)
	}
	if manifest.SchemaVersion != 2 {
		return cachedPackage{}, fmt.Errorf("manifest schema version is %d, expected 2", manifest.SchemaVersion)
	}
	if manifest.MediaType != ocispec.MediaTypeImageManifest {
		return cachedPackage{}, fmt.Errorf("manifest declares media type %q, expected %q", manifest.MediaType, ocispec.MediaTypeImageManifest)
	}
	if manifest.ArtifactType != ArtifactType {
		return cachedPackage{}, fmt.Errorf("manifest artifact type is %q, expected %q", manifest.ArtifactType, ArtifactType)
	}
	if len(manifest.Layers) != 1 {
		return cachedPackage{}, fmt.Errorf("manifest contains %d layers, expected exactly 1", len(manifest.Layers))
	}
	layer := manifest.Layers[0]
	if layer.MediaType != ocispec.MediaTypeImageLayerGzip {
		return cachedPackage{}, fmt.Errorf("layer media type is %q, expected %q", layer.MediaType, ocispec.MediaTypeImageLayerGzip)
	}

	layerReader, err := store.Fetch(ctx, layer)
	if err != nil {
		return cachedPackage{}, fmt.Errorf("fetch layer %s: %w", layer.Digest, err)
	}
	verifier := content.NewVerifyReader(layerReader, layer)
	_, copyErr := io.Copy(io.Discard, verifier)
	verifyErr := verifier.Verify()
	closeErr = layerReader.Close()
	if copyErr != nil {
		return cachedPackage{}, fmt.Errorf("read layer %s: %w", layer.Digest, copyErr)
	}
	if verifyErr != nil {
		return cachedPackage{}, fmt.Errorf("verify layer %s: %w", layer.Digest, verifyErr)
	}
	if closeErr != nil {
		return cachedPackage{}, fmt.Errorf("close layer %s: %w", layer.Digest, closeErr)
	}
	return cachedPackage{manifest: manifestDescriptor, layer: layer}, nil
}

func cacheRepositoryPath(cacheRoot string, reference packageReference) string {
	parts := []string{cacheRoot, encodeCacheSegment("r-", reference.Registry)}
	for _, segment := range strings.Split(reference.Repository, "/") {
		parts = append(parts, encodeCacheSegment("p-", segment))
	}
	return filepath.Join(parts...)
}

func encodeCacheSegment(prefix, value string) string {
	const hexadecimal = "0123456789ABCDEF"
	var encoded strings.Builder
	encoded.Grow(len(prefix) + len(value))
	encoded.WriteString(prefix)
	bytes := []byte(value)
	for index, b := range bytes {
		allowed := b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b == '.' || b == '_' || b == '-'
		if allowed && !(b == '.' && index == len(bytes)-1) {
			encoded.WriteByte(b)
			continue
		}
		encoded.WriteByte('%')
		encoded.WriteByte(hexadecimal[b>>4])
		encoded.WriteByte(hexadecimal[b&0x0f])
	}
	return encoded.String()
}
