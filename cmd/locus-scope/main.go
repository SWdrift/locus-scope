package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"locus-scope/internal/pkgapp"
	"locus-scope/internal/scope"
	"locus-scope/internal/scopecli"
)

type options struct {
	scopeDirectory string
	jsonOutput     bool
}

func main() { os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr)) }

func run(arguments []string, streams ...io.Reader) int {
	var stdin io.Reader = strings.NewReader("")
	var stdout, stderr io.Writer
	if len(streams) == 3 {
		stdin, stdout, stderr = streams[0], streams[1].(io.Writer), streams[2].(io.Writer)
	} else {
		stdout, stderr = streams[0].(io.Writer), streams[1].(io.Writer)
	}
	opts, queryArguments, err := parseArguments(arguments)
	if err != nil {
		writeFailure(stderr, opts.jsonOutput, err)
		return 2
	}
	workingDirectory, err := os.Getwd()
	if err != nil {
		writeFailure(stderr, opts.jsonOutput, err)
		return 1
	}
	load := func() (*scope.Workspace, error) {
		root, err := rootDirectory(opts.scopeDirectory)
		if err != nil {
			return nil, err
		}
		return pkgapp.New(root, pkgapp.Options{}).LoadWorkspace()
	}
	context := scopecli.Context{Stdin: stdin, WorkingDirectory: workingDirectory, Load: load, Reload: load, LoadPath: func(path string) (*scope.Workspace, error) {
		if !filepath.IsAbs(path) {
			path = filepath.Join(workingDirectory, path)
		}
		if strings.HasSuffix(path, ".locus.yaml") || strings.HasSuffix(path, ".locus.yml") || strings.HasSuffix(path, ".locus.json") {
			return scope.LoadDefinitionFile(path)
		}
		return pkgapp.New(path, pkgapp.Options{}).LoadWorkspace()
	}}
	return scopecli.Run(context, queryArguments, stdout, stderr)
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
		_ = json.NewEncoder(output).Encode(map[string]string{"error": err.Error()})
		return
	}
	fmt.Fprintf(output, "locus-scope: %v\n", err)
}
