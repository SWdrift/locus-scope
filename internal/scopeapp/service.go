// Package scopeapp exposes transport-independent Scope management operations.
package scopeapp

import (
	"sort"
	"strings"

	"locus-scope/internal/apperror"
	"locus-scope/internal/scope"
)

type Service struct {
	workspace  *scope.Workspace
	scopeRefs  map[scope.ScopeKey]string
	entityRefs map[scope.EntityKey]string
}

type ValidationResult struct {
	Valid     bool           `json:"valid"`
	Root      scope.ScopeKey `json:"root"`
	Scopes    int            `json:"scopes"`
	Entities  int            `json:"entities"`
	Relations int            `json:"relations"`
}

type Source struct {
	Scope   scope.ScopeKey `json:"scope"`
	ScopeID string         `json:"scopeId"`
	File    string         `json:"file"`
	Group   string         `json:"group,omitempty"`
	Line    int            `json:"line,omitempty"`
	Index   int            `json:"index,omitempty"`
}

type Entity struct {
	Key    scope.EntityKey `json:"key"`
	Ref    string          `json:"ref,omitempty"`
	Object map[string]any  `json:"object"`
	Source *Source         `json:"source,omitempty"`
}

type Relation struct {
	FromKey scope.EntityKey `json:"fromKey"`
	ToKey   scope.EntityKey `json:"toKey"`
	Object  map[string]any  `json:"object"`
	Source  *Source         `json:"source,omitempty"`
}

type Scope struct {
	Key    scope.ScopeKey `json:"key"`
	Ref    string         `json:"ref"`
	Object map[string]any `json:"object"`
	Source *Source        `json:"source,omitempty"`
}

type Group struct {
	Scope   scope.ScopeKey `json:"scope"`
	Ref     string         `json:"ref"`
	Object  map[string]any `json:"object"`
	Sources []Source       `json:"sources,omitempty"`
}

func New(workspace *scope.Workspace) *Service {
	s := &Service{workspace: workspace}
	if workspace != nil {
		s.scopeRefs = makeScopeRefs(workspace)
		s.entityRefs = makeEntityRefs(workspace, s.scopeRefs)
	}
	return s
}

func (s *Service) Workspace() *scope.Workspace { return s.workspace }

func (s *Service) Validate() ValidationResult {
	entities := 0
	for _, current := range s.workspace.Scopes {
		entities += len(current.Entities)
	}
	return ValidationResult{true, s.workspace.Root, len(s.workspace.Scopes), entities, len(s.workspace.Relations)}
}

func (s *Service) QueryScopes(predicates []scope.Predicate, withSource bool) []Scope {
	keys := sortedScopeKeys(s.workspace)
	result := make([]Scope, 0, len(keys))
	for _, key := range keys {
		current := s.workspace.Scopes[key]
		object := map[string]any{"id": current.Manifest.ID, "imports": current.Manifest.Imports, "exports": current.Manifest.Exports, "root": key == s.workspace.Root}
		metadata := map[string]any{"scope": s.scopeRefs[key]}
		if !scope.Match(object, metadata, predicates) {
			continue
		}
		item := Scope{Key: key, Ref: s.scopeRefs[key], Object: object}
		if withSource {
			item.Source = &Source{Scope: key, ScopeID: current.Manifest.ID, File: manifestFile(current)}
		}
		result = append(result, item)
	}
	return result
}

func (s *Service) GetScope(reference string, withSource bool) (Scope, error) {
	key, err := s.ResolveScope(reference)
	if err != nil {
		return Scope{}, err
	}
	for _, item := range s.QueryScopes(nil, withSource) {
		if item.Key == key {
			return item, nil
		}
	}
	return Scope{}, apperror.Message(apperror.NotFound, "scope.object_not_found", "scope %q not found", reference)
}

func (s *Service) QueryGroups(predicates []scope.Predicate, withSource bool) []Group {
	type groupKey struct {
		scope scope.ScopeKey
		group string
	}
	groups := make(map[groupKey][]Source)
	for key, current := range s.workspace.Scopes {
		for _, entity := range current.Entities {
			if entity.Source.Group == "" {
				continue
			}
			k := groupKey{key, entity.Source.Group}
			sourceValue := makeSource(s.workspace, entity.Source)
			found := false
			for _, existing := range groups[k] {
				if existing.File == sourceValue.File {
					found = true
					break
				}
			}
			if !found {
				groups[k] = append(groups[k], sourceValue)
			}
		}
	}
	keys := make([]groupKey, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].scope != keys[j].scope {
			return keys[i].scope < keys[j].scope
		}
		return keys[i].group < keys[j].group
	})
	result := make([]Group, 0, len(keys))
	for _, key := range keys {
		ref := s.scopeRefs[key.scope] + "#" + key.group
		object := map[string]any{"id": key.group}
		metadata := map[string]any{"scope": s.scopeRefs[key.scope], "group": key.group}
		if !scope.Match(object, metadata, predicates) {
			continue
		}
		item := Group{Scope: key.scope, Ref: ref, Object: object}
		if withSource {
			sort.Slice(groups[key], func(i, j int) bool { return groups[key][i].File < groups[key][j].File })
			item.Sources = groups[key]
		}
		result = append(result, item)
	}
	return result
}

func (s *Service) GetGroup(reference string, withSource bool) (Group, error) {
	separator := strings.LastIndex(reference, "#")
	if separator <= 0 || separator == len(reference)-1 {
		return Group{}, apperror.Message(apperror.InvalidArgument, "scope.selector_invalid", "invalid group reference %q", reference)
	}
	key, err := s.ResolveScope(reference[:separator])
	if err != nil {
		return Group{}, err
	}
	group := reference[separator+1:]
	for _, item := range s.QueryGroups(nil, withSource) {
		if item.Scope == key && item.Object["id"] == group {
			return item, nil
		}
	}
	return Group{}, apperror.Message(apperror.NotFound, "scope.object_not_found", "group %q not found", reference)
}

func (s *Service) QueryEntities(predicates []scope.Predicate, withSource bool) []Entity {
	keys := make([]scope.EntityKey, 0)
	for owner, current := range s.workspace.Scopes {
		for id := range current.Entities {
			keys = append(keys, scope.EntityKey{Scope: owner, ID: id})
		}
	}
	sortEntityKeys(keys)
	result := make([]Entity, 0, len(keys))
	for _, key := range keys {
		entity := s.workspace.Scopes[key.Scope].Entities[key.ID]
		object := cloneMap(entity.Properties)
		object["id"] = entity.ID
		metadata := map[string]any{"scope": s.scopeRefs[key.Scope], "group": entity.Source.Group}
		if !scope.Match(object, metadata, predicates) {
			continue
		}
		item := Entity{Key: key, Ref: s.entityRefs[key], Object: object}
		if withSource {
			sourceValue := makeSource(s.workspace, entity.Source)
			item.Source = &sourceValue
		}
		result = append(result, item)
	}
	return result
}

func (s *Service) GetEntity(reference string, withSource bool) (Entity, error) {
	key, err := s.workspace.Resolve(s.workspace.Root, reference)
	if err != nil {
		return Entity{}, apperror.Wrap(apperror.NotFound, "scope.object_not_found", err.Error(), err, map[string]string{"reference": reference})
	}
	return s.EntityByKey(key, withSource)
}

func (s *Service) EntityByKey(key scope.EntityKey, withSource bool) (Entity, error) {
	for _, item := range s.QueryEntities(nil, withSource) {
		if item.Key == key {
			return item, nil
		}
	}
	return Entity{}, apperror.Message(apperror.NotFound, "scope.object_not_found", "entity %s#%s not found", key.Scope, key.ID)
}

func (s *Service) QueryRelations(predicates []scope.Predicate, withSource bool) []Relation {
	result := make([]Relation, 0, len(s.workspace.Relations))
	for _, relation := range s.workspace.Relations {
		object := cloneMap(relation.Properties)
		object["from"], object["type"], object["to"] = relation.FromRef, relation.Type, relation.ToRef
		metadata := map[string]any{"scope": s.scopeRefs[relation.Source.Scope], "fromScope": s.scopeRefs[relation.From.Scope], "toScope": s.scopeRefs[relation.To.Scope]}
		if !scope.Match(object, metadata, predicates) {
			continue
		}
		item := Relation{FromKey: relation.From, ToKey: relation.To, Object: object}
		if withSource {
			sourceValue := makeSource(s.workspace, relation.Source)
			item.Source = &sourceValue
		}
		result = append(result, item)
	}
	return result
}

func (s *Service) ResolveScope(reference string) (scope.ScopeKey, error) {
	if reference == "." {
		return s.workspace.Root, nil
	}
	current := s.workspace.Root
	for _, alias := range strings.Split(reference, ":") {
		next, ok := s.workspace.Scopes[current].Imports[alias]
		if !ok {
			return "", apperror.Message(apperror.NotFound, "scope.object_not_found", "scope reference %q does not resolve at alias %q", reference, alias)
		}
		current = next
	}
	return current, nil
}

func makeSource(workspace *scope.Workspace, provenance scope.Provenance) Source {
	return Source{Scope: provenance.Scope, ScopeID: workspace.Scopes[provenance.Scope].Manifest.ID, File: provenance.File, Group: provenance.Group, Line: provenance.Line, Index: provenance.Index}
}

func manifestFile(current *scope.Scope) string { return current.ManifestFile }

func makeScopeRefs(workspace *scope.Workspace) map[scope.ScopeKey]string {
	refs := map[scope.ScopeKey]string{workspace.Root: "."}
	queue := []scope.ScopeKey{workspace.Root}
	for len(queue) != 0 {
		current := queue[0]
		queue = queue[1:]
		aliases := make([]string, 0, len(workspace.Scopes[current].Imports))
		for alias := range workspace.Scopes[current].Imports {
			aliases = append(aliases, alias)
		}
		sort.Strings(aliases)
		for _, alias := range aliases {
			target := workspace.Scopes[current].Imports[alias]
			candidate := alias
			if refs[current] != "." {
				candidate = refs[current] + ":" + alias
			}
			if existing, ok := refs[target]; !ok || candidate < existing {
				refs[target] = candidate
				queue = append(queue, target)
			}
		}
	}
	return refs
}

func makeEntityRefs(workspace *scope.Workspace, scopeRefs map[scope.ScopeKey]string) map[scope.EntityKey]string {
	refs := make(map[scope.EntityKey]string)
	root := workspace.Scopes[workspace.Root]
	for id := range root.Entities {
		refs[scope.EntityKey{Scope: workspace.Root, ID: id}] = id
	}
	keys := sortedScopeKeys(workspace)
	for _, key := range keys {
		if key == workspace.Root {
			continue
		}
		prefix := scopeRefs[key]
		for _, exported := range workspace.Scopes[key].Manifest.Exports {
			candidate := prefix + ":" + exported
			resolved, err := workspace.Resolve(workspace.Root, candidate)
			if err == nil {
				if previous, ok := refs[resolved]; !ok || candidate < previous {
					refs[resolved] = candidate
				}
			}
		}
	}
	return refs
}

func sortedScopeKeys(workspace *scope.Workspace) []scope.ScopeKey {
	keys := make([]scope.ScopeKey, 0, len(workspace.Scopes))
	for key := range workspace.Scopes {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	return keys
}
func sortEntityKeys(keys []scope.EntityKey) {
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Scope != keys[j].Scope {
			return keys[i].Scope < keys[j].Scope
		}
		return keys[i].ID < keys[j].ID
	})
}
func cloneMap(value map[string]any) map[string]any {
	result := make(map[string]any, len(value)+1)
	for key, item := range value {
		result[key] = cloneValue(item)
	}
	return result
}
func cloneValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneMap(typed)
	case []any:
		result := make([]any, len(typed))
		for index, item := range typed {
			result[index] = cloneValue(item)
		}
		return result
	default:
		return value
	}
}
