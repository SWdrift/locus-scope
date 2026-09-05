package npm

import (
	"fmt"
	"strings"

	"deps.dev/util/semver"
)

const identityPrefix = "npm:"

// PackageSpec is an npm package name and a Registry SemVer constraint.
type PackageSpec struct {
	Name       string
	Constraint string
}

// ParsePackageName validates and returns a canonical npm package name.
func ParsePackageName(value string) (string, error) {
	if value == "" || len(value) > 214 || value != strings.ToLower(value) {
		return "", fmt.Errorf("invalid npm package name %q", value)
	}
	if value == "node_modules" || value == "favicon.ico" {
		return "", fmt.Errorf("invalid npm package name %q", value)
	}
	if strings.HasPrefix(value, "@") {
		parts := strings.Split(value[1:], "/")
		if len(parts) != 2 || !validNamePart(parts[0]) || !validNamePart(parts[1]) {
			return "", fmt.Errorf("invalid npm package name %q", value)
		}
		return value, nil
	}
	if strings.Contains(value, "/") || !validNamePart(value) {
		return "", fmt.Errorf("invalid npm package name %q", value)
	}
	return value, nil
}

func validNamePart(value string) bool {
	if value == "" || value[0] == '.' || value[0] == '_' || value[len(value)-1] == '.' {
		return false
	}
	for i := range len(value) {
		c := value[i]
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.' || c == '~' {
			continue
		}
		return false
	}
	return true
}

// ParsePackageSpec parses name, name@constraint, or @scope/name@constraint.
// A bare package name has the npm wildcard constraint "*".
func ParsePackageSpec(value string) (PackageSpec, error) {
	name, constraint := value, "*"
	separator := strings.LastIndex(value, "@")
	if strings.HasPrefix(value, "@") {
		if slash := strings.IndexByte(value, '/'); slash < 0 {
			return PackageSpec{}, fmt.Errorf("invalid npm package spec %q", value)
		} else if separator > slash {
			name, constraint = value[:separator], value[separator+1:]
		}
	} else if separator >= 0 {
		name, constraint = value[:separator], value[separator+1:]
	}
	if _, err := ParsePackageName(name); err != nil {
		return PackageSpec{}, fmt.Errorf("invalid npm package spec %q: %w", value, err)
	}
	if constraint == "" {
		return PackageSpec{}, fmt.Errorf("invalid npm package spec %q: empty version constraint", value)
	}
	if _, err := semver.NPM.ParseConstraint(constraint); err != nil {
		return PackageSpec{}, fmt.Errorf("invalid npm package spec %q: %w", value, err)
	}
	return PackageSpec{Name: name, Constraint: constraint}, nil
}

// ValidateVersion accepts only concrete npm SemVer versions.
func ValidateVersion(value string) error {
	_, err := parseConcreteVersion(value)
	return err
}

func parseConcreteVersion(value string) (*semver.Version, error) {
	version, err := semver.NPM.Parse(value)
	if err != nil {
		return nil, fmt.Errorf("invalid npm version %q: %w", value, err)
	}
	if canonical := version.Canon(true); canonical != value {
		return nil, fmt.Errorf("npm version %q is not canonical (use %q)", value, canonical)
	}
	return version, nil
}

// ValidateConstraint accepts only npm Registry SemVer requirements.
func ValidateConstraint(value string) error {
	if value == "" {
		return fmt.Errorf("npm version constraint is empty")
	}
	if _, err := semver.NPM.ParseConstraint(value); err != nil {
		return fmt.Errorf("invalid npm version constraint %q: %w", value, err)
	}
	return nil
}

// Matches reports whether a concrete version satisfies an npm constraint.
func Matches(version, constraint string) (bool, error) {
	v, err := parseConcreteVersion(version)
	if err != nil {
		return false, err
	}
	c, err := semver.NPM.ParseConstraint(constraint)
	if err != nil {
		return false, fmt.Errorf("invalid npm version constraint %q: %w", constraint, err)
	}
	return c.MatchVersion(v), nil
}

// HighestMatching returns the highest concrete version satisfying constraint.
func HighestMatching(versions []string, constraint string) (string, error) {
	c, err := semver.NPM.ParseConstraint(constraint)
	if err != nil {
		return "", fmt.Errorf("invalid npm version constraint %q: %w", constraint, err)
	}
	var best *semver.Version
	bestText := ""
	for _, text := range versions {
		version, err := parseConcreteVersion(text)
		if err != nil {
			return "", err
		}
		if c.MatchVersion(version) && (best == nil || version.Compare(best) > 0) {
			best, bestText = version, text
		}
	}
	if best == nil {
		return "", fmt.Errorf("no version matches npm constraint %q", constraint)
	}
	return bestText, nil
}

// Identity constructs the semantic identity used by package environments.
func Identity(name, version string) (string, error) {
	if _, err := ParsePackageName(name); err != nil {
		return "", err
	}
	if err := ValidateVersion(version); err != nil {
		return "", err
	}
	return identityPrefix + name + "@" + version, nil
}

// ParseIdentity validates an npm semantic identity.
func ParseIdentity(value string) (name, version string, err error) {
	if !strings.HasPrefix(value, identityPrefix) {
		return "", "", fmt.Errorf("invalid npm identity %q", value)
	}
	body := strings.TrimPrefix(value, identityPrefix)
	separator := strings.LastIndex(body, "@")
	if separator <= 0 {
		return "", "", fmt.Errorf("invalid npm identity %q", value)
	}
	name, version = body[:separator], body[separator+1:]
	canonical, identityErr := Identity(name, version)
	if identityErr != nil || canonical != value {
		if identityErr != nil {
			return "", "", fmt.Errorf("invalid npm identity %q: %w", value, identityErr)
		}
		return "", "", fmt.Errorf("invalid npm identity %q", value)
	}
	return name, version, nil
}
