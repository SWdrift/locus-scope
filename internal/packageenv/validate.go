package packageenv

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"deps.dev/util/semver"
	"locus-scope/internal/scope"
)

var scopeManifestNames = map[string]struct{}{
	"locus.json": {},
	"locus.yaml": {},
	"locus.yml":  {},
}

// PackageMetadata is the validated package.json information used by package environments.
type PackageMetadata struct {
	Name         string
	Version      string
	Entry        string
	Dependencies map[string]string
}

type packageDocument struct {
	Name                 string          `json:"name"`
	Version              string          `json:"version"`
	Dependencies         json.RawMessage `json:"dependencies"`
	DevDependencies      json.RawMessage `json:"devDependencies"`
	PeerDependencies     json.RawMessage `json:"peerDependencies"`
	OptionalDependencies json.RawMessage `json:"optionalDependencies"`
	Exports              json.RawMessage `json:"exports"`
	Locus                json.RawMessage `json:"locus"`
}

type locusDocument struct {
	Entry string `json:"entry"`
}

// ValidatePackage validates package metadata and, when present, its single Locus Scope.
func ValidatePackage(root string) (PackageMetadata, bool, error) {
	canonicalRoot, err := canonicalDirectory(root)
	if err != nil {
		return PackageMetadata{}, false, err
	}
	packageJSONPath := filepath.Join(canonicalRoot, "package.json")
	contents, err := os.ReadFile(packageJSONPath)
	if err != nil {
		return PackageMetadata{}, false, fmt.Errorf("read package.json: %w", err)
	}
	if err := validateStrictJSON(contents); err != nil {
		return PackageMetadata{}, false, fmt.Errorf("%s: %w", packageJSONPath, err)
	}

	var document packageDocument
	if err := json.Unmarshal(contents, &document); err != nil {
		return PackageMetadata{}, false, fmt.Errorf("%s: decode package metadata: %w", packageJSONPath, err)
	}
	if err := validatePackageName(document.Name); err != nil {
		return PackageMetadata{}, false, fmt.Errorf("%s: invalid name: %w", packageJSONPath, err)
	}
	if err := validateExactVersion(document.Version); err != nil {
		return PackageMetadata{}, false, fmt.Errorf("%s: invalid version %q: %w", packageJSONPath, document.Version, err)
	}

	dependencies, err := decodeDependencyMap(document.Dependencies, "dependencies", true)
	if err != nil {
		return PackageMetadata{}, false, fmt.Errorf("%s: %w", packageJSONPath, err)
	}
	if _, err := decodeDependencyMap(document.DevDependencies, "devDependencies", false); err != nil {
		return PackageMetadata{}, false, fmt.Errorf("%s: %w", packageJSONPath, err)
	}
	if document.PeerDependencies != nil {
		return PackageMetadata{}, false, fmt.Errorf("%s: peerDependencies are not supported", packageJSONPath)
	}
	if document.OptionalDependencies != nil {
		return PackageMetadata{}, false, fmt.Errorf("%s: optionalDependencies are not supported", packageJSONPath)
	}

	metadata := PackageMetadata{Name: document.Name, Version: document.Version, Dependencies: dependencies}
	if document.Locus == nil {
		return metadata, false, nil
	}
	var locus locusDocument
	decoder := json.NewDecoder(bytes.NewReader(document.Locus))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&locus); err != nil {
		return PackageMetadata{}, false, fmt.Errorf("%s: invalid locus metadata: %w", packageJSONPath, err)
	}
	if locus.Entry == "" {
		return PackageMetadata{}, false, fmt.Errorf("%s: locus.entry is required", packageJSONPath)
	}
	rootPath, entry, err := secureEntry(canonicalRoot, locus.Entry)
	if err != nil {
		return PackageMetadata{}, false, fmt.Errorf("%s: invalid locus.entry: %w", packageJSONPath, err)
	}
	metadata.Entry = entry
	if err := requireUniqueManifest(rootPath, entry); err != nil {
		return PackageMetadata{}, false, err
	}
	if document.Exports != nil && !exportsPackageJSON(document.Exports) {
		return PackageMetadata{}, false, fmt.Errorf("%s: exports must explicitly expose ./package.json", packageJSONPath)
	}

	identity := Identity("npm:" + document.Name + "@" + document.Version)
	source := scope.Source{Key: scope.ScopeKey(identity), LocalPath: filepath.Dir(filepath.Join(rootPath, filepath.FromSlash(entry)))}
	loaded, err := scope.ReadSource(source)
	if err != nil {
		return PackageMetadata{}, false, fmt.Errorf("package %q Scope: %w", identity, err)
	}
	aliases := make([]string, 0, len(loaded.Manifest.Imports))
	for alias := range loaded.Manifest.Imports {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	for _, alias := range aliases {
		reference := loaded.Manifest.Imports[alias]
		if isLocalImport(reference) {
			return PackageMetadata{}, false, fmt.Errorf("package %q Scope import %q cannot use local source %q", identity, alias, reference)
		}
		if err := validatePackageName(reference); err != nil {
			return PackageMetadata{}, false, fmt.Errorf("package %q Scope import %q must be a bare npm package name without a version or subpath: %q", identity, alias, reference)
		}
		if _, declared := dependencies[reference]; !declared {
			return PackageMetadata{}, false, fmt.Errorf("package %q Scope import %q references undeclared dependency %q", identity, alias, reference)
		}
	}
	if err := scope.CheckSource(source); err != nil {
		return PackageMetadata{}, false, fmt.Errorf("package %q Scope: %w", identity, err)
	}
	return metadata, true, nil
}

func parseIdentity(identity Identity) (string, string, error) {
	value := string(identity)
	if !strings.HasPrefix(value, "npm:") {
		return "", "", fmt.Errorf("invalid package identity %q: expected npm:<name>@<version>", identity)
	}
	body := strings.TrimPrefix(value, "npm:")
	separator := strings.LastIndex(body, "@")
	if separator <= 0 || separator == len(body)-1 {
		return "", "", fmt.Errorf("invalid package identity %q: expected npm:<name>@<version>", identity)
	}
	name, version := body[:separator], body[separator+1:]
	if err := validatePackageName(name); err != nil {
		return "", "", fmt.Errorf("invalid package identity %q: %w", identity, err)
	}
	if err := validateExactVersion(version); err != nil {
		return "", "", fmt.Errorf("invalid package identity %q: invalid version: %w", identity, err)
	}
	return name, version, nil
}

func validatePackageName(name string) error {
	if name == "" {
		return errors.New("package name is required")
	}
	if len(name) > 214 || !utf8.ValidString(name) {
		return fmt.Errorf("package name %q is invalid", name)
	}
	if name == "node_modules" || name == "favicon.ico" {
		return fmt.Errorf("package name %q is reserved", name)
	}
	parts := []string{name}
	if strings.HasPrefix(name, "@") {
		if strings.Count(name, "/") != 1 {
			return fmt.Errorf("scoped package name %q must be @scope/name", name)
		}
		scopeName, packageName, _ := strings.Cut(strings.TrimPrefix(name, "@"), "/")
		parts = []string{scopeName, packageName}
	} else if strings.Contains(name, "/") {
		return fmt.Errorf("package name %q must not contain a subpath", name)
	}
	for _, part := range parts {
		if part == "" || part[0] == '.' || part[0] == '_' {
			return fmt.Errorf("package name %q is invalid", name)
		}
		for _, character := range part {
			validLetter := character >= 'a' && character <= 'z'
			validDigit := character >= '0' && character <= '9'
			if validLetter || validDigit || strings.ContainsRune("-._~", character) {
				continue
			}
			return fmt.Errorf("package name %q contains unsupported character %q", name, character)
		}
	}
	return nil
}

func validateExactVersion(version string) error {
	parsed, err := semver.NPM.Parse(version)
	if err != nil {
		return err
	}
	if canonical := parsed.Canon(true); canonical != version {
		return fmt.Errorf("version must be canonical npm SemVer %q", canonical)
	}
	return nil
}

func decodeDependencyMap(raw json.RawMessage, field string, requireSemver bool) (map[string]string, error) {
	if raw == nil {
		return make(map[string]string), nil
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, fmt.Errorf("%s must be an object", field)
	}
	var dependencies map[string]string
	if err := json.Unmarshal(raw, &dependencies); err != nil {
		return nil, fmt.Errorf("%s must map package names to string specifiers: %w", field, err)
	}
	names := make([]string, 0, len(dependencies))
	for name := range dependencies {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		specifier := dependencies[name]
		if err := validatePackageName(name); err != nil {
			return nil, fmt.Errorf("%s contains invalid package name %q: %w", field, name, err)
		}
		if strings.TrimSpace(specifier) == "" {
			return nil, fmt.Errorf("%s[%q] must be a non-empty string", field, name)
		}
		if requireSemver {
			if _, err := semver.NPM.ParseConstraint(specifier); err != nil {
				return nil, fmt.Errorf("%s[%q] must be a Registry SemVer constraint, got %q: %w", field, name, specifier, err)
			}
		}
	}
	return dependencies, nil
}

func canonicalDirectory(directory string) (string, error) {
	if strings.TrimSpace(directory) == "" {
		return "", errors.New("package root is required")
	}
	absolute, err := filepath.Abs(directory)
	if err != nil {
		return "", fmt.Errorf("resolve package root %q: %w", directory, err)
	}
	resolved, err := filepath.EvalSymlinks(filepath.Clean(absolute))
	if err != nil {
		return "", fmt.Errorf("resolve package root %q: %w", absolute, err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("inspect package root %q: %w", resolved, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("package root %q is not a directory", resolved)
	}
	return filepath.Clean(resolved), nil
}

func secureEntry(root, entry string) (string, string, error) {
	canonicalRoot, err := canonicalDirectory(root)
	if err != nil {
		return "", "", err
	}
	if entry == "" || strings.Contains(entry, "\\") || path.IsAbs(entry) || filepath.IsAbs(entry) {
		return "", "", fmt.Errorf("entry %q must be a relative slash path", entry)
	}
	cleanEntry := path.Clean(entry)
	if cleanEntry == "." || cleanEntry != entry || cleanEntry == ".." || strings.HasPrefix(cleanEntry, "../") {
		return "", "", fmt.Errorf("entry %q must be normalized and remain inside the package root", entry)
	}
	if _, valid := scopeManifestNames[path.Base(cleanEntry)]; !valid {
		return "", "", fmt.Errorf("entry %q must name locus.yaml, locus.yml, or locus.json", entry)
	}
	joined := filepath.Join(canonicalRoot, filepath.FromSlash(cleanEntry))
	resolved, err := filepath.EvalSymlinks(joined)
	if err != nil {
		return "", "", fmt.Errorf("resolve entry %q: %w", entry, err)
	}
	if !pathWithin(canonicalRoot, resolved) {
		return "", "", fmt.Errorf("entry %q escapes package root through a symbolic link", entry)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", "", fmt.Errorf("inspect entry %q: %w", entry, err)
	}
	if !info.Mode().IsRegular() {
		return "", "", fmt.Errorf("entry %q is not a regular file", entry)
	}
	return canonicalRoot, cleanEntry, nil
}

func pathWithin(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

func requireUniqueManifest(root, entry string) error {
	var manifests []string
	err := filepath.WalkDir(root, func(current string, item fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if item.IsDir() {
			if current != root && (item.Name() == "node_modules" || item.Name() == ".git" || item.Name() == ".locus") {
				return fs.SkipDir
			}
			return nil
		}
		if _, match := scopeManifestNames[item.Name()]; match {
			relative, err := filepath.Rel(root, current)
			if err != nil {
				return err
			}
			manifests = append(manifests, filepath.ToSlash(relative))
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("scan package %q for Scope manifests: %w", root, err)
	}
	sort.Strings(manifests)
	if len(manifests) != 1 || manifests[0] != entry {
		return fmt.Errorf("package %q must contain exactly the declared Scope manifest %q; found %v", root, entry, manifests)
	}
	return nil
}

func exportsPackageJSON(raw json.RawMessage) bool {
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if err := decoder.Decode(&value); err != nil {
		return false
	}
	exports, ok := value.(map[string]any)
	if !ok {
		return false
	}
	target, ok := exports["./package.json"].(string)
	return ok && target == "./package.json"
}

func validateStrictJSON(contents []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.UseNumber()
	if err := validateJSONValue(decoder); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("invalid JSON: trailing value")
		}
		return fmt.Errorf("invalid JSON: trailing data: %w", err)
	}
	return nil
}

func validateJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, composite := token.(json.Delim)
	if !composite {
		return nil
	}
	switch delimiter {
	case '{':
		keys := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("object key is not a string")
			}
			if _, duplicate := keys[key]; duplicate {
				return fmt.Errorf("duplicate object field %q", key)
			}
			keys[key] = struct{}{}
			if err := validateJSONValue(decoder); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
		return err
	case '[':
		for decoder.More() {
			if err := validateJSONValue(decoder); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
		return err
	default:
		return fmt.Errorf("unexpected JSON delimiter %q", delimiter)
	}
}
