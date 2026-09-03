package packages

import (
	_ "crypto/sha256"
	"fmt"
	"strings"

	"oras.land/oras-go/v2/registry"
)

const (
	ArtifactType = "application/vnd.locus.scope.package.v1"
	ociScheme    = "oci://"
)

type packageReference struct {
	Registry   string
	Repository string
	Reference  string
	Canonical  string
	Mutable    bool
}

func parsePackageReference(value string) (packageReference, error) {
	if !strings.HasPrefix(value, ociScheme) {
		if len(value) >= len(ociScheme) && strings.EqualFold(value[:len(ociScheme)], ociScheme) {
			return packageReference{}, fmt.Errorf("package reference %q must use the exact lowercase %q scheme", value, ociScheme)
		}
		if separator := strings.Index(value, "://"); separator >= 0 {
			return packageReference{}, fmt.Errorf("unsupported package reference scheme %q", value[:separator])
		}
		return packageReference{}, fmt.Errorf("package reference %q must use the exact %q scheme", value, ociScheme)
	}
	if strings.Contains(value, "#") {
		return packageReference{}, fmt.Errorf("package reference %q must not contain a fragment", value)
	}

	trimmed := strings.TrimPrefix(value, ociScheme)
	_, repositoryAndReference, found := strings.Cut(trimmed, "/")
	if found {
		if at := strings.IndexByte(repositoryAndReference, '@'); at >= 0 {
			beforeDigest := repositoryAndReference[:at]
			if colon := strings.LastIndexByte(beforeDigest, ':'); colon > strings.LastIndexByte(beforeDigest, '/') {
				return packageReference{}, fmt.Errorf("package reference %q must not combine a tag and digest", value)
			}
		}
	}

	parsed, err := registry.ParseReference(trimmed)
	if err != nil {
		return packageReference{}, fmt.Errorf("parse package reference %q: %w", value, err)
	}
	if parsed.Reference == "" {
		parsed.Reference = "latest"
		if err := parsed.ValidateReferenceAsTag(); err != nil {
			return packageReference{}, fmt.Errorf("parse package reference %q: %w", value, err)
		}
	}

	mutable := parsed.ValidateReferenceAsDigest() != nil
	if mutable {
		if err := parsed.ValidateReferenceAsTag(); err != nil {
			return packageReference{}, fmt.Errorf("parse package reference %q: %w", value, err)
		}
	}
	return packageReference{
		Registry:   parsed.Registry,
		Repository: parsed.Repository,
		Reference:  parsed.Reference,
		Canonical:  ociScheme + parsed.String(),
		Mutable:    mutable,
	}, nil
}

func digestReference(registryName, repository, digest string) (packageReference, error) {
	return parsePackageReference(ociScheme + registryName + "/" + repository + "@" + digest)
}
