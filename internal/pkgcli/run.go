// Package pkgcli implements the locus-pkg command-line frontend.
package pkgcli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"locus-scope/internal/buildinfo"
	"locus-scope/internal/pkgapp"
)

type options struct {
	registry       string
	frozenLockfile bool
	offline        bool
	jsonOutput     bool
}

// Run parses and executes one locus-pkg invocation.
func Run(arguments []string, stdout, stderr io.Writer) int {
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
	if err := validateInvocation(command, opts); err != nil {
		writeFailure(stderr, opts.jsonOutput, err)
		return 2
	}

	workingDirectory, err := os.Getwd()
	if err != nil {
		writeFailure(stderr, opts.jsonOutput, fmt.Errorf("get current directory: %w", err))
		return 1
	}
	var root string
	if command[0] == "pack" || command[0] == "publish" {
		root, err = findPackageRoot(workingDirectory)
	} else {
		root, err = findProjectRoot(workingDirectory)
	}
	if err != nil {
		writeFailure(stderr, opts.jsonOutput, err)
		return 1
	}

	manager := pkgapp.New(root, pkgapp.Options{
		Registry: opts.registry, FrozenLockfile: opts.frozenLockfile, Offline: opts.offline,
	})
	ctx := context.Background()
	switch command[0] {
	case "install":
		result, err := manager.Install(ctx, command[1:])
		return writeInstallResult(stdout, stderr, opts.jsonOutput, result, err)
	case "uninstall":
		result, err := manager.Uninstall(ctx, command[1:])
		return writeInstallResult(stdout, stderr, opts.jsonOutput, result, err)
	case "update":
		result, err := manager.Update(ctx, command[1:])
		return writeInstallResult(stdout, stderr, opts.jsonOutput, result, err)
	case "list":
		result, err := manager.List()
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
			fmt.Fprintf(stdout, "root: %s\n", result.Root)
			for _, dependency := range result.Dependencies {
				writeDependency(stdout, dependency, "")
			}
		}
		return 0
	case "pack":
		result, err := manager.Pack()
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
			fmt.Fprintf(stdout, "packed: %s\nname: %s\nversion: %s\nintegrity: %s\nfiles: %d\n", result.Filename, result.Name, result.Version, result.Integrity, len(result.Files))
		}
		return 0
	case "publish":
		result, err := manager.Publish(ctx)
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
			fmt.Fprintf(stdout, "published: %s@%s\nregistry: %s\nintegrity: %s\n", result.Name, result.Version, result.Registry, result.Integrity)
		}
		return 0
	default:
		panic("validated command was not dispatched")
	}
}

func validateInvocation(command []string, opts options) error {
	switch command[0] {
	case "install":
		if opts.frozenLockfile && len(command) > 1 {
			return errors.New("--frozen-lockfile cannot be used with explicit install specs")
		}
	case "uninstall":
		if len(command) == 1 {
			return errors.New("uninstall requires at least one direct dependency name")
		}
	case "update":
	case "list", "pack", "publish":
		if len(command) != 1 {
			return fmt.Errorf("%s does not accept positional arguments", command[0])
		}
	default:
		return fmt.Errorf("unknown command %q; run locus-pkg help", command[0])
	}
	if command[0] == "publish" && (opts.offline || opts.frozenLockfile) {
		return errors.New("publish does not support --offline or --frozen-lockfile")
	}
	return nil
}

func parseArguments(arguments []string) (options, []string, error) {
	var opts options
	command := make([]string, 0, len(arguments))
	for index := 0; index < len(arguments); index++ {
		switch argument := arguments[index]; {
		case argument == "--json":
			opts.jsonOutput = true
		case argument == "--offline":
			opts.offline = true
		case argument == "--frozen-lockfile":
			opts.frozenLockfile = true
		case argument == "--registry":
			if index+1 == len(arguments) {
				return opts, nil, errors.New("--registry requires a URL")
			}
			index++
			opts.registry = arguments[index]
			if opts.registry == "" {
				return opts, nil, errors.New("--registry requires a URL")
			}
		case strings.HasPrefix(argument, "--registry="):
			opts.registry = strings.TrimPrefix(argument, "--registry=")
			if opts.registry == "" {
				return opts, nil, errors.New("--registry requires a URL")
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

func findProjectRoot(start string) (string, error) {
	current, err := absoluteDirectory(start)
	if err != nil {
		return "", err
	}
	for {
		if regularFile(filepath.Join(current, "package.json")) && hasScopeManifest(current) {
			return current, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("no project containing package.json and a root Scope manifest found from %q", start)
		}
		current = parent
	}
}

func findPackageRoot(start string) (string, error) {
	current, err := absoluteDirectory(start)
	if err != nil {
		return "", err
	}
	for {
		packageJSON := filepath.Join(current, "package.json")
		if regularFile(packageJSON) {
			body, err := os.ReadFile(packageJSON)
			if err != nil {
				return "", fmt.Errorf("read %s: %w", packageJSON, err)
			}
			var manifest struct {
				Locus *struct {
					Entry string `json:"entry"`
				} `json:"locus"`
			}
			if err := json.Unmarshal(body, &manifest); err != nil {
				return "", fmt.Errorf("decode %s: %w", packageJSON, err)
			}
			if manifest.Locus != nil && manifest.Locus.Entry != "" {
				return current, nil
			}
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("no package.json with locus.entry found from %q", start)
		}
		current = parent
	}
}

func absoluteDirectory(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve directory %q: %w", path, err)
	}
	information, err := os.Stat(absolute)
	if err != nil {
		return "", fmt.Errorf("inspect directory %q: %w", absolute, err)
	}
	if !information.IsDir() {
		return "", fmt.Errorf("%q is not a directory", absolute)
	}
	return absolute, nil
}

func hasScopeManifest(directory string) bool {
	for _, name := range []string{"locus.yaml", "locus.yml", "locus.json"} {
		if regularFile(filepath.Join(directory, name)) {
			return true
		}
	}
	return false
}

func regularFile(path string) bool {
	information, err := os.Stat(path)
	return err == nil && information.Mode().IsRegular()
}

func writeInstallResult(stdout, stderr io.Writer, jsonOutput bool, result pkgapp.InstallResult, err error) int {
	if err != nil {
		writeFailure(stderr, jsonOutput, err)
		return 1
	}
	if jsonOutput {
		if err := writeJSON(stdout, result); err != nil {
			writeFailure(stderr, true, err)
			return 1
		}
		return 0
	}
	fmt.Fprintf(stdout, "root: %s\nadded: %s\nremoved: %s\nupdated: %s\nreused: %d, fetched: %d, installed: %d, packages: %d\nscopes: %d, entities: %d, relations: %d\n",
		result.Root, joinFacts(result.Added), joinFacts(result.Removed), joinFacts(result.Updated), result.Reused, result.Fetched, result.Installed, result.Packages, result.Scopes, result.Entities, result.Relations)
	return 0
}

func writeDependency(output io.Writer, dependency pkgapp.ListDependency, indent string) {
	fmt.Fprintf(output, "%s%s@%s (%s)\n", indent, dependency.Name, dependency.Version, dependency.Identity)
	for _, child := range dependency.Dependencies {
		writeDependency(output, child, indent+"  ")
	}
}

func joinFacts(values []string) string {
	if len(values) == 0 {
		return "none"
	}
	values = append([]string(nil), values...)
	sort.Strings(values)
	return strings.Join(values, ", ")
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

const usage = `locus-pkg installs and publishes npm-compatible Locus packages.

Usage:
  locus-pkg [options] install [<package-spec>...]
  locus-pkg [options] uninstall <package>...
  locus-pkg [options] update [<package>...]
  locus-pkg [options] list
  locus-pkg [options] pack
  locus-pkg [options] publish
  locus-pkg [--json] version
  locus-pkg help

Options:
  --registry <url>     Override the npm Registry.
  --offline            Prohibit Registry requests.
  --frozen-lockfile    Require package.json and locus.lock to agree exactly.
  --json               Write stable JSON output.
`
