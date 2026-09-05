// Package scopeapp exposes transport-independent Scope application queries.
package scopeapp

import (
	"fmt"
	"sort"

	"locus-scope/internal/scope"
)

// Service queries one fully loaded and validated Scope workspace.
type Service struct {
	workspace *scope.Workspace
}

// ValidationResult summarizes a loaded workspace.
type ValidationResult struct {
	Valid     bool           `json:"valid"`
	Root      scope.ScopeKey `json:"root"`
	Scopes    int            `json:"scopes"`
	Entities  int            `json:"entities"`
	Relations int            `json:"relations"`
}

// Import describes one resolved Scope import.
type Import struct {
	Alias  string         `json:"alias"`
	Source string         `json:"source"`
	Target scope.ScopeKey `json:"target"`
}

// Scope describes one loaded Scope.
type Scope struct {
	ID      string         `json:"id"`
	Source  scope.ScopeKey `json:"source"`
	Root    bool           `json:"root"`
	Imports []Import       `json:"imports"`
	Exports []string       `json:"exports"`
}

// ScopesResult contains every loaded Scope in stable source order.
type ScopesResult struct {
	Scopes []Scope `json:"scopes"`
}

// EntityKey identifies an entity and its owning Scope.
type EntityKey struct {
	ScopeID string         `json:"scope_id"`
	Scope   scope.ScopeKey `json:"scope"`
	ID      string         `json:"id"`
}

// Entity describes a resolved entity.
type Entity struct {
	ScopeID    string         `json:"scope_id"`
	Scope      scope.ScopeKey `json:"scope"`
	ID         string         `json:"id"`
	Properties map[string]any `json:"properties"`
}

// EntitiesResult contains every entity in stable Scope and entity order.
type EntitiesResult struct {
	Entities []EntityKey `json:"entities"`
}

// EntityResult associates the requested reference with its resolved entity.
type EntityResult struct {
	Reference string `json:"reference"`
	Entity    Entity `json:"entity"`
}

// Relation describes one resolved directed relation.
type Relation struct {
	From EntityKey `json:"from"`
	Name string    `json:"name"`
	To   EntityKey `json:"to"`
}

// RelationsResult contains every resolved relation.
type RelationsResult struct {
	Relations []Relation `json:"relations"`
}

// ResolveResult associates the requested reference with its stable entity identity.
type ResolveResult struct {
	Reference string    `json:"reference"`
	Entity    EntityKey `json:"entity"`
}

// New creates an application service over a loaded workspace.
func New(workspace *scope.Workspace) *Service {
	return &Service{workspace: workspace}
}

// Validate returns the summary of a workspace that passed loading validation.
func (s *Service) Validate() ValidationResult {
	entities := 0
	for _, loaded := range s.workspace.Scopes {
		entities += len(loaded.Entities)
	}
	return ValidationResult{
		Valid:     true,
		Root:      s.workspace.Root,
		Scopes:    len(s.workspace.Scopes),
		Entities:  entities,
		Relations: len(s.workspace.Relations),
	}
}

// RootScope returns the root Scope.
func (s *Service) RootScope() Scope {
	return s.makeScope(s.workspace.Root)
}

// ListScopes returns every loaded Scope in deterministic order.
func (s *Service) ListScopes() ScopesResult {
	keys := s.scopeKeys()
	result := ScopesResult{Scopes: make([]Scope, 0, len(keys))}
	for _, key := range keys {
		result.Scopes = append(result.Scopes, s.makeScope(key))
	}
	return result
}

// ListEntities returns every entity in deterministic owner and ID order.
func (s *Service) ListEntities() EntitiesResult {
	var entities []EntityKey
	for _, key := range s.scopeKeys() {
		loaded := s.workspace.Scopes[key]
		ids := make([]string, 0, len(loaded.Entities))
		for id := range loaded.Entities {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			entities = append(entities, EntityKey{ScopeID: loaded.Manifest.ID, Scope: key, ID: id})
		}
	}
	return EntitiesResult{Entities: entities}
}

// GetEntity resolves a reference from the root Scope and returns the entity.
func (s *Service) GetEntity(reference string) (EntityResult, error) {
	key, err := s.workspace.Resolve(s.workspace.Root, reference)
	if err != nil {
		return EntityResult{}, fmt.Errorf("resolve entity %q: %w", reference, err)
	}
	loaded := s.workspace.Scopes[key.Scope]
	entity := loaded.Entities[key.ID]
	return EntityResult{
		Reference: reference,
		Entity: Entity{
			ScopeID:    loaded.Manifest.ID,
			Scope:      key.Scope,
			ID:         key.ID,
			Properties: entity.Properties,
		},
	}, nil
}

// ListRelations returns every resolved relation.
func (s *Service) ListRelations() RelationsResult {
	relations := make([]Relation, 0, len(s.workspace.Relations))
	for _, relation := range s.workspace.Relations {
		relations = append(relations, Relation{
			From: s.makeEntityKey(relation.From),
			Name: relation.Name,
			To:   s.makeEntityKey(relation.To),
		})
	}
	return RelationsResult{Relations: relations}
}

// ResolveEntity resolves a reference from the root Scope to its stable identity.
func (s *Service) ResolveEntity(reference string) (ResolveResult, error) {
	key, err := s.workspace.Resolve(s.workspace.Root, reference)
	if err != nil {
		return ResolveResult{}, fmt.Errorf("resolve %q: %w", reference, err)
	}
	return ResolveResult{Reference: reference, Entity: s.makeEntityKey(key)}, nil
}

func (s *Service) makeScope(key scope.ScopeKey) Scope {
	loaded := s.workspace.Scopes[key]
	aliases := make([]string, 0, len(loaded.Imports))
	for alias := range loaded.Imports {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	imports := make([]Import, 0, len(aliases))
	for _, alias := range aliases {
		imports = append(imports, Import{Alias: alias, Source: loaded.Manifest.Imports[alias], Target: loaded.Imports[alias]})
	}
	exports := append([]string(nil), loaded.Manifest.Exports...)
	sort.Strings(exports)
	return Scope{ID: loaded.Manifest.ID, Source: key, Root: key == s.workspace.Root, Imports: imports, Exports: exports}
}

func (s *Service) makeEntityKey(key scope.EntityKey) EntityKey {
	return EntityKey{ScopeID: s.workspace.Scopes[key.Scope].Manifest.ID, Scope: key.Scope, ID: key.ID}
}

func (s *Service) scopeKeys() []scope.ScopeKey {
	keys := make([]scope.ScopeKey, 0, len(s.workspace.Scopes))
	for key := range s.workspace.Scopes {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	return keys
}
