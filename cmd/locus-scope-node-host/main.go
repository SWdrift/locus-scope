package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"locus-scope/internal/packageenv"
	"locus-scope/internal/scope"
	"locus-scope/internal/scopecli"
)

const protocolVersion = 2

type request struct {
	Version          int                          `json:"version"`
	WorkingDirectory string                       `json:"workingDirectory"`
	Arguments        []string                     `json:"arguments"`
	Root             rootDescriptor               `json:"root"`
	Packages         map[string]packageDescriptor `json:"packages"`
	Stdin            string                       `json:"stdin,omitempty"`
}

type rootDescriptor struct {
	ScopeRoot    string            `json:"scopeRoot"`
	PackageRoot  string            `json:"packageRoot"`
	Dependencies map[string]string `json:"dependencies"`
}

type packageDescriptor struct {
	Root         string            `json:"root"`
	Entry        string            `json:"entry"`
	Dependencies map[string]string `json:"dependencies"`
}

type response struct {
	Version  int    `json:"version"`
	ExitCode int    `json:"exitCode"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
}

func main() {
	if len(os.Args) != 1 {
		os.Exit(writeResponse(os.Stdout, response{
			Version: protocolVersion, ExitCode: 1, Stderr: "locus-scope-node-host: command-line arguments are not supported\n",
		}))
	}
	os.Exit(run(os.Stdin, os.Stdout))
}

func run(input io.Reader, output io.Writer) int {
	body, err := io.ReadAll(input)
	if err != nil {
		return writeResponse(output, response{Version: protocolVersion, ExitCode: 1, Stderr: fmt.Sprintf("read request: %v\n", err)})
	}
	var request request
	if err := decodeRequest(body, &request); err != nil {
		return writeResponse(output, response{Version: protocolVersion, ExitCode: 1, Stderr: fmt.Sprintf("locus-scope-node-host: %v\n", err)})
	}
	if err := validateRequest(request); err != nil {
		return writeRequestFailure(output, request.Arguments, err)
	}
	if isRootlessRequest(request.Arguments) {
		var stdout, stderr bytes.Buffer
		exitCode := scopecli.Run(scopecli.Context{Stdin: strings.NewReader(request.Stdin), WorkingDirectory: request.WorkingDirectory}, request.Arguments, &stdout, &stderr)
		return writeResponse(output, response{Version: protocolVersion, ExitCode: exitCode, Stdout: stdout.String(), Stderr: stderr.String()})
	}

	environment := packageenv.Environment{
		Mode:             packageenv.NPMEnvironment,
		RootDependencies: make(map[string]packageenv.Identity, len(request.Root.Dependencies)),
		Packages:         make(map[packageenv.Identity]packageenv.Package, len(request.Packages)),
	}
	for name, identity := range request.Root.Dependencies {
		environment.RootDependencies[name] = packageenv.Identity(identity)
	}
	for rawIdentity, descriptor := range request.Packages {
		identity := packageenv.Identity(rawIdentity)
		metadata, isLocus, validationErr := packageenv.ValidatePackage(descriptor.Root)
		if validationErr != nil {
			return writeRequestFailure(output, request.Arguments, fmt.Errorf("validate package %q: %w", rawIdentity, validationErr))
		}
		if !isLocus {
			return writeRequestFailure(output, request.Arguments, fmt.Errorf("descriptor package %q has no locus.entry", rawIdentity))
		}
		expectedIdentity := packageenv.Identity("npm:" + metadata.Name + "@" + metadata.Version)
		if identity != expectedIdentity {
			return writeRequestFailure(output, request.Arguments, fmt.Errorf("package identity %q does not match metadata identity %q", identity, expectedIdentity))
		}
		if filepath.Clean(filepath.FromSlash(metadata.Entry)) != filepath.Clean(filepath.FromSlash(descriptor.Entry)) {
			return writeRequestFailure(output, request.Arguments, fmt.Errorf("package %q entry does not match package.json", identity))
		}
		dependencies := make(map[string]packageenv.Identity, len(descriptor.Dependencies))
		for name, dependency := range descriptor.Dependencies {
			dependencies[name] = packageenv.Identity(dependency)
		}
		environment.Packages[identity] = packageenv.Package{
			Identity:     identity,
			Root:         descriptor.Root,
			Entry:        descriptor.Entry,
			Dependencies: dependencies,
		}
	}
	load := func() (*scope.Workspace, error) { return packageenv.Load(request.Root.ScopeRoot, environment) }
	workspace, err := load()
	if err != nil {
		return writeRequestFailure(output, request.Arguments, err)
	}

	var stdout, stderr bytes.Buffer
	exitCode := scopecli.Run(scopecli.Context{
		Stdin: strings.NewReader(request.Stdin), WorkingDirectory: request.WorkingDirectory,
		Load: func() (*scope.Workspace, error) { return workspace, nil }, Reload: load,
		LoadPath: func(path string) (*scope.Workspace, error) {
			if !filepath.IsAbs(path) {
				path = filepath.Join(request.WorkingDirectory, path)
			}
			if strings.HasSuffix(path, ".locus.yaml") || strings.HasSuffix(path, ".locus.yml") || strings.HasSuffix(path, ".locus.json") {
				return scope.LoadDefinitionFile(path)
			}
			if absolute, absoluteErr := filepath.Abs(path); absoluteErr == nil {
				if root, rootErr := filepath.Abs(request.Root.ScopeRoot); rootErr == nil && filepath.Clean(absolute) == filepath.Clean(root) {
					return load()
				}
			}
			source, sourceErr := scope.NewLocalSource(path)
			if sourceErr != nil {
				return nil, sourceErr
			}
			return scope.Load(source, scope.LocalResolver{})
		},
	}, request.Arguments, &stdout, &stderr)
	return writeResponse(output, response{Version: protocolVersion, ExitCode: exitCode, Stdout: stdout.String(), Stderr: stderr.String()})
}

func decodeRequest(body []byte, destination *request) error {
	if len(bytes.TrimSpace(body)) == 0 {
		return errors.New("request is empty")
	}
	if err := rejectDuplicateJSONFields(body); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("decode request: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("request contains trailing JSON")
		}
		return fmt.Errorf("decode trailing request data: %w", err)
	}
	return nil
}

func rejectDuplicateJSONFields(body []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	var visit func() error
	visit = func() error {
		token, err := decoder.Token()
		if err != nil {
			return fmt.Errorf("decode request: %w", err)
		}
		delimiter, isDelimiter := token.(json.Delim)
		if !isDelimiter {
			return nil
		}
		switch delimiter {
		case '{':
			seen := make(map[string]struct{})
			for decoder.More() {
				keyToken, err := decoder.Token()
				if err != nil {
					return fmt.Errorf("decode request: %w", err)
				}
				key, ok := keyToken.(string)
				if !ok {
					return errors.New("decode request: object key is not a string")
				}
				if _, exists := seen[key]; exists {
					return fmt.Errorf("decode request: duplicate field %q", key)
				}
				seen[key] = struct{}{}
				if err := visit(); err != nil {
					return err
				}
			}
		case '[':
			for decoder.More() {
				if err := visit(); err != nil {
					return err
				}
			}
		default:
			return fmt.Errorf("decode request: unexpected delimiter %q", delimiter)
		}
		if _, err := decoder.Token(); err != nil {
			return fmt.Errorf("decode request: %w", err)
		}
		return nil
	}
	if err := visit(); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("request contains trailing JSON")
		}
		return fmt.Errorf("decode trailing request data: %w", err)
	}
	return nil
}

func validateRequest(request request) error {
	if request.Version != protocolVersion {
		return fmt.Errorf("unsupported protocol version %d", request.Version)
	}
	if err := requireAbsoluteDirectory("workingDirectory", request.WorkingDirectory); err != nil {
		return err
	}
	if isRootlessRequest(request.Arguments) {
		return nil
	}
	if err := requireAbsoluteDirectory("root.scopeRoot", request.Root.ScopeRoot); err != nil {
		return err
	}
	if request.Root.PackageRoot != "" {
		if err := requireAbsoluteDirectory("root.packageRoot", request.Root.PackageRoot); err != nil {
			return err
		}
	}
	if request.Root.Dependencies == nil {
		request.Root.Dependencies = map[string]string{}
	}
	if request.Packages == nil {
		return errors.New("packages must be an object")
	}
	for identity, descriptor := range request.Packages {
		if identity == "" {
			return errors.New("package identity must not be empty")
		}
		if err := requireAbsoluteDirectory(fmt.Sprintf("packages[%q].root", identity), descriptor.Root); err != nil {
			return err
		}
		if descriptor.Entry == "" || filepath.IsAbs(descriptor.Entry) {
			return fmt.Errorf("package %q entry must be a relative path", identity)
		}
		entry := filepath.Clean(filepath.Join(descriptor.Root, filepath.FromSlash(descriptor.Entry)))
		relative, err := filepath.Rel(descriptor.Root, entry)
		if err != nil || relative == ".." || filepath.IsAbs(relative) || (len(relative) > 3 && relative[:3] == ".."+string(filepath.Separator)) {
			return fmt.Errorf("package %q entry escapes its root", identity)
		}
	}
	return nil
}

func isRootlessRequest(arguments []string) bool {
	command := make([]string, 0, len(arguments))
	for _, argument := range arguments {
		if argument != "--json" {
			command = append(command, argument)
		}
	}
	return len(command) == 0 || len(command) == 1 && (command[0] == "help" || command[0] == "--help" || command[0] == "-h" || command[0] == "version" || command[0] == "--version")
}

func requireAbsoluteDirectory(field, path string) error {
	if path == "" || !filepath.IsAbs(path) {
		return fmt.Errorf("%s must be an absolute path", field)
	}
	information, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("%s: %w", field, err)
	}
	if !information.IsDir() {
		return fmt.Errorf("%s must name a directory", field)
	}
	return nil
}

func writeRequestFailure(output io.Writer, arguments []string, err error) int {
	message := fmt.Sprintf("locus-scope-node: %v\n", err)
	for _, argument := range arguments {
		if argument == "--json" {
			body, _ := json.Marshal(struct {
				Error string `json:"error"`
			}{err.Error()})
			message = string(body) + "\n"
			break
		}
	}
	return writeResponse(output, response{Version: protocolVersion, ExitCode: 1, Stderr: message})
}

func writeResponse(output io.Writer, value response) int {
	if err := json.NewEncoder(output).Encode(value); err != nil {
		return 1
	}
	return value.ExitCode
}
