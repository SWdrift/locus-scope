// Package scopecli implements the shared Workspace inspection command dispatcher.
package scopecli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"locus-scope/internal/buildinfo"
	"locus-scope/internal/scope"
)

type importView struct {
	Alias  string         `json:"alias"`
	Source string         `json:"source"`
	Target scope.ScopeKey `json:"target"`
}

type scopeView struct {
	ID      string         `json:"id"`
	Source  scope.ScopeKey `json:"source"`
	Root    bool           `json:"root"`
	Imports []importView   `json:"imports"`
	Exports []string       `json:"exports"`
}

type entityView struct {
	ScopeID string         `json:"scope_id"`
	Scope   scope.ScopeKey `json:"scope"`
	ID      string         `json:"id"`
}

type entityDetailView struct {
	ScopeID    string         `json:"scope_id"`
	Scope      scope.ScopeKey `json:"scope"`
	ID         string         `json:"id"`
	Properties map[string]any `json:"properties"`
}

type relationView struct {
	From entityKeyView `json:"from"`
	Name string        `json:"name"`
	To   entityKeyView `json:"to"`
}

type entityKeyView struct {
	ScopeID string         `json:"scope_id"`
	Scope   scope.ScopeKey `json:"scope"`
	ID      string         `json:"id"`
}

// Run executes one query against an already loaded workspace.
func Run(workspace *scope.Workspace, arguments []string, stdout, stderr io.Writer) int {
	jsonOutput, command, err := parseArguments(arguments)
	if err != nil {
		writeFailure(stderr, jsonOutput, err)
		return 2
	}
	if len(command) == 0 || command[0] == "help" {
		_, _ = fmt.Fprint(stdout, Usage)
		return 0
	}
	if len(command) == 1 && command[0] == "version" {
		if err := buildinfo.WriteVersion(stdout, "locus-scope", jsonOutput); err != nil {
			writeFailure(stderr, jsonOutput, err)
			return 1
		}
		return 0
	}
	if workspace == nil {
		writeFailure(stderr, jsonOutput, errors.New("Scope workspace is required"))
		return 1
	}
	if err := execute(workspace, command, jsonOutput, stdout); err != nil {
		writeFailure(stderr, jsonOutput, err)
		return 2
	}
	return 0
}

func parseArguments(arguments []string) (bool, []string, error) {
	jsonOutput := false
	command := make([]string, 0, len(arguments))
	for _, argument := range arguments {
		switch {
		case argument == "--json":
			jsonOutput = true
		case argument == "--help" || argument == "-h":
			command = []string{"help"}
		case argument == "--version":
			command = []string{"version"}
		case strings.HasPrefix(argument, "-"):
			return jsonOutput, nil, fmt.Errorf("unknown option %q", argument)
		default:
			command = append(command, argument)
		}
	}
	return jsonOutput, command, nil
}

func execute(workspace *scope.Workspace, command []string, jsonOutput bool, output io.Writer) error {
	switch {
	case len(command) == 1 && command[0] == "validate":
		return showValidation(workspace, jsonOutput, output)
	case len(command) == 2 && command[0] == "scope" && command[1] == "show":
		return showRootScope(workspace, jsonOutput, output)
	case len(command) == 2 && command[0] == "scope" && command[1] == "list":
		return listScopes(workspace, jsonOutput, output)
	case len(command) == 2 && command[0] == "entity" && command[1] == "list":
		return listEntities(workspace, jsonOutput, output)
	case len(command) == 3 && command[0] == "entity" && command[1] == "show":
		return showEntity(workspace, command[2], jsonOutput, output)
	case len(command) == 2 && command[0] == "relation" && command[1] == "list":
		return listRelations(workspace, jsonOutput, output)
	case len(command) == 2 && command[0] == "resolve":
		return resolveEntity(workspace, command[1], jsonOutput, output)
	default:
		return fmt.Errorf("unknown command %q; run locus-scope help", strings.Join(command, " "))
	}
}

func showValidation(workspace *scope.Workspace, jsonOutput bool, output io.Writer) error {
	entityCount := 0
	for _, loaded := range workspace.Scopes {
		entityCount += len(loaded.Entities)
	}
	view := struct {
		Valid     bool           `json:"valid"`
		Root      scope.ScopeKey `json:"root"`
		Scopes    int            `json:"scopes"`
		Entities  int            `json:"entities"`
		Relations int            `json:"relations"`
	}{true, workspace.Root, len(workspace.Scopes), entityCount, len(workspace.Relations)}
	if jsonOutput {
		return writeJSON(output, view)
	}
	_, err := fmt.Fprintf(output, "valid: %s (%d scopes, %d entities, %d relations)\n",
		workspace.Root, view.Scopes, view.Entities, view.Relations)
	return err
}

func showRootScope(workspace *scope.Workspace, jsonOutput bool, output io.Writer) error {
	view := makeScopeView(workspace, workspace.Root)
	if jsonOutput {
		return writeJSON(output, view)
	}
	if _, err := fmt.Fprintf(output, "id: %s\nsource: %s\n", view.ID, view.Source); err != nil {
		return err
	}
	fmt.Fprintln(output, "imports:")
	for _, imported := range view.Imports {
		fmt.Fprintf(output, "  %s: %s -> %s\n", imported.Alias, imported.Source, imported.Target)
	}
	fmt.Fprintln(output, "exports:")
	for _, exported := range view.Exports {
		fmt.Fprintf(output, "  %s\n", exported)
	}
	return nil
}

func listScopes(workspace *scope.Workspace, jsonOutput bool, output io.Writer) error {
	keys := scopeKeys(workspace)
	views := make([]scopeView, 0, len(keys))
	for _, key := range keys {
		views = append(views, makeScopeView(workspace, key))
	}
	if jsonOutput {
		return writeJSON(output, struct {
			Scopes []scopeView `json:"scopes"`
		}{views})
	}
	for _, view := range views {
		marker := " "
		if view.Root {
			marker = "*"
		}
		fmt.Fprintf(output, "%s %s\t%s\n", marker, view.ID, view.Source)
	}
	return nil
}

func makeScopeView(workspace *scope.Workspace, key scope.ScopeKey) scopeView {
	loaded := workspace.Scopes[key]
	aliases := make([]string, 0, len(loaded.Imports))
	for alias := range loaded.Imports {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	imports := make([]importView, 0, len(aliases))
	for _, alias := range aliases {
		imports = append(imports, importView{Alias: alias, Source: loaded.Manifest.Imports[alias], Target: loaded.Imports[alias]})
	}
	exports := append([]string(nil), loaded.Manifest.Exports...)
	sort.Strings(exports)
	return scopeView{ID: loaded.Manifest.ID, Source: key, Root: key == workspace.Root, Imports: imports, Exports: exports}
}

func listEntities(workspace *scope.Workspace, jsonOutput bool, output io.Writer) error {
	views := entityViews(workspace)
	if jsonOutput {
		return writeJSON(output, struct {
			Entities []entityView `json:"entities"`
		}{views})
	}
	for _, view := range views {
		fmt.Fprintf(output, "%s\t%s\t%s\n", view.ScopeID, view.ID, view.Scope)
	}
	return nil
}

func entityViews(workspace *scope.Workspace) []entityView {
	var views []entityView
	for _, key := range scopeKeys(workspace) {
		loaded := workspace.Scopes[key]
		ids := make([]string, 0, len(loaded.Entities))
		for id := range loaded.Entities {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			views = append(views, entityView{ScopeID: loaded.Manifest.ID, Scope: key, ID: id})
		}
	}
	return views
}

func showEntity(workspace *scope.Workspace, reference string, jsonOutput bool, output io.Writer) error {
	key, err := workspace.Resolve(workspace.Root, reference)
	if err != nil {
		return fmt.Errorf("resolve entity %q: %w", reference, err)
	}
	loaded := workspace.Scopes[key.Scope]
	entity := loaded.Entities[key.ID]
	view := entityDetailView{ScopeID: loaded.Manifest.ID, Scope: key.Scope, ID: key.ID, Properties: entity.Properties}
	if jsonOutput {
		return writeJSON(output, struct {
			Reference string           `json:"reference"`
			Entity    entityDetailView `json:"entity"`
		}{reference, view})
	}
	fmt.Fprintf(output, "id: %s\nowner: %s (%s)\nproperties:\n", view.ID, view.ScopeID, view.Scope)
	propertyNames := make([]string, 0, len(view.Properties))
	for name := range view.Properties {
		propertyNames = append(propertyNames, name)
	}
	sort.Strings(propertyNames)
	for _, name := range propertyNames {
		encoded, err := json.Marshal(view.Properties[name])
		if err != nil {
			return fmt.Errorf("encode property %q: %w", name, err)
		}
		fmt.Fprintf(output, "  %s: %s\n", name, encoded)
	}
	return nil
}

func listRelations(workspace *scope.Workspace, jsonOutput bool, output io.Writer) error {
	views := make([]relationView, 0, len(workspace.Relations))
	for _, relation := range workspace.Relations {
		views = append(views, relationView{
			From: makeEntityKeyView(workspace, relation.From),
			Name: relation.Name,
			To:   makeEntityKeyView(workspace, relation.To),
		})
	}
	if jsonOutput {
		return writeJSON(output, struct {
			Relations []relationView `json:"relations"`
		}{views})
	}
	for _, view := range views {
		fmt.Fprintf(output, "%s:%s (%s) %s %s:%s (%s)\n",
			view.From.ScopeID, view.From.ID, view.From.Scope,
			view.Name,
			view.To.ScopeID, view.To.ID, view.To.Scope)
	}
	return nil
}

func resolveEntity(workspace *scope.Workspace, reference string, jsonOutput bool, output io.Writer) error {
	key, err := workspace.Resolve(workspace.Root, reference)
	if err != nil {
		return fmt.Errorf("resolve %q: %w", reference, err)
	}
	view := makeEntityKeyView(workspace, key)
	if jsonOutput {
		return writeJSON(output, struct {
			Reference string        `json:"reference"`
			Entity    entityKeyView `json:"entity"`
		}{reference, view})
	}
	_, err = fmt.Fprintf(output, "%s:%s\nowner: %s\n", view.ScopeID, view.ID, view.Scope)
	return err
}

func makeEntityKeyView(workspace *scope.Workspace, key scope.EntityKey) entityKeyView {
	return entityKeyView{ScopeID: workspace.Scopes[key.Scope].Manifest.ID, Scope: key.Scope, ID: key.ID}
}

func scopeKeys(workspace *scope.Workspace) []scope.ScopeKey {
	keys := make([]scope.ScopeKey, 0, len(workspace.Scopes))
	for key := range workspace.Scopes {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	return keys
}

func writeJSON(output io.Writer, value any) error {
	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	return encoder.Encode(value)
}

func writeFailure(output io.Writer, jsonOutput bool, err error) {
	if jsonOutput {
		_ = writeJSON(output, struct {
			Error string `json:"error"`
		}{err.Error()})
		return
	}
	fmt.Fprintf(output, "locus-scope: %v\n", err)
}

// Usage is the stable locus-scope command help.
const Usage = `locus-scope loads and inspects local Scope workspaces.

Usage:
  locus-scope [--scope <dir>] [--json] validate
  locus-scope [--scope <dir>] [--json] scope show
  locus-scope [--scope <dir>] [--json] scope list
  locus-scope [--scope <dir>] [--json] entity list
  locus-scope [--scope <dir>] [--json] entity show <ref>
  locus-scope [--scope <dir>] [--json] relation list
  locus-scope [--scope <dir>] [--json] resolve <ref>
  locus-scope [--json] version
  locus-scope help
`
