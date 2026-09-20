// Package scopecli 实现共享的 locus-scope 命令面。
package scopecli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/spf13/cobra"
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

type options struct {
	json    bool
	source  bool
	version bool
}

var errVersionPrinted = errors.New("version printed")

func Run(context Context, arguments []string, stdout, stderr io.Writer) int {
	if context.Stdin == nil {
		context.Stdin = strings.NewReader("")
	}
	state := &options{}
	root := newRootCommand(context, state, stdout)
	root.SetArgs(arguments)
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SilenceErrors = true
	root.SilenceUsage = true
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return apperror.Invalid("scope.selector_invalid", err) })
	if err := root.Execute(); err != nil {
		if errors.Is(err, errVersionPrinted) {
			return 0
		}
		if apperror.CodeOf(err) == apperror.Internal {
			err = apperror.Invalid("scope.selector_invalid", err)
		}
		return fail(stderr, state.json, err)
	}
	return 0
}

func newRootCommand(context Context, state *options, output io.Writer) *cobra.Command {
	root := &cobra.Command{
		Use:           "locus-scope",
		Short:         "Query and mutate Locus Scope workspaces",
		SilenceErrors: true,
		PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
			if !state.version {
				return nil
			}
			if err := buildinfo.WriteVersion(output, "locus-scope", state.json); err != nil {
				return err
			}
			return errVersionPrinted
		},
		RunE: func(command *cobra.Command, _ []string) error {
			return command.Help()
		},
	}
	root.PersistentFlags().BoolVar(&state.json, "json", false, "write JSON output")
	root.PersistentFlags().BoolVar(&state.source, "source", false, "include declaration provenance")
	root.PersistentFlags().BoolVar(&state.version, "version", false, "print version")

	for _, kind := range []string{"scope", "group", "entity", "relation"} {
		root.AddCommand(newQueryCommand(context, state, output, kind))
	}
	root.AddCommand(newGraphCommand(context, state, output))
	root.AddCommand(newPathCommand(context, state, output))
	root.AddCommand(newImpactCommand(context, state, output))
	root.AddCommand(newDiffCommand(context, state, output))
	root.AddCommand(&cobra.Command{
		Use: "validate", Args: cobra.NoArgs, Short: "Validate the composed Workspace",
		RunE: func(_ *cobra.Command, _ []string) error {
			return withWorkspace(context, func(app *scopeapp.Service) error {
				return writeResult(output, app.Validate(), state.json)
			})
		},
	})
	root.AddCommand(&cobra.Command{
		Use: "version", Args: cobra.NoArgs, Short: "Print version",
		RunE: func(_ *cobra.Command, _ []string) error {
			return buildinfo.WriteVersion(output, "locus-scope", state.json)
		},
	})
	return root
}

func newQueryCommand(context Context, state *options, output io.Writer, kind string) *cobra.Command {
	command := &cobra.Command{
		Use: kind + " [reference|filter...]", Args: cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			return withWorkspace(context, func(app *scopeapp.Service) error {
				result, err := query(app, kind, args, state.source)
				if err != nil {
					return err
				}
				return writeResult(output, result, state.json)
			})
		},
	}
	if kind == "entity" || kind == "relation" {
		for _, action := range []string{"add", "set", "unset", "remove"} {
			command.AddCommand(newMutationCommand(context, state, output, kind, action))
		}
	}
	return command
}

func query(app *scopeapp.Service, kind string, args []string, withSource bool) (any, error) {
	if kind == "relation" {
		predicates, err := scope.ParseFilters(args)
		if err != nil {
			return nil, apperror.Invalid("scope.filter_invalid", err)
		}
		return app.QueryRelations(predicates, withSource), nil
	}
	if len(args) == 0 {
		switch kind {
		case "scope":
			return app.QueryScopes(nil, withSource), nil
		case "group":
			return app.QueryGroups(nil, withSource), nil
		default:
			return app.QueryEntities(nil, withSource), nil
		}
	}
	if len(args) == 1 && !looksLikeFilter(args[0]) {
		switch kind {
		case "scope":
			return app.GetScope(args[0], withSource)
		case "group":
			return app.GetGroup(args[0], withSource)
		default:
			return app.GetEntity(args[0], withSource)
		}
	}
	predicates, err := scope.ParseFilters(args)
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

func newGraphCommand(context Context, state *options, output io.Writer) *cobra.Command {
	var depth int
	var via []string
	command := &cobra.Command{
		Use: "graph [entity|-]", Args: cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if depth < 0 {
				return invalid("graph depth must be a non-negative integer")
			}
			return withWorkspace(context, func(app *scopeapp.Service) error {
				predicates, err := parseFilters(via)
				if err != nil {
					return err
				}
				var seeds []scope.EntityKey
				if len(args) == 0 {
					for _, entity := range app.QueryEntities(nil, false) {
						if entity.Key.Scope == app.Workspace().Root {
							seeds = append(seeds, entity.Key)
						}
					}
				} else if seeds, err = parseSeeds(app, args[0], context.Stdin); err != nil {
					return apperror.Invalid("scope.graph_seed_invalid", err)
				}
				result, err := app.Graph(seeds, depth, predicates, state.source)
				if err != nil {
					return operational(err)
				}
				return writeResult(output, result, state.json)
			})
		},
	}
	command.Flags().IntVar(&depth, "depth", 1, "maximum outgoing hops")
	command.Flags().StringArrayVar(&via, "via", nil, "relation filter; repeat for AND")
	return command
}

func newPathCommand(context Context, state *options, output io.Writer) *cobra.Command {
	var via []string
	command := &cobra.Command{
		Use: "path <from> <to>", Args: cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			return withWorkspace(context, func(app *scopeapp.Service) error {
				predicates, err := parseFilters(via)
				if err != nil {
					return err
				}
				from, err := app.GetEntity(args[0], false)
				if err != nil {
					return operational(err)
				}
				to, err := app.GetEntity(args[1], false)
				if err != nil {
					return operational(err)
				}
				result, err := app.Path(from.Key, to.Key, predicates, state.source)
				if err != nil {
					return apperror.Wrap(apperror.NotFound, "scope.path_unreachable", err.Error(), err, nil)
				}
				return writeResult(output, result, state.json)
			})
		},
	}
	command.Flags().StringArrayVar(&via, "via", nil, "relation filter; repeat for AND")
	return command
}

func newImpactCommand(context Context, state *options, output io.Writer) *cobra.Command {
	var via []string
	command := &cobra.Command{
		Use: "impact <entity|->", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withWorkspace(context, func(app *scopeapp.Service) error {
				predicates, err := parseFilters(via)
				if err != nil {
					return err
				}
				seeds, err := parseSeeds(app, args[0], context.Stdin)
				if err != nil {
					return apperror.Invalid("scope.graph_seed_invalid", err)
				}
				result, err := app.Impact(seeds, predicates, state.source)
				if err != nil {
					return operational(err)
				}
				return writeResult(output, result, state.json)
			})
		},
	}
	command.Flags().StringArrayVar(&via, "via", nil, "relation filter; repeat for AND")
	return command
}

func newDiffCommand(context Context, state *options, output io.Writer) *cobra.Command {
	return &cobra.Command{
		Use: "diff <left> <right>", Args: cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			var workspace *scope.Workspace
			var err error
			if strings.HasPrefix(args[0], "path:") {
				workspace, err = loadPath(context, strings.TrimPrefix(args[0], "path:"))
			} else {
				workspace, err = load(context)
			}
			if err != nil {
				return operational(err)
			}
			result, err := scopeapp.New(workspace).Diff(args[0], args[1], func(path string) (*scope.Workspace, error) { return loadPath(context, path) })
			if err != nil {
				return operational(err)
			}
			return writeResult(output, result, state.json)
		},
	}
}

func newMutationCommand(context Context, state *options, output io.Writer, kind, action string) *cobra.Command {
	var file string
	command := &cobra.Command{
		Use: action + " [arguments...]", Args: cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			request, err := parseMutation(kind, action, args, file, context.Stdin)
			if err != nil {
				return apperror.Invalid("scope.mutation_invalid", err)
			}
			return withWorkspace(context, func(app *scopeapp.Service) error {
				reload := context.Reload
				if reload == nil {
					reload = context.Load
				}
				result, _, err := app.Mutate(request, reload)
				if err != nil {
					return operational(err)
				}
				return writeResult(output, result, state.json)
			})
		},
	}
	if action == "add" {
		command.Flags().StringVar(&file, "file", "", "target *.locus.yaml definition")
	}
	return command
}

func parseMutation(kind, action string, args []string, file string, stdin io.Reader) (scopeapp.MutationRequest, error) {
	request := scopeapp.MutationRequest{Kind: kind, Action: action, Object: map[string]any{}, Fields: map[string]any{}, File: file}
	if action == "unset" {
		if kind == "entity" && len(args) >= 2 {
			request.Ref, request.Unset = args[0], args[1:]
			return request, nil
		}
		if kind == "relation" && len(args) >= 4 {
			request.From, request.Type, request.To, request.Unset = args[0], args[1], args[2], args[3:]
			return request, nil
		}
		return request, fmt.Errorf("%s unset requires object identity and fields", kind)
	}
	if action == "remove" {
		if kind == "entity" && len(args) == 1 {
			request.Ref = args[0]
			return request, nil
		}
		if kind == "relation" && len(args) == 3 {
			request.From, request.Type, request.To = args[0], args[1], args[2]
			return request, nil
		}
		return request, fmt.Errorf("%s remove requires object identity", kind)
	}
	var positionals, payloads []string
	for _, argument := range args {
		if argument == "-" || strings.HasPrefix(strings.TrimSpace(argument), "{") || looksLikeFilter(argument) {
			payloads = append(payloads, argument)
		} else {
			positionals = append(positionals, argument)
		}
	}
	if kind == "entity" {
		if len(positionals) > 1 {
			return request, errors.New("entity mutation accepts at most one id/ref")
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
		var object map[string]any
		if payload == "-" {
			data, err := io.ReadAll(stdin)
			if err != nil {
				return request, err
			}
			if err := json.Unmarshal(data, &object); err != nil {
				return request, fmt.Errorf("decode stdin JSON: %w", err)
			}
		} else if strings.HasPrefix(strings.TrimSpace(payload), "{") {
			if err := json.Unmarshal([]byte(payload), &object); err != nil {
				return request, fmt.Errorf("decode JSON object: %w", err)
			}
		} else {
			field, value, found := strings.Cut(payload, "=")
			if !found || field == "" {
				return request, fmt.Errorf("invalid field assignment %q", payload)
			}
			var decoded any
			if err := json.Unmarshal([]byte(value), &decoded); err != nil {
				decoded = value
			}
			request.Fields[field] = decoded
			continue
		}
		if err := mergePayloadObject(request.Object, object); err != nil {
			return request, err
		}
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
	if json.Unmarshal(data, &many) == nil {
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

func parseFilters(values []string) ([]scope.Predicate, error) {
	predicates, err := scope.ParseFilters(values)
	if err != nil {
		return nil, apperror.Invalid("scope.filter_invalid", err)
	}
	return predicates, nil
}
func looksLikeFilter(value string) bool { return strings.ContainsAny(value, "=!") }
func load(context Context) (*scope.Workspace, error) {
	if context.Load == nil {
		return nil, errors.New("Scope workspace loader is required")
	}
	return context.Load()
}
func withWorkspace(context Context, execute func(*scopeapp.Service) error) error {
	workspace, err := load(context)
	if err != nil {
		return operational(err)
	}
	return execute(scopeapp.New(workspace))
}
func loadPath(context Context, path string) (*scope.Workspace, error) {
	if context.LoadPath != nil {
		return context.LoadPath(path)
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(context.WorkingDirectory, path)
	}
	if strings.HasSuffix(path, ".locus.yaml") || strings.HasSuffix(path, ".locus.yml") || strings.HasSuffix(path, ".locus.json") {
		return scope.LoadDefinitionFile(path)
	}
	source, err := scope.NewLocalSource(path)
	if err != nil {
		return nil, err
	}
	return scope.Load(source, scope.LocalResolver{})
}
func invalid(format string, values ...any) error {
	return apperror.Invalid("scope.selector_invalid", fmt.Errorf(format, values...))
}
func operational(err error) error { return apperror.Normalize(err, "scope.execute") }
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
