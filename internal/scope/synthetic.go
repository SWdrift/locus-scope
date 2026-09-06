package scope

import (
	"fmt"
	"path/filepath"
)

// LoadDefinitionFile loads one definition document as an isolated synthetic Scope.
func LoadDefinitionFile(path string) (*Workspace, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve definition file %q: %w", path, err)
	}
	key := localScopeKey(absolute)
	current := &Scope{
		Key:       key,
		LocalPath: filepath.Dir(absolute),
		Manifest:  Manifest{ID: filepath.Base(absolute), Imports: map[string]string{}, Exports: []string{}},
		Entities:  make(map[string]Entity), Imports: make(map[string]ScopeKey), exported: make(map[string]struct{}),
	}
	if err := current.decodeDefinition(absolute); err != nil {
		return nil, err
	}
	workspace := &Workspace{Root: key, Scopes: map[ScopeKey]*Scope{key: current}}
	if err := workspace.validate(); err != nil {
		return nil, err
	}
	return workspace, nil
}
