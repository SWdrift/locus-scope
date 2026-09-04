package scope

import (
	"bufio"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

const (
	locusIgnoreFileName  = ".locusignore"
	definitionFileMarker = ".locus"
	definitionJSONSuffix = definitionFileMarker + ".json"
	definitionYAMLSuffix = definitionFileMarker + ".yaml"
	definitionYMLSuffix  = definitionFileMarker + ".yml"
)

var definitionFileSuffixes = [...]string{definitionJSONSuffix, definitionYAMLSuffix, definitionYMLSuffix}

type scopeIgnorePattern struct {
	value         string
	directoryOnly bool
	basenameOnly  bool
}

func discoverScopeDocuments(directory string, entries []os.DirEntry) ([]string, []string, error) {
	manifests := manifestNames(entries)
	patterns, err := readScopeIgnore(directory)
	if err != nil {
		return nil, nil, err
	}

	definitions := make([]string, 0)
	var walk func(string, string, []os.DirEntry) error
	walk = func(current, relativeDirectory string, currentEntries []os.DirEntry) error {
		for _, entry := range currentEntries {
			name := entry.Name()
			relative := name
			if relativeDirectory != "" {
				relative = path.Join(relativeDirectory, name)
			}

			if entry.IsDir() {
				if name == ".git" || name == ".locus" || matchesScopeIgnore(patterns, relative, true) {
					continue
				}
				child := filepath.Join(current, name)
				childEntries, err := os.ReadDir(child)
				if err != nil {
					return fmt.Errorf("read scope directory %q: %w", child, err)
				}
				if hasManifest(childEntries) {
					continue
				}
				if err := walk(child, relative, childEntries); err != nil {
					return err
				}
				continue
			}

			if slices.Contains(manifestFileNames, name) || !isDefinitionFile(name) || matchesScopeIgnore(patterns, relative, false) {
				continue
			}
			definitions = append(definitions, relative)
		}
		return nil
	}
	if err := walk(directory, "", entries); err != nil {
		return nil, nil, err
	}

	sort.Strings(manifests)
	sort.Strings(definitions)
	return manifests, definitions, nil
}

func manifestNames(entries []os.DirEntry) []string {
	manifests := make([]string, 0, 1)
	for _, entry := range entries {
		if !entry.IsDir() && slices.Contains(manifestFileNames, entry.Name()) {
			manifests = append(manifests, entry.Name())
		}
	}
	return manifests
}

func hasManifest(entries []os.DirEntry) bool {
	for _, entry := range entries {
		if !entry.IsDir() && slices.Contains(manifestFileNames, entry.Name()) {
			return true
		}
	}
	return false
}

func isDefinitionFile(name string) bool {
	for _, suffix := range definitionFileSuffixes {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}
	return false
}

func readScopeIgnore(directory string) ([]scopeIgnorePattern, error) {
	ignorePath := filepath.Join(directory, locusIgnoreFileName)
	file, err := os.Open(ignorePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read %s: %w", ignorePath, err)
	}
	defer file.Close()

	patterns := make([]scopeIgnorePattern, 0)
	scanner := bufio.NewScanner(file)
	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		line := strings.TrimSpace(strings.TrimPrefix(scanner.Text(), "\ufeff"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "!") {
			return nil, fmt.Errorf("%s:%d: negated patterns are not supported", ignorePath, lineNumber)
		}
		if strings.Contains(line, "\\") {
			return nil, fmt.Errorf("%s:%d: ignore patterns must use '/' separators", ignorePath, lineNumber)
		}

		directoryOnly := strings.HasSuffix(line, "/")
		anchored := strings.HasPrefix(line, "/")
		line = strings.TrimPrefix(strings.TrimSuffix(line, "/"), "/")
		segments := strings.Split(line, "/")
		if line == "" || slices.Contains(segments, "") || slices.Contains(segments, ".") || slices.Contains(segments, "..") {
			return nil, fmt.Errorf("%s:%d: invalid scope-relative ignore pattern %q", ignorePath, lineNumber, scanner.Text())
		}
		if strings.Contains(line, "**") {
			return nil, fmt.Errorf("%s:%d: '**' patterns are not supported", ignorePath, lineNumber)
		}
		if _, err := path.Match(line, ""); err != nil {
			return nil, fmt.Errorf("%s:%d: invalid ignore pattern %q: %w", ignorePath, lineNumber, scanner.Text(), err)
		}
		patterns = append(patterns, scopeIgnorePattern{
			value:         line,
			directoryOnly: directoryOnly,
			basenameOnly:  !anchored && !strings.Contains(line, "/"),
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read %s: %w", ignorePath, err)
	}
	return patterns, nil
}

func matchesScopeIgnore(patterns []scopeIgnorePattern, relative string, directory bool) bool {
	for _, pattern := range patterns {
		if pattern.directoryOnly && !directory {
			continue
		}
		candidate := relative
		if pattern.basenameOnly {
			candidate = path.Base(relative)
		}
		matched, _ := path.Match(pattern.value, candidate)
		if matched {
			return true
		}
	}
	return false
}
