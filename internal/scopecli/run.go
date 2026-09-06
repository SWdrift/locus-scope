// Package scopecli implements the shared locus-scope management command surface.
package scopecli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"

	"locus-scope/internal/apperror"
	"locus-scope/internal/buildinfo"
	"locus-scope/internal/scope"
	"locus-scope/internal/scopeapp"
)

type Context struct {
	Stdin            io.Reader
	WorkingDirectory string
	Load             func() (*scope.Workspace, error)
	Reload           func() (*scope.Workspace, error)
	LoadPath         func(string) (*scope.Workspace, error)
}

type runOptions struct{ json, source bool }

func Run(context Context, arguments []string, stdout, stderr io.Writer) int {
	options, command, err := parseGlobalArguments(arguments)
	if err != nil {
		return fail(stderr, options.json, apperror.Invalid("scope.selector_invalid", err))
	}
	if len(command) == 0 || command[0] == "help" {
		_, _ = fmt.Fprint(stdout, Usage)
		return 0
	}
	if len(command) == 1 && command[0] == "version" {
		if err := buildinfo.WriteVersion(stdout, "locus-scope", options.json); err != nil {
			return fail(stderr, options.json, err)
		}
		return 0
	}
	result, err := execute(context, command, options)
	if err != nil {
		return fail(stderr, options.json, err)
	}
	if err := writeResult(stdout, result, options.json); err != nil {
		return fail(stderr, options.json, err)
	}
	return 0
}

func parseGlobalArguments(arguments []string) (runOptions, []string, error) {
	options := runOptions{}
	command := make([]string, 0, len(arguments))
	for _, argument := range arguments {
		switch argument {
		case "--json":
			options.json = true
		case "--source":
			options.source = true
		case "--help", "-h":
			return options, []string{"help"}, nil
		case "--version":
			return options, []string{"version"}, nil
		default:
			command = append(command, argument)
		}
	}
	return options, command, nil
}

func execute(context Context, command []string, options runOptions) (any, error) {
	if context.Stdin == nil {
		context.Stdin = strings.NewReader("")
	}
	load := func() (*scope.Workspace, error) {
		if context.Load == nil {
			return nil, errors.New("Scope workspace loader is required")
		}
		return context.Load()
	}
	if command[0] == "diff" {
		if len(command) != 3 {
			return nil, invalid("diff requires left and right selectors")
		}
		var workspace *scope.Workspace
		var err error
		if strings.HasPrefix(command[1], "path:") {
			workspace, err = loadPath(context, strings.TrimPrefix(command[1], "path:"))
		} else {
			workspace, err = load()
		}
		if err != nil {
			return nil, operational(err)
		}
		result, err := scopeapp.New(workspace).Diff(command[1], command[2], func(path string) (*scope.Workspace, error) { return loadPath(context, path) })
		if err != nil {
			return nil, operational(err)
		}
		return result, nil
	}
	workspace, err := load()
	if err != nil {
		return nil, operational(err)
	}
	app := scopeapp.New(workspace)
	if (command[0] == "entity" || command[0] == "relation") && len(command) > 1 && isMutationAction(command[1]) {
		return executeMutation(app, context, command)
	}
	switch command[0] {
	case "validate":
		if len(command) != 1 {
			return nil, invalid("validate does not accept arguments")
		}
		return app.Validate(), nil
	case "scope", "group", "entity", "relation":
		return executeQuery(app, command, options.source)
	case "graph", "path", "impact":
		return executeGraph(app, command, context.Stdin, options.source)
	default:
		return nil, invalid("unknown command %q", strings.Join(command, " "))
	}
}

func executeQuery(app *scopeapp.Service, command []string, withSource bool) (any, error) {
	kind, arguments := command[0], command[1:]
	if (kind == "entity" || kind == "relation") && len(arguments) != 0 && isMutationAction(arguments[0]) {
		return nil, invalid("mutation command must be dispatched as a mutation")
	}
	if kind == "relation" {
		predicates, err := scope.ParseFilters(arguments)
		if err != nil {
			return nil, apperror.Invalid("scope.filter_invalid", err)
		}
		return app.QueryRelations(predicates, withSource), nil
	}
	if len(arguments) == 0 {
		switch kind {
		case "scope":
			return app.QueryScopes(nil, withSource), nil
		case "group":
			return app.QueryGroups(nil, withSource), nil
		default:
			return app.QueryEntities(nil, withSource), nil
		}
	}
	if len(arguments) == 1 && !looksLikeFilter(arguments[0]) {
		switch kind {
		case "scope":
			return app.GetScope(arguments[0], withSource)
		case "group":
			return app.GetGroup(arguments[0], withSource)
		default:
			return app.GetEntity(arguments[0], withSource)
		}
	}
	predicates, err := scope.ParseFilters(arguments)
	if err != nil {
		return nil, apperror.Invalid("scope.filter_invalid", err)
	}
	switch kind {
	case "scope":
		return app.QueryScopes(predicates, withSource), nil
	case "group":
		return app.QueryGroups(predicates, withSource), nil
	default:
		return app.QueryEntities(predicates, withSource), nil
	}
}

func executeGraph(app *scopeapp.Service, command []string, stdin io.Reader, withSource bool) (any, error) {
	kind := command[0]
	arguments := command[1:]
	depth := 1
	viaArguments := []string{}
	positional := []string{}
	inVia := false
	for index := 0; index < len(arguments); index++ {
		switch arguments[index] {
		case "--via":
			inVia = true
		case "--depth":
			if kind != "graph" || index+1 >= len(arguments) {
				return nil, invalid("--depth requires a value and is only valid for graph")
			}
			parsed, err := strconv.Atoi(arguments[index+1])
			if err != nil || parsed < 0 {
				return nil, invalid("graph depth must be a non-negative integer")
			}
			depth = parsed
			index++
		default:
			if inVia {
				viaArguments = append(viaArguments, arguments[index])
			} else {
				positional = append(positional, arguments[index])
			}
		}
	}
	via, err := scope.ParseFilters(viaArguments)
	if err != nil {
		return nil, apperror.Invalid("scope.filter_invalid", err)
	}
	switch kind {
	case "graph":
		if len(positional) > 1 {
			return nil, invalid("graph accepts at most one seed")
		}
		var seeds []scope.EntityKey
		if len(positional) == 0 {
			for _, entity := range app.QueryEntities(nil, false) {
				if entity.Key.Scope == app.Workspace().Root {
					seeds = append(seeds, entity.Key)
				}
			}
		} else {
			seeds, err = parseSeeds(app, positional[0], stdin)
		}
		if err != nil {
			return nil, apperror.Invalid("scope.graph_seed_invalid", err)
		}
		return app.Graph(seeds, depth, via, withSource)
	case "path":
		if len(positional) != 2 {
			return nil, invalid("path requires from and to")
		}
		from, err := app.GetEntity(positional[0], false)
		if err != nil {
			return nil, operational(err)
		}
		to, err := app.GetEntity(positional[1], false)
		if err != nil {
			return nil, operational(err)
		}
		result, err := app.Path(from.Key, to.Key, via, withSource)
		if err != nil {
			return nil, apperror.Wrap(apperror.NotFound, "scope.path_unreachable", err.Error(), err, nil)
		}
		return result, nil
	case "impact":
		if len(positional) != 1 {
			return nil, invalid("impact requires one seed or -")
		}
		seeds, err := parseSeeds(app, positional[0], stdin)
		if err != nil {
			return nil, apperror.Invalid("scope.graph_seed_invalid", err)
		}
		return app.Impact(seeds, via, withSource)
	default:
		return nil, invalid("unknown graph command %q", kind)
	}
}

func parseSeeds(app *scopeapp.Service, argument string, stdin io.Reader) ([]scope.EntityKey, error) {
	if argument != "-" {
		item, err := app.GetEntity(argument, false)
		if err != nil {
			return nil, err
		}
		return []scope.EntityKey{item.Key}, nil
	}
	data, err := io.ReadAll(stdin)
	if err != nil {
		return nil, fmt.Errorf("read stdin: %w", err)
	}
	var many []struct {
		Key scope.EntityKey `json:"key"`
	}
	if err := json.Unmarshal(data, &many); err == nil {
		result := make([]scope.EntityKey, 0, len(many))
		for _, item := range many {
			if item.Key.Scope == "" || item.Key.ID == "" {
				return nil, errors.New("stdin Entity envelope requires key.scope and key.id")
			}
			result = append(result, item.Key)
		}
		return result, nil
	}
	var one struct {
		Key scope.EntityKey `json:"key"`
	}
	if err := json.Unmarshal(data, &one); err != nil {
		return nil, fmt.Errorf("stdin must contain an Entity envelope or array: %w", err)
	}
	if one.Key.Scope == "" || one.Key.ID == "" {
		return nil, errors.New("stdin Entity envelope requires key.scope and key.id")
	}
	return []scope.EntityKey{one.Key}, nil
}

func executeMutation(app *scopeapp.Service, context Context, command []string) (any, error) {
	if len(command) < 2 || !isMutationAction(command[1]) {
		return nil, invalid("unknown command %q", strings.Join(command, " "))
	}
	kind, action := command[0], command[1]
	request, err := parseMutation(kind, action, command[2:], context.Stdin)
	if err != nil {
		return nil, apperror.Invalid("scope.mutation_invalid", err)
	}
	reload := context.Reload
	if reload == nil {
		reload = context.Load
	}
	result, _, err := app.Mutate(request, reload)
	if err != nil {
		return nil, operational(err)
	}
	return result, nil
}

func parseMutation(kind, action string, arguments []string, stdin io.Reader) (scopeapp.MutationRequest, error) {
	request := scopeapp.MutationRequest{Kind: kind, Action: action, Object: map[string]any{}, Fields: map[string]any{}}
	clean := make([]string, 0, len(arguments))
	for index := 0; index < len(arguments); index++ {
		if arguments[index] == "--file" {
			if action != "add" || index+1 >= len(arguments) {
				return request, errors.New("--file requires a path and is only valid for add")
			}
			request.File = arguments[index+1]
			index++
			continue
		}
		clean = append(clean, arguments[index])
	}
	if action == "unset" {
		return parseUnset(request, clean)
	}
	if action == "remove" {
		return parseRemove(request, clean)
	}
	positionals := []string{}
	payloads := []string{}
	for _, argument := range clean {
		if argument == "-" || strings.HasPrefix(strings.TrimSpace(argument), "{") || looksLikeFilter(argument) {
			payloads = append(payloads, argument)
		} else {
			positionals = append(positionals, argument)
		}
	}
	if kind == "entity" {
		if len(positionals) > 1 {
			return request, errors.New("entity mutation accepts at most one positional id/ref")
		}
		if len(positionals) == 1 {
			if action == "add" {
				request.Object["id"] = positionals[0]
			} else {
				request.Ref = positionals[0]
			}
		}
	} else {
		if len(positionals) != 0 && len(positionals) != 3 {
			return request, errors.New("relation mutation requires from type to or a complete JSON object")
		}
		if len(positionals) == 3 {
			request.From, request.Type, request.To = positionals[0], positionals[1], positionals[2]
			request.Object["from"], request.Object["type"], request.Object["to"] = request.From, request.Type, request.To
		}
	}
	for _, payload := range payloads {
		if payload == "-" {
			data, err := io.ReadAll(stdin)
			if err != nil {
				return request, err
			}
			var object map[string]any
			if err := json.Unmarshal(data, &object); err != nil {
				return request, fmt.Errorf("decode stdin JSON: %w", err)
			}
			if err := mergePayloadObject(request.Object, object); err != nil {
				return request, err
			}
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(payload), "{") {
			var object map[string]any
			if err := json.Unmarshal([]byte(payload), &object); err != nil {
				return request, fmt.Errorf("decode JSON object: %w", err)
			}
			if err := mergePayloadObject(request.Object, object); err != nil {
				return request, err
			}
			continue
		}
		field, value, found := strings.Cut(payload, "=")
		if !found || field == "" {
			return request, fmt.Errorf("invalid field assignment %q", payload)
		}
		var decoded any
		if err := json.Unmarshal([]byte(value), &decoded); err != nil {
			decoded = value
		}
		request.Fields[field] = decoded
	}
	if kind == "entity" {
		id, _ := request.Object["id"].(string)
		if action == "add" && id == "" {
			return request, errors.New("entity add requires id")
		}
		if action == "set" {
			if request.Ref == "" {
				request.Ref = id
			}
			if request.Ref == "" {
				return request, errors.New("entity set requires ref or JSON id")
			}
			if id != "" && id != request.Ref {
				return request, errors.New("positional ref and JSON id must match")
			}
		}
	} else {
		for _, field := range []string{"from", "type", "to"} {
			if request.Object[field] == nil {
				return request, fmt.Errorf("relation %s is required", field)
			}
		}
		if request.From == "" {
			request.From, _ = request.Object["from"].(string)
			request.Type, _ = request.Object["type"].(string)
			request.To, _ = request.Object["to"].(string)
		}
	}
	return request, nil
}

func mergePayloadObject(target, input map[string]any) error {
	for key, existing := range target {
		if value, exists := input[key]; exists && !reflect.DeepEqual(existing, value) {
			return fmt.Errorf("positional %s does not match JSON value", key)
		}
	}
	for key, value := range input {
		target[key] = value
	}
	return nil
}

func parseUnset(request scopeapp.MutationRequest, arguments []string) (scopeapp.MutationRequest, error) {
	if request.Kind == "entity" {
		if len(arguments) < 2 {
			return request, errors.New("entity unset requires ref and fields")
		}
		request.Ref, request.Unset = arguments[0], arguments[1:]
		return request, nil
	}
	if len(arguments) < 4 {
		return request, errors.New("relation unset requires from type to and fields")
	}
	request.From, request.Type, request.To, request.Unset = arguments[0], arguments[1], arguments[2], arguments[3:]
	return request, nil
}
func parseRemove(request scopeapp.MutationRequest, arguments []string) (scopeapp.MutationRequest, error) {
	if request.Kind == "entity" {
		if len(arguments) != 1 {
			return request, errors.New("entity remove requires ref")
		}
		request.Ref = arguments[0]
		return request, nil
	}
	if len(arguments) != 3 {
		return request, errors.New("relation remove requires from type to")
	}
	request.From, request.Type, request.To = arguments[0], arguments[1], arguments[2]
	return request, nil
}
func isMutationAction(value string) bool {
	return value == "add" || value == "set" || value == "unset" || value == "remove"
}
func looksLikeFilter(value string) bool { return strings.ContainsAny(value, "=!") }
func loadPath(context Context, path string) (*scope.Workspace, error) {
	if context.LoadPath != nil {
		return context.LoadPath(path)
	}
	absolute := path
	if !filepath.IsAbs(absolute) {
		absolute = filepath.Join(context.WorkingDirectory, path)
	}
	if strings.HasSuffix(absolute, ".locus.yaml") || strings.HasSuffix(absolute, ".locus.yml") || strings.HasSuffix(absolute, ".locus.json") {
		return scope.LoadDefinitionFile(absolute)
	}
	source, err := scope.NewLocalSource(absolute)
	if err != nil {
		return nil, err
	}
	return scope.Load(source, scope.LocalResolver{})
}
func invalid(format string, values ...any) error {
	err := fmt.Errorf(format, values...)
	return apperror.Invalid("scope.selector_invalid", err)
}
func operational(err error) error {
	return apperror.Normalize(err, "scope.execute")
}
func fail(output io.Writer, jsonOutput bool, err error) int {
	application := apperror.Normalize(err, "scope.cli")
	if jsonOutput {
		_ = json.NewEncoder(output).Encode(map[string]string{"error": application.Message})
	} else {
		fmt.Fprintf(output, "locus-scope: %s\n", application.Message)
	}
	return apperror.ExitCode(application)
}
func writeResult(output io.Writer, value any, jsonOutput bool) error {
	encoder := json.NewEncoder(output)
	if !jsonOutput {
		encoder.SetIndent("", "  ")
	}
	return encoder.Encode(value)
}

const Usage = `locus-scope manages Scope workspaces.

Usage:
  locus-scope [--scope <dir>] [--json] scope [<scope-ref>|filters...] [--source]
  locus-scope [--scope <dir>] [--json] group [<group-ref>|filters...] [--source]
  locus-scope [--scope <dir>] [--json] entity [<entity-ref>|filters...] [--source]
  locus-scope [--scope <dir>] [--json] relation [filters...] [--source]
  locus-scope [--scope <dir>] [--json] graph [<entity-ref>|-] [--depth N] [--via filters...]
  locus-scope [--scope <dir>] [--json] path <from> <to> [--via filters...]
  locus-scope [--scope <dir>] [--json] impact <entity-ref|-> [--via filters...]
  locus-scope [--scope <dir>] [--json] entity <add|set|unset|remove> ...
  locus-scope [--scope <dir>] [--json] relation <add|set|unset|remove> ...
  locus-scope [--scope <dir>] [--json] diff <left> <right>
  locus-scope [--scope <dir>] [--json] validate
  locus-scope [--json] version
  locus-scope help
`
