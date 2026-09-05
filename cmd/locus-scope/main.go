package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"locus-scope/internal/purepkg"
	"locus-scope/internal/scope"
	"locus-scope/internal/scopecli"
)

type options struct {
	scopeDirectory string
	jsonOutput     bool
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(arguments []string, stdout, stderr io.Writer) int {
	opts, queryArguments, err := parseArguments(arguments)
	if err != nil {
		writeFailure(stderr, opts.jsonOutput, err)
		return 2
	}
	if isRootlessCommand(queryArguments) {
		return scopecli.Run(nil, queryArguments, stdout, stderr)
	}

	root, err := rootDirectory(opts.scopeDirectory)
	if err != nil {
		writeFailure(stderr, opts.jsonOutput, err)
		return 1
	}
	workspace, err := purepkg.LoadWorkspace(root)
	if err != nil {
		writeFailure(stderr, opts.jsonOutput, err)
		return 1
	}
	return scopecli.Run(workspace, queryArguments, stdout, stderr)
}

func parseArguments(arguments []string) (options, []string, error) {
	var opts options
	queryArguments := make([]string, 0, len(arguments))
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		switch {
		case argument == "--scope":
			if index+1 == len(arguments) {
				return opts, nil, errors.New("--scope requires a directory")
			}
			index++
			opts.scopeDirectory = arguments[index]
		case strings.HasPrefix(argument, "--scope="):
			opts.scopeDirectory = strings.TrimPrefix(argument, "--scope=")
			if opts.scopeDirectory == "" {
				return opts, nil, errors.New("--scope requires a directory")
			}
		default:
			if argument == "--json" {
				opts.jsonOutput = true
			}
			queryArguments = append(queryArguments, argument)
		}
	}
	return opts, queryArguments, nil
}

func isRootlessCommand(arguments []string) bool {
	if len(arguments) == 0 {
		return true
	}
	command := make([]string, 0, len(arguments))
	for _, argument := range arguments {
		if argument != "--json" {
			command = append(command, argument)
		}
	}
	return len(command) == 0 || len(command) == 1 && (command[0] == "help" || command[0] == "--help" || command[0] == "-h" || command[0] == "version" || command[0] == "--version")
}

func rootDirectory(explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	workingDirectory, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get current directory: %w", err)
	}
	return scope.FindScope(workingDirectory)
}

func writeFailure(output io.Writer, jsonOutput bool, err error) {
	if jsonOutput {
		_ = json.NewEncoder(output).Encode(struct {
			Error string `json:"error"`
		}{err.Error()})
		return
	}
	fmt.Fprintf(output, "locus-scope: %v\n", err)
}
