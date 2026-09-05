package purepkg

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	locusnpm "locus-scope/internal/npm"
)

type projectManifest struct {
	fields       map[string]json.RawMessage
	dependencies map[string]string
}

func readProjectManifest(root string) (projectManifest, []byte, error) {
	path := filepath.Join(root, "package.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return projectManifest{}, nil, fmt.Errorf("read package.json: %w", err)
	}
	if err := validateUniqueJSON(data); err != nil {
		return projectManifest{}, nil, fmt.Errorf("decode package.json: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var fields map[string]json.RawMessage
	if err := decoder.Decode(&fields); err != nil {
		return projectManifest{}, nil, fmt.Errorf("decode package.json: %w", err)
	}
	if fields == nil {
		return projectManifest{}, nil, fmt.Errorf("decode package.json: root must be an object")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return projectManifest{}, nil, fmt.Errorf("decode package.json: trailing JSON value")
	}
	dependencies := map[string]string{}
	if raw, ok := fields["dependencies"]; ok {
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return projectManifest{}, nil, fmt.Errorf("decode package.json: dependencies must be an object")
		}
		if err := json.Unmarshal(raw, &dependencies); err != nil || dependencies == nil {
			return projectManifest{}, nil, fmt.Errorf("decode package.json: dependencies must map names to string constraints")
		}
	}
	for name, constraint := range dependencies {
		canonical, err := locusnpm.ParsePackageName(name)
		if err != nil || canonical != name {
			return projectManifest{}, nil, fmt.Errorf("package.json dependency has invalid name %q", name)
		}
		if err := locusnpm.ValidateConstraint(constraint); err != nil {
			return projectManifest{}, nil, fmt.Errorf("package.json dependency %s has unsupported specifier %q", name, constraint)
		}
	}
	for _, field := range []string{"peerDependencies", "optionalDependencies"} {
		if raw, ok := fields[field]; ok {
			var values map[string]json.RawMessage
			if err := json.Unmarshal(raw, &values); err != nil {
				return projectManifest{}, nil, fmt.Errorf("decode package.json: %s must be an object", field)
			}
			if len(values) != 0 {
				return projectManifest{}, nil, fmt.Errorf("package.json %s are unsupported by Pure Locus", field)
			}
		}
	}
	return projectManifest{fields: fields, dependencies: dependencies}, data, nil
}

func (manifest projectManifest) withDependencies(dependencies map[string]string) projectManifest {
	fields := make(map[string]json.RawMessage, len(manifest.fields)+1)
	for name, value := range manifest.fields {
		fields[name] = append(json.RawMessage(nil), value...)
	}
	cloned := make(map[string]string, len(dependencies))
	for name, constraint := range dependencies {
		cloned[name] = constraint
	}
	manifest.fields = fields
	manifest.dependencies = cloned
	return manifest
}

func encodeProjectManifest(manifest projectManifest) ([]byte, error) {
	fields := make(map[string]any, len(manifest.fields)+1)
	for name, raw := range manifest.fields {
		if name == "dependencies" {
			continue
		}
		fields[name] = raw
	}
	fields["dependencies"] = manifest.dependencies
	data, err := json.MarshalIndent(fields, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode package.json: %w", err)
	}
	return append(data, '\n'), nil
}

func validateUniqueJSON(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := consumeJSONValue(decoder); err != nil {
		return err
	}
	if token, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return fmt.Errorf("trailing token %v", token)
		}
		return err
	}
	return nil
}

func consumeJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		seen := map[string]bool{}
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return fmt.Errorf("object key must be a string")
			}
			if seen[key] {
				return fmt.Errorf("duplicate field %q", key)
			}
			seen[key] = true
			if err := consumeJSONValue(decoder); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim('}') {
			return fmt.Errorf("unterminated object")
		}
	case '[':
		for decoder.More() {
			if err := consumeJSONValue(decoder); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim(']') {
			return fmt.Errorf("unterminated array")
		}
	default:
		return fmt.Errorf("unexpected delimiter %q", delimiter)
	}
	return nil
}

func sortedNames(values map[string]string) []string {
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
