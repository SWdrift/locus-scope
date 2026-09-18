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

const protocolVersion = 3

type request struct {
	Version          int                    `json:"version"`
	WorkingDirectory string                 `json:"workingDirectory"`
	Arguments        []string               `json:"arguments"`
	Environment      packageenv.Environment `json:"environment"`
	Stdin            string                 `json:"stdin,omitempty"`
}

type response struct {
	Version  int    `json:"version"`
	ExitCode int    `json:"exitCode"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
}

func main() {
	if len(os.Args) != 1 {
		os.Exit(writeResponse(os.Stdout, response{Version: protocolVersion, ExitCode: 1, Stderr: "locus-scope-node-host: command-line arguments are not supported\n"}))
	}
	os.Exit(run(os.Stdin, os.Stdout))
}

func run(input io.Reader, output io.Writer) int {
	var value request
	decoder := json.NewDecoder(input)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return writeResponse(output, response{Version: protocolVersion, ExitCode: 1, Stderr: fmt.Sprintf("locus-scope-node-host: decode request: %v\n", err)})
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return writeResponse(output, response{Version: protocolVersion, ExitCode: 1, Stderr: "locus-scope-node-host: request contains trailing JSON\n"})
	}
	if value.Version != protocolVersion {
		return writeRequestFailure(output, value.Arguments, fmt.Errorf("unsupported protocol version %d", value.Version))
	}
	if value.WorkingDirectory == "" || !filepath.IsAbs(value.WorkingDirectory) {
		return writeRequestFailure(output, value.Arguments, errors.New("workingDirectory must be an absolute path"))
	}

	load := func() (*scope.Workspace, error) {
		if value.Environment.RootScope == "" {
			return nil, errors.New("Scope root is required")
		}
		return packageenv.Load(value.Environment)
	}
	var stdout, stderr bytes.Buffer
	exitCode := scopecli.Run(scopecli.Context{
		Stdin: strings.NewReader(value.Stdin), WorkingDirectory: value.WorkingDirectory,
		Load: load, Reload: load,
		LoadPath: func(path string) (*scope.Workspace, error) {
			if !filepath.IsAbs(path) {
				path = filepath.Join(value.WorkingDirectory, path)
			}
			if strings.HasSuffix(path, ".locus.yaml") || strings.HasSuffix(path, ".locus.yml") || strings.HasSuffix(path, ".locus.json") {
				return scope.LoadDefinitionFile(path)
			}
			if absolute, err := filepath.Abs(path); err == nil && value.Environment.RootScope != "" {
				root, rootErr := filepath.Abs(value.Environment.RootScope)
				if rootErr == nil && filepath.Clean(absolute) == filepath.Clean(root) {
					return load()
				}
			}
			source, err := scope.NewLocalSource(path)
			if err != nil {
				return nil, err
			}
			return scope.Load(source, scope.LocalResolver{})
		},
	}, value.Arguments, &stdout, &stderr)
	return writeResponse(output, response{Version: protocolVersion, ExitCode: exitCode, Stdout: stdout.String(), Stderr: stderr.String()})
}

func writeRequestFailure(output io.Writer, arguments []string, err error) int {
	message := fmt.Sprintf("locus-scope: %v\n", err)
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
