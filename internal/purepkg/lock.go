package purepkg

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	locusnpm "locus-scope/internal/npm"
)

const lockVersion = 1

type lockFile struct {
	Version   int                    `yaml:"version"`
	Importers map[string]importer    `yaml:"importers"`
	Packages  map[string]lockPackage `yaml:"packages"`
}

type importer struct {
	Dependencies map[string]lockEdge `yaml:"dependencies"`
}

type lockPackage struct {
	Registry     string              `yaml:"registry"`
	Resolved     string              `yaml:"resolved"`
	Integrity    string              `yaml:"integrity"`
	Dependencies map[string]lockEdge `yaml:"dependencies"`
}

type lockEdge struct {
	Specifier string `yaml:"specifier"`
	Package   string `yaml:"package"`
}

func emptyLock() lockFile {
	return lockFile{
		Version:   lockVersion,
		Importers: map[string]importer{".": {Dependencies: map[string]lockEdge{}}},
		Packages:  map[string]lockPackage{},
	}
}

func readLock(root string, required bool) (lockFile, []byte, error) {
	path := filepath.Join(root, "locus.lock")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) && !required {
		return emptyLock(), nil, nil
	}
	if err != nil {
		return lockFile{}, nil, fmt.Errorf("read lock file %s: %w", path, err)
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	var lock lockFile
	if err := decoder.Decode(&lock); err != nil {
		return lockFile{}, nil, fmt.Errorf("decode lock file %s: %w", path, err)
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			err = fmt.Errorf("multiple YAML documents")
		}
		return lockFile{}, nil, fmt.Errorf("decode lock file %s: %w", path, err)
	}
	if err := validateLock(lock); err != nil {
		return lockFile{}, nil, fmt.Errorf("decode lock file %s: %w", path, err)
	}
	canonical, err := encodeLock(lock)
	if err != nil {
		return lockFile{}, nil, fmt.Errorf("encode lock file %s: %w", path, err)
	}
	if !bytes.Equal(data, canonical) {
		return lockFile{}, nil, fmt.Errorf("decode lock file %s: lock file is not canonical", path)
	}
	return lock, data, nil
}

func validateLock(lock lockFile) error {
	if lock.Version != lockVersion {
		return fmt.Errorf("unsupported lock version %d", lock.Version)
	}
	if len(lock.Importers) != 1 {
		return fmt.Errorf("importers must contain only root importer %q", ".")
	}
	root, ok := lock.Importers["."]
	if !ok || root.Dependencies == nil {
		return fmt.Errorf("root importer dependencies must be a mapping")
	}
	if lock.Packages == nil {
		return fmt.Errorf("packages must be a mapping")
	}
	for identity, record := range lock.Packages {
		if _, _, err := locusnpm.ParseIdentity(identity); err != nil {
			return fmt.Errorf("invalid package identity %q: %w", identity, err)
		}
		if record.Dependencies == nil {
			return fmt.Errorf("package %s dependencies must be a mapping", identity)
		}
		if err := validateHTTPURL(record.Registry, true); err != nil {
			return fmt.Errorf("package %s registry: %w", identity, err)
		}
		if err := validateHTTPURL(record.Resolved, false); err != nil {
			return fmt.Errorf("package %s resolved: %w", identity, err)
		}
		integrity, err := locusnpm.ParseIntegrity(record.Integrity)
		if err != nil || integrity.String() != record.Integrity {
			return fmt.Errorf("package %s has invalid or non-canonical integrity", identity)
		}
		if err := validateEdges(record.Dependencies, lock.Packages); err != nil {
			return fmt.Errorf("package %s: %w", identity, err)
		}
	}
	if err := validateEdges(root.Dependencies, lock.Packages); err != nil {
		return fmt.Errorf("root importer: %w", err)
	}
	reachable := make(map[string]bool, len(lock.Packages))
	var visit func(string)
	visit = func(identity string) {
		if reachable[identity] {
			return
		}
		reachable[identity] = true
		for _, edge := range lock.Packages[identity].Dependencies {
			visit(edge.Package)
		}
	}
	for _, edge := range root.Dependencies {
		visit(edge.Package)
	}
	if len(reachable) != len(lock.Packages) {
		var unreachable []string
		for identity := range lock.Packages {
			if !reachable[identity] {
				unreachable = append(unreachable, identity)
			}
		}
		sort.Strings(unreachable)
		return fmt.Errorf("unreachable packages: %v", unreachable)
	}
	return nil
}

func validateEdges(edges map[string]lockEdge, packages map[string]lockPackage) error {
	for dependencyName, edge := range edges {
		name, err := locusnpm.ParsePackageName(dependencyName)
		if err != nil || name != dependencyName {
			return fmt.Errorf("invalid dependency name %q", dependencyName)
		}
		if err := locusnpm.ValidateConstraint(edge.Specifier); err != nil {
			return fmt.Errorf("dependency %s has invalid specifier %q", dependencyName, edge.Specifier)
		}
		targetName, targetVersion, err := locusnpm.ParseIdentity(edge.Package)
		if err != nil {
			return fmt.Errorf("dependency %s has invalid target %q", dependencyName, edge.Package)
		}
		if targetName != dependencyName {
			return fmt.Errorf("dependency %s targets package %s", dependencyName, targetName)
		}
		if _, ok := packages[edge.Package]; !ok {
			return fmt.Errorf("dependency %s target %s is missing", dependencyName, edge.Package)
		}
		matches, err := locusnpm.Matches(targetVersion, edge.Specifier)
		if err != nil || !matches {
			return fmt.Errorf("dependency %s target %s does not satisfy %q", dependencyName, edge.Package, edge.Specifier)
		}
	}
	return nil
}

func validateHTTPURL(value string, registry bool) error {
	parsed, err := url.Parse(value)
	if err != nil || !parsed.IsAbs() || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("must be an absolute HTTP(S) URL")
	}
	if parsed.User != nil || parsed.Fragment != "" {
		return fmt.Errorf("must not contain user information or fragment")
	}
	if parsed.Scheme == "http" {
		host := parsed.Hostname()
		address := net.ParseIP(host)
		if !strings.EqualFold(host, "localhost") && (address == nil || !address.IsLoopback()) {
			return fmt.Errorf("plain HTTP is allowed only for loopback hosts")
		}
	}
	if registry && (parsed.RawQuery != "" || parsed.Path == "" || parsed.Path[len(parsed.Path)-1] != '/') {
		return fmt.Errorf("registry URL must end with / and contain no query")
	}
	return nil
}

func encodeLock(lock lockFile) ([]byte, error) {
	if err := validateLock(lock); err != nil {
		return nil, err
	}
	var output bytes.Buffer
	encoder := yaml.NewEncoder(&output)
	encoder.SetIndent(2)
	if err := encoder.Encode(lock); err != nil {
		return nil, err
	}
	if err := encoder.Close(); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}
