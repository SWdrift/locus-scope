package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"locus-scope/internal/buildinfo"

	"locus-scope/internal/packages"
	"locus-scope/internal/scope"
)

type options struct {
	scopeDirectory string
	frozen         bool
	jsonOutput     bool
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(arguments []string, stdout, stderr io.Writer) int {
	opts, command, err := parseArguments(arguments)
	if err != nil {
		writeFailure(stderr, opts.jsonOutput, err)
		return 2
	}
	if len(command) == 0 || command[0] == "help" {
		fmt.Fprint(stdout, usage)
		return 0
	}
	if len(command) == 1 && command[0] == "version" {
		if err := buildinfo.WriteVersion(stdout, "locus-pkg", opts.jsonOutput); err != nil {
			writeFailure(stderr, opts.jsonOutput, err)
			return 1
		}
		return 0
	}
	switch command[0] {
	case "install":
		if len(command) != 1 {
			writeFailure(stderr, opts.jsonOutput, errors.New("install does not accept positional arguments"))
			return 2
		}
	case "publish":
		if len(command) != 2 {
			writeFailure(stderr, opts.jsonOutput, errors.New("publish requires exactly one OCI target"))
			return 2
		}
		if opts.frozen {
			writeFailure(stderr, opts.jsonOutput, errors.New("--frozen is only valid with install"))
			return 2
		}
	default:
		writeFailure(stderr, opts.jsonOutput, fmt.Errorf("unknown command %q; run locus-pkg help", command[0]))
		return 2
	}

	root, err := rootDirectory(opts.scopeDirectory)
	if err != nil {
		writeFailure(stderr, opts.jsonOutput, err)
		return 1
	}
	credential, err := packages.DockerCredential()
	if err != nil {
		writeFailure(stderr, opts.jsonOutput, err)
		return 1
	}

	if command[0] == "publish" {
		result, err := packages.Publish(context.Background(), root, command[1], packages.PublishOptions{Credential: credential})
		if err != nil {
			writeFailure(stderr, opts.jsonOutput, err)
			return 1
		}
		if opts.jsonOutput {
			if err := writeJSON(stdout, result); err != nil {
				writeFailure(stderr, true, err)
				return 1
			}
		} else {
			fmt.Fprintf(stdout, "published: %s\ndigest: %s\n", result.Target, result.Digest)
		}
		return 0
	}

	cacheRoot, err := packages.DefaultCacheRoot()
	if err != nil {
		writeFailure(stderr, opts.jsonOutput, err)
		return 1
	}
	result, err := packages.Install(context.Background(), root, packages.InstallOptions{
		Frozen: opts.frozen, CacheRoot: cacheRoot, Credential: credential,
	})
	if err != nil {
		writeFailure(stderr, opts.jsonOutput, err)
		return 1
	}
	if opts.jsonOutput {
		if err := writeJSON(stdout, result); err != nil {
			writeFailure(stderr, true, err)
			return 1
		}
		return 0
	}
	fmt.Fprintf(stdout, "installed: %s\nresolved: %d, reused: %d, fetched: %d, materialized: %d\nscopes: %d, entities: %d, relations: %d\n",
		result.Root, result.Resolved, result.Reused, result.Fetched, result.Materialized, result.Scopes, result.Entities, result.Relations)
	return 0
}

func parseArguments(arguments []string) (options, []string, error) {
	var opts options
	command := make([]string, 0, len(arguments))
	for index := 0; index < len(arguments); index++ {
		switch argument := arguments[index]; {
		case argument == "--json":
			opts.jsonOutput = true
		case argument == "--frozen":
			opts.frozen = true
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
		case argument == "--help" || argument == "-h":
			command = []string{"help"}
		case argument == "--version":
			command = []string{"version"}
		case strings.HasPrefix(argument, "-"):
			return opts, nil, fmt.Errorf("unknown option %q", argument)
		default:
			command = append(command, argument)
		}
	}
	return opts, command, nil
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
	fmt.Fprintf(output, "locus-pkg: %v\n", err)
}

const usage = `locus-pkg publishes and installs Scope packages.

Usage:
  locus-pkg [--scope <dir>] [--json] publish <oci-tag>
  locus-pkg [--scope <dir>] [--frozen] [--json] install
  locus-pkg [--json] version
  locus-pkg help
`
