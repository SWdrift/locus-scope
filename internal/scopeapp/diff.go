package scopeapp

import (
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"locus-scope/internal/scope"
)

type DiffSide struct {
	Entities  []Entity   `json:"entities"`
	Relations []Relation `json:"relations"`
}

type EntityChange struct {
	Before Entity `json:"before"`
	After  Entity `json:"after"`
}
type RelationChange struct {
	Before Relation `json:"before"`
	After  Relation `json:"after"`
}
type DiffChanged struct {
	Entities  []EntityChange   `json:"entities"`
	Relations []RelationChange `json:"relations"`
}
type DiffResult struct {
	Added   DiffSide    `json:"added"`
	Removed DiffSide    `json:"removed"`
	Changed DiffChanged `json:"changed"`
}

type semanticGraph struct {
	entities  map[string]Entity
	relations map[string]Relation
}

func (s *Service) Diff(left, right string, loadPath func(string) (*scope.Workspace, error)) (DiffResult, error) {
	leftGraph, err := s.selectSemanticGraph(left, loadPath)
	if err != nil {
		return DiffResult{}, err
	}
	rightGraph, err := s.selectSemanticGraph(right, loadPath)
	if err != nil {
		return DiffResult{}, err
	}
	result := DiffResult{Added: DiffSide{Entities: []Entity{}, Relations: []Relation{}}, Removed: DiffSide{Entities: []Entity{}, Relations: []Relation{}}, Changed: DiffChanged{Entities: []EntityChange{}, Relations: []RelationChange{}}}
	for _, id := range sortedMapKeys(leftGraph.entities) {
		before := leftGraph.entities[id]
		after, exists := rightGraph.entities[id]
		if !exists {
			result.Removed.Entities = append(result.Removed.Entities, before)
		} else if !reflect.DeepEqual(before.Object, after.Object) {
			result.Changed.Entities = append(result.Changed.Entities, EntityChange{before, after})
		}
	}
	for _, id := range sortedMapKeys(rightGraph.entities) {
		if _, exists := leftGraph.entities[id]; !exists {
			result.Added.Entities = append(result.Added.Entities, rightGraph.entities[id])
		}
	}
	for _, id := range sortedMapKeys(leftGraph.relations) {
		before := leftGraph.relations[id]
		after, exists := rightGraph.relations[id]
		if !exists {
			result.Removed.Relations = append(result.Removed.Relations, before)
		} else if !reflect.DeepEqual(before.Object, after.Object) {
			result.Changed.Relations = append(result.Changed.Relations, RelationChange{before, after})
		}
	}
	for _, id := range sortedMapKeys(rightGraph.relations) {
		if _, exists := leftGraph.relations[id]; !exists {
			result.Added.Relations = append(result.Added.Relations, rightGraph.relations[id])
		}
	}
	return result, nil
}

func (s *Service) selectSemanticGraph(selector string, loadPath func(string) (*scope.Workspace, error)) (semanticGraph, error) {
	kind, value, found := strings.Cut(selector, ":")
	if !found || value == "" {
		return semanticGraph{}, fmt.Errorf("invalid diff selector %q", selector)
	}
	switch kind {
	case "scope":
		key, err := s.ResolveScope(value)
		if err != nil {
			return semanticGraph{}, err
		}
		return s.graphForScope(key, ""), nil
	case "group":
		separator := strings.LastIndex(value, "#")
		if separator <= 0 {
			return semanticGraph{}, fmt.Errorf("invalid group diff selector %q", selector)
		}
		key, err := s.ResolveScope(value[:separator])
		if err != nil {
			return semanticGraph{}, err
		}
		return s.graphForScope(key, value[separator+1:]), nil
	case "path":
		workspace, err := loadPath(filepath.FromSlash(value))
		if err != nil {
			return semanticGraph{}, err
		}
		return New(workspace).graphForWorkspace(), nil
	default:
		return semanticGraph{}, fmt.Errorf("unknown diff selector kind %q", kind)
	}
}

func (s *Service) graphForScope(owner scope.ScopeKey, group string) semanticGraph {
	selected := make(map[scope.EntityKey]struct{})
	for id, entity := range s.workspace.Scopes[owner].Entities {
		if group == "" || entity.Source.Group == group {
			selected[scope.EntityKey{Scope: owner, ID: id}] = struct{}{}
		}
	}
	return s.graphForKeys(selected)
}
func (s *Service) graphForWorkspace() semanticGraph {
	selected := make(map[scope.EntityKey]struct{})
	for owner, current := range s.workspace.Scopes {
		for id := range current.Entities {
			selected[scope.EntityKey{Scope: owner, ID: id}] = struct{}{}
		}
	}
	return s.graphForKeys(selected)
}
func (s *Service) graphForKeys(selected map[scope.EntityKey]struct{}) semanticGraph {
	result := semanticGraph{entities: make(map[string]Entity), relations: make(map[string]Relation)}
	for key := range selected {
		item, _ := s.EntityByKey(key, false)
		result.entities[key.ID] = item
	}
	for _, relation := range s.QueryRelations(nil, false) {
		if _, ok := selected[relation.FromKey]; !ok {
			continue
		}
		if _, ok := selected[relation.ToKey]; !ok {
			continue
		}
		relationType, _ := relation.Object["type"].(string)
		identity := relation.FromKey.ID + "\x00" + relationType + "\x00" + relation.ToKey.ID
		result.relations[identity] = relation
	}
	return result
}
func sortedMapKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
