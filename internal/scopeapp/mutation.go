package scopeapp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/natefinch/atomic"
	"gopkg.in/yaml.v3"
	"locus-scope/internal/apperror"
	"locus-scope/internal/scope"
)

type MutationRequest struct {
	Kind   string
	Action string
	Ref    string
	From   string
	Type   string
	To     string
	Object map[string]any
	Fields map[string]any
	Unset  []string
	File   string
}

type MutationResult struct {
	Action string `json:"action"`
	Object any    `json:"object"`
	Source Source `json:"source"`
}

type mutableDocument struct {
	Group     string           `yaml:"group,omitempty" json:"group,omitempty"`
	Entities  []map[string]any `yaml:"entities,omitempty" json:"entities,omitempty"`
	Relations []map[string]any `yaml:"relations,omitempty" json:"relations,omitempty"`
}

func (s *Service) Mutate(request MutationRequest, reload func() (*scope.Workspace, error)) (MutationResult, *scope.Workspace, error) {
	root := s.workspace.Scopes[s.workspace.Root]
	path, localID, relationIndex, err := s.mutationTarget(root, request)
	if err != nil {
		return MutationResult{}, nil, err
	}
	document, original, existed, err := readMutableDocument(path)
	if err != nil {
		return MutationResult{}, nil, err
	}

	var object map[string]any
	switch request.Kind {
	case "entity":
		object, err = mutateEntity(&document, request, localID)
	case "relation":
		object, err = mutateRelation(&document, request, relationIndex)
	default:
		err = fmt.Errorf("unknown mutation kind %q", request.Kind)
	}
	if err != nil {
		return MutationResult{}, nil, err
	}
	encoded, err := encodeMutableDocument(path, document)
	if err != nil {
		return MutationResult{}, nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return MutationResult{}, nil, apperror.Wrap(apperror.Internal, "scope.mutation_write_failed", "create definition directory failed", err, map[string]string{"operation": "scope.mutate"})
	}
	if err := atomic.WriteFile(path, bytes.NewReader(encoded)); err != nil {
		return MutationResult{}, nil, apperror.Wrap(apperror.Internal, "scope.mutation_write_failed", "write definition failed", err, map[string]string{"operation": "scope.mutate"})
	}
	workspace, err := reload()
	if err != nil {
		if existed {
			_ = atomic.WriteFile(path, bytes.NewReader(original))
		} else {
			_ = os.Remove(path)
		}
		_, _ = reload()
		return MutationResult{}, nil, apperror.Wrap(apperror.InvalidData, "scope.mutation_invalid", err.Error(), err, map[string]string{"operation": "scope.mutate"})
	}
	updated := New(workspace)
	result, err := updated.mutationResult(request, object, path)
	if err != nil {
		return MutationResult{}, nil, err
	}
	return result, workspace, nil
}

func (s *Service) mutationTarget(root *scope.Scope, request MutationRequest) (string, string, int, error) {
	if request.Action == "add" {
		path := request.File
		if path == "" {
			path = "scope.locus.yaml"
		}
		if filepath.Ext(path) != ".yaml" || !strings.HasSuffix(filepath.ToSlash(path), ".locus.yaml") {
			return "", "", -1, fmt.Errorf("--file must name a lowercase *.locus.yaml definition")
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(root.LocalPath, filepath.FromSlash(path))
		}
		absolute, err := filepath.Abs(path)
		if err != nil {
			return "", "", -1, err
		}
		relative, err := filepath.Rel(root.LocalPath, absolute)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return "", "", -1, fmt.Errorf("definition path must remain inside root Scope")
		}
		if err := rejectNestedScope(root.LocalPath, absolute); err != nil {
			return "", "", -1, err
		}
		return absolute, "", -1, nil
	}
	if request.Kind == "entity" {
		key, err := s.workspace.Resolve(s.workspace.Root, request.Ref)
		if err != nil {
			return "", "", -1, err
		}
		if key.Scope != s.workspace.Root {
			return "", "", -1, apperror.Message(apperror.PermissionDenied, "scope.object_read_only", "dependency Entity %q is read-only", request.Ref)
		}
		entity := root.Entities[key.ID]
		return entity.Source.Path, entity.LocalID, -1, nil
	}
	from, err := s.workspace.Resolve(s.workspace.Root, request.From)
	if err != nil {
		return "", "", -1, err
	}
	to, err := s.workspace.Resolve(s.workspace.Root, request.To)
	if err != nil {
		return "", "", -1, err
	}
	for _, relation := range s.workspace.Relations {
		if relation.From == from && relation.Type == request.Type && relation.To == to {
			if relation.Source.Scope != s.workspace.Root {
				return "", "", -1, apperror.Message(apperror.PermissionDenied, "scope.object_read_only", "dependency Relation is read-only")
			}
			return relation.Source.Path, "", relation.Source.Index - 1, nil
		}
	}
	return "", "", -1, apperror.Message(apperror.NotFound, "scope.object_not_found", "relation %s %s %s not found", request.From, request.Type, request.To)
}

func readMutableDocument(path string) (mutableDocument, []byte, bool, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return mutableDocument{}, nil, false, nil
	}
	if err != nil {
		return mutableDocument{}, nil, false, fmt.Errorf("read definition %s: %w", path, err)
	}
	var raw struct {
		Group     string           `yaml:"group"`
		Entities  []map[string]any `yaml:"entities"`
		Relations []map[string]any `yaml:"relations"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return mutableDocument{}, nil, true, fmt.Errorf("decode definition %s: %w", path, err)
	}
	document := mutableDocument{Group: raw.Group, Entities: raw.Entities, Relations: raw.Relations}
	return document, data, true, nil
}

func encodeMutableDocument(path string, document mutableDocument) ([]byte, error) {
	if strings.HasSuffix(path, ".json") {
		data, err := json.MarshalIndent(document, "", "  ")
		if err != nil {
			return nil, err
		}
		return append(data, '\n'), nil
	}
	data, err := yaml.Marshal(document)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func mutateEntity(document *mutableDocument, request MutationRequest, localID string) (map[string]any, error) {
	if request.Action == "add" {
		object := cloneMap(request.Object)
		for key, value := range request.Fields {
			if err := setPath(object, key, value); err != nil {
				return nil, err
			}
		}
		id, ok := object["id"].(string)
		if !ok || strings.TrimSpace(id) == "" {
			return nil, fmt.Errorf("entity id is required")
		}
		local, err := localEntityID(id, document.Group)
		if err != nil {
			return nil, err
		}
		object["id"] = local
		for _, existing := range document.Entities {
			if existing["id"] == local {
				return nil, fmt.Errorf("entity %q already exists in target document", id)
			}
		}
		document.Entities = append(document.Entities, object)
		result := cloneMap(object)
		result["id"] = id
		return result, nil
	}
	index := -1
	for i, object := range document.Entities {
		if object["id"] == localID {
			index = i
			break
		}
	}
	if index < 0 {
		return nil, fmt.Errorf("entity declaration %q not found", localID)
	}
	if request.Action == "remove" {
		result := cloneMap(document.Entities[index])
		document.Entities = append(document.Entities[:index], document.Entities[index+1:]...)
		return result, nil
	}
	object := document.Entities[index]
	if err := applyObjectMutation(object, request, map[string]struct{}{"id": {}}); err != nil {
		return nil, err
	}
	return cloneMap(object), nil
}

func mutateRelation(document *mutableDocument, request MutationRequest, index int) (map[string]any, error) {
	if request.Action == "add" {
		object := cloneMap(request.Object)
		for key, value := range request.Fields {
			if err := setPath(object, key, value); err != nil {
				return nil, err
			}
		}
		for _, field := range []string{"from", "type", "to"} {
			if text, ok := object[field].(string); !ok || strings.TrimSpace(text) == "" {
				return nil, fmt.Errorf("relation %s is required", field)
			}
		}
		document.Relations = append(document.Relations, object)
		return cloneMap(object), nil
	}
	if index < 0 || index >= len(document.Relations) {
		return nil, fmt.Errorf("relation declaration not found")
	}
	if request.Action == "remove" {
		result := cloneMap(document.Relations[index])
		document.Relations = append(document.Relations[:index], document.Relations[index+1:]...)
		return result, nil
	}
	object := document.Relations[index]
	if err := applyObjectMutation(object, request, map[string]struct{}{"from": {}, "type": {}, "to": {}}); err != nil {
		return nil, err
	}
	return cloneMap(object), nil
}

func applyObjectMutation(object map[string]any, request MutationRequest, protected map[string]struct{}) error {
	if request.Action == "set" {
		patch := cloneMap(request.Object)
		for key := range protected {
			delete(patch, key)
		}
		mergeMap(object, patch)
		for key, value := range request.Fields {
			if _, locked := protected[key]; locked {
				return fmt.Errorf("structural field %q cannot be changed", key)
			}
			if err := setPath(object, key, value); err != nil {
				return err
			}
		}
		return nil
	}
	if request.Action == "unset" {
		for _, key := range request.Unset {
			if _, locked := protected[key]; locked {
				return fmt.Errorf("structural field %q cannot be unset", key)
			}
			if err := unsetPath(object, key); err != nil {
				return err
			}
		}
		return nil
	}
	return fmt.Errorf("unknown mutation action %q", request.Action)
}

func (s *Service) mutationResult(request MutationRequest, object map[string]any, path string) (MutationResult, error) {
	withSource := true
	if request.Kind == "entity" {
		ref := request.Ref
		if request.Action == "add" {
			ref, _ = object["id"].(string)
		}
		item, err := s.GetEntity(ref, withSource)
		if err != nil {
			if request.Action == "remove" {
				return MutationResult{Action: request.Action, Object: object, Source: Source{Scope: s.workspace.Root, ScopeID: s.workspace.Scopes[s.workspace.Root].Manifest.ID, File: filepath.ToSlash(mustRelative(s.workspace.Scopes[s.workspace.Root].LocalPath, path))}}, nil
			}
			return MutationResult{}, err
		}
		return MutationResult{Action: request.Action, Object: item, Source: *item.Source}, nil
	}
	from, relationType, to := request.From, request.Type, request.To
	if request.Action == "add" {
		from, _ = object["from"].(string)
		relationType, _ = object["type"].(string)
		to, _ = object["to"].(string)
	}
	fromKey, err := s.workspace.Resolve(s.workspace.Root, from)
	if err != nil {
		return MutationResult{}, err
	}
	toKey, err := s.workspace.Resolve(s.workspace.Root, to)
	if err != nil {
		return MutationResult{}, err
	}
	for _, item := range s.QueryRelations(nil, true) {
		if item.FromKey == fromKey && item.ToKey == toKey && item.Object["type"] == relationType {
			return MutationResult{Action: request.Action, Object: item, Source: *item.Source}, nil
		}
	}
	return MutationResult{Action: request.Action, Object: object, Source: Source{Scope: s.workspace.Root, ScopeID: s.workspace.Scopes[s.workspace.Root].Manifest.ID, File: filepath.ToSlash(mustRelative(s.workspace.Scopes[s.workspace.Root].LocalPath, path))}}, nil
}

func localEntityID(id, group string) (string, error) {
	if group == "" {
		if strings.Contains(id, "/") {
			return "", fmt.Errorf("entity id %q does not belong to root group", id)
		}
		return id, nil
	}
	prefix := group + "/"
	if !strings.HasPrefix(id, prefix) || strings.TrimPrefix(id, prefix) == "" {
		return "", fmt.Errorf("entity id %q does not belong to group %q", id, group)
	}
	return strings.TrimPrefix(id, prefix), nil
}
func mergeMap(target, patch map[string]any) {
	for key, value := range patch {
		if child, ok := value.(map[string]any); ok {
			existing, exists := target[key].(map[string]any)
			if !exists {
				existing = make(map[string]any)
				target[key] = existing
			}
			mergeMap(existing, child)
		} else {
			target[key] = cloneValue(value)
		}
	}
}
func setPath(object map[string]any, path string, value any) error {
	parts := strings.Split(path, ".")
	current := object
	for _, part := range parts[:len(parts)-1] {
		if part == "" {
			return fmt.Errorf("invalid field path %q", path)
		}
		next, exists := current[part]
		if !exists {
			child := make(map[string]any)
			current[part] = child
			current = child
			continue
		}
		child, ok := next.(map[string]any)
		if !ok {
			return fmt.Errorf("field path %q crosses non-object %q", path, part)
		}
		current = child
	}
	if parts[len(parts)-1] == "" {
		return fmt.Errorf("invalid field path %q", path)
	}
	current[parts[len(parts)-1]] = cloneValue(value)
	return nil
}
func unsetPath(object map[string]any, path string) error {
	parts := strings.Split(path, ".")
	current := object
	for _, part := range parts[:len(parts)-1] {
		next, exists := current[part]
		if !exists {
			return fmt.Errorf("field %q does not exist", path)
		}
		child, ok := next.(map[string]any)
		if !ok {
			return fmt.Errorf("field path %q crosses non-object %q", path, part)
		}
		current = child
	}
	if _, exists := current[parts[len(parts)-1]]; !exists {
		return fmt.Errorf("field %q does not exist", path)
	}
	delete(current, parts[len(parts)-1])
	return nil
}
func mustRelative(root, path string) string { relative, _ := filepath.Rel(root, path); return relative }

func rejectNestedScope(root, target string) error {
	for directory := filepath.Dir(target); directory != root; directory = filepath.Dir(directory) {
		if directory == "." || directory == string(filepath.Separator) || len(directory) < len(root) {
			return fmt.Errorf("definition path must remain inside root Scope")
		}
		for _, name := range []string{"locus.yaml", "locus.yml", "locus.json"} {
			if _, err := os.Stat(filepath.Join(directory, name)); err == nil {
				return fmt.Errorf("definition path enters nested Scope %s", directory)
			} else if !os.IsNotExist(err) {
				return fmt.Errorf("inspect nested Scope %s: %w", directory, err)
			}
		}
	}
	return nil
}
