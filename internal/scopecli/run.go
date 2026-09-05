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
	"locus-scope/internal/scopeapp"
)

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
	if err := execute(scopeapp.New(workspace), command, jsonOutput, stdout); err != nil {
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

func execute(app *scopeapp.Service, command []string, jsonOutput bool, output io.Writer) error {
	switch {
	case len(command) == 1 && command[0] == "validate":
		return showValidation(app.Validate(), jsonOutput, output)
	case len(command) == 2 && command[0] == "scope" && command[1] == "show":
		return showRootScope(app.RootScope(), jsonOutput, output)
	case len(command) == 2 && command[0] == "scope" && command[1] == "list":
		return listScopes(app.ListScopes(), jsonOutput, output)
	case len(command) == 2 && command[0] == "entity" && command[1] == "list":
		return listEntities(app.ListEntities(), jsonOutput, output)
	case len(command) == 3 && command[0] == "entity" && command[1] == "show":
		result, err := app.GetEntity(command[2])
		if err != nil {
			return err
		}
		return showEntity(result, jsonOutput, output)
	case len(command) == 2 && command[0] == "relation" && command[1] == "list":
		return listRelations(app.ListRelations(), jsonOutput, output)
	case len(command) == 2 && command[0] == "resolve":
		result, err := app.ResolveEntity(command[1])
		if err != nil {
			return err
		}
		return resolveEntity(result, jsonOutput, output)
	default:
		return fmt.Errorf("unknown command %q; run locus-scope help", strings.Join(command, " "))
	}
}

func showValidation(result scopeapp.ValidationResult, jsonOutput bool, output io.Writer) error {
	if jsonOutput {
		return writeJSON(output, result)
	}
	_, err := fmt.Fprintf(output, "valid: %s (%d scopes, %d entities, %d relations)\n",
		result.Root, result.Scopes, result.Entities, result.Relations)
	return err
}

func showRootScope(result scopeapp.Scope, jsonOutput bool, output io.Writer) error {
	if jsonOutput {
		return writeJSON(output, result)
	}
	if _, err := fmt.Fprintf(output, "id: %s\nsource: %s\n", result.ID, result.Source); err != nil {
		return err
	}
	fmt.Fprintln(output, "imports:")
	for _, imported := range result.Imports {
		fmt.Fprintf(output, "  %s: %s -> %s\n", imported.Alias, imported.Source, imported.Target)
	}
	fmt.Fprintln(output, "exports:")
	for _, exported := range result.Exports {
		fmt.Fprintf(output, "  %s\n", exported)
	}
	return nil
}

func listScopes(result scopeapp.ScopesResult, jsonOutput bool, output io.Writer) error {
	if jsonOutput {
		return writeJSON(output, result)
	}
	for _, item := range result.Scopes {
		marker := " "
		if item.Root {
			marker = "*"
		}
		fmt.Fprintf(output, "%s %s\t%s\n", marker, item.ID, item.Source)
	}
	return nil
}

func listEntities(result scopeapp.EntitiesResult, jsonOutput bool, output io.Writer) error {
	if jsonOutput {
		return writeJSON(output, result)
	}
	for _, entity := range result.Entities {
		fmt.Fprintf(output, "%s\t%s\t%s\n", entity.ScopeID, entity.ID, entity.Scope)
	}
	return nil
}

func showEntity(result scopeapp.EntityResult, jsonOutput bool, output io.Writer) error {
	if jsonOutput {
		return writeJSON(output, result)
	}
	entity := result.Entity
	fmt.Fprintf(output, "id: %s\nowner: %s (%s)\nproperties:\n", entity.ID, entity.ScopeID, entity.Scope)
	propertyNames := make([]string, 0, len(entity.Properties))
	for name := range entity.Properties {
		propertyNames = append(propertyNames, name)
	}
	sort.Strings(propertyNames)
	for _, name := range propertyNames {
		encoded, err := json.Marshal(entity.Properties[name])
		if err != nil {
			return fmt.Errorf("encode property %q: %w", name, err)
		}
		fmt.Fprintf(output, "  %s: %s\n", name, encoded)
	}
	return nil
}

func listRelations(result scopeapp.RelationsResult, jsonOutput bool, output io.Writer) error {
	if jsonOutput {
		return writeJSON(output, result)
	}
	for _, relation := range result.Relations {
		fmt.Fprintf(output, "%s:%s (%s) %s %s:%s (%s)\n",
			relation.From.ScopeID, relation.From.ID, relation.From.Scope,
			relation.Name,
			relation.To.ScopeID, relation.To.ID, relation.To.Scope)
	}
	return nil
}

func resolveEntity(result scopeapp.ResolveResult, jsonOutput bool, output io.Writer) error {
	if jsonOutput {
		return writeJSON(output, result)
	}
	_, err := fmt.Fprintf(output, "%s:%s\nowner: %s\n", result.Entity.ScopeID, result.Entity.ID, result.Entity.Scope)
	return err
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
