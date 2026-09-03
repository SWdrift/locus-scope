package scope

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

var manifestFileNames = []string{"locus.json", "locus.yaml", "locus.yml"}

type manifestDocument struct {
	ID      string            `yaml:"id"`
	Imports map[string]string `yaml:"imports"`
	Exports []string          `yaml:"exports"`
}

type definitionDocument struct {
	Group     string             `yaml:"group"`
	Entities  []entityDocument   `yaml:"entities"`
	Relations []relationDocument `yaml:"relations"`
}

type entityDocument struct {
	ID         string         `yaml:"id"`
	Properties map[string]any `yaml:",inline"`
}

type relationDocument struct {
	from string
	name string
	to   string
	line int
}

func (r *relationDocument) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.SequenceNode || len(node.Content) != 3 {
		return fmt.Errorf("line %d: relation must contain exactly [from, relation, to]", node.Line)
	}

	values := make([]string, 3)
	for i, item := range node.Content {
		if item.Kind != yaml.ScalarNode || item.Tag != "!!str" || strings.TrimSpace(item.Value) == "" {
			return fmt.Errorf("line %d: relation item %d must be a non-empty string", item.Line, i+1)
		}
		values[i] = item.Value
	}

	r.from, r.name, r.to, r.line = values[0], values[1], values[2], node.Line
	return nil
}

func decodeScope(source Source) (*Scope, error) {
	directory := source.LocalPath
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, fmt.Errorf("read scope directory %q: %w", directory, err)
	}

	var manifests []string
	var definitions []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if slices.Contains(manifestFileNames, name) {
			manifests = append(manifests, name)
			continue
		}
		switch filepath.Ext(name) {
		case ".json", ".yaml", ".yml":
			definitions = append(definitions, name)
		}
	}
	sort.Strings(manifests)
	sort.Strings(definitions)

	if len(manifests) == 0 {
		return nil, fmt.Errorf("scope %q: manifest missing; expected exactly one of locus.yaml, locus.yml, or locus.json", directory)
	}
	if len(manifests) > 1 {
		return nil, fmt.Errorf("scope %q: multiple manifests found: %s", directory, strings.Join(manifests, ", "))
	}

	manifestPath := filepath.Join(directory, manifests[0])
	var rawManifest manifestDocument
	if err := decodeFile(manifestPath, &rawManifest); err != nil {
		return nil, err
	}
	if strings.TrimSpace(rawManifest.ID) == "" {
		return nil, fmt.Errorf("%s: scope id is required", manifestPath)
	}
	if rawManifest.Imports == nil {
		rawManifest.Imports = make(map[string]string)
	}
	if rawManifest.Exports == nil {
		rawManifest.Exports = make([]string, 0)
	}
	for alias, source := range rawManifest.Imports {
		if strings.TrimSpace(alias) == "" {
			return nil, fmt.Errorf("%s: import projection name must not be empty", manifestPath)
		}
		if strings.Contains(alias, ":") {
			return nil, fmt.Errorf("%s: import projection %q must not contain ':'", manifestPath, alias)
		}
		if strings.TrimSpace(source) == "" {
			return nil, fmt.Errorf("%s: import %q has an empty source path", manifestPath, alias)
		}
	}
	for i, ref := range rawManifest.Exports {
		if strings.TrimSpace(ref) == "" {
			return nil, fmt.Errorf("%s: export %d must be a non-empty entity reference", manifestPath, i+1)
		}
	}

	s := &Scope{
		Key: source.Key,
		Manifest: Manifest{
			ID:      rawManifest.ID,
			Imports: rawManifest.Imports,
			Exports: rawManifest.Exports,
		},
		Entities:      make(map[string]Entity),
		Imports:       make(map[string]ScopeKey),
		manifestPath:  manifestPath,
		exported:      make(map[string]struct{}, len(rawManifest.Exports)),
		entityOrigins: make(map[string]string),
	}
	for _, ref := range rawManifest.Exports {
		s.exported[ref] = struct{}{}
	}

	for _, name := range definitions {
		path := filepath.Join(directory, name)
		if err := s.decodeDefinition(path); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func (s *Scope) decodeDefinition(path string) error {
	var document definitionDocument
	if err := decodeFile(path, &document); err != nil {
		return err
	}
	if strings.Contains(document.Group, ":") {
		return fmt.Errorf("%s: group %q must not contain ':'", path, document.Group)
	}

	declaredIDs := make(map[string]struct{}, len(document.Entities))
	for i, raw := range document.Entities {
		if strings.TrimSpace(raw.ID) == "" {
			return fmt.Errorf("%s: entity %d: id is required", path, i+1)
		}
		if strings.Contains(raw.ID, ":") {
			return fmt.Errorf("%s: entity %q: id must not contain ':'", path, raw.ID)
		}
		declaredIDs[raw.ID] = struct{}{}

		id := raw.ID
		if document.Group != "" {
			id = document.Group + "/" + id
		}
		if previous, exists := s.entityOrigins[id]; exists {
			return fmt.Errorf("%s: entity %q conflicts with entity declared in %s after group expansion", path, id, previous)
		}
		properties := raw.Properties
		if properties == nil {
			properties = make(map[string]any)
		}
		s.Entities[id] = Entity{ID: id, Properties: properties}
		s.entityOrigins[id] = path
	}

	for _, raw := range document.Relations {
		from, to := raw.from, raw.to
		if !strings.Contains(from, ":") {
			if _, local := declaredIDs[from]; local && document.Group != "" {
				from = document.Group + "/" + from
			}
		}
		if !strings.Contains(to, ":") {
			if _, local := declaredIDs[to]; local && document.Group != "" {
				to = document.Group + "/" + to
			}
		}
		s.relationDecls = append(s.relationDecls, relationDecl{
			from: from, name: raw.name, to: to, source: path, line: raw.line,
		})
	}
	return nil
}

func decodeFile(path string, destination any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}

	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}

	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		return fmt.Errorf("%s: multiple YAML/JSON documents are not supported", path)
	}
	return nil
}
