package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunExecutesOneScopeCLIRequest(t *testing.T) {
	root := hostTestDirectory(t, "success")
	writeHostTestFile(t, filepath.Join(root, "locus.yaml"), "id: root\n")
	requestBody := map[string]any{
		"version":          2,
		"workingDirectory": root,
		"arguments":        []string{"validate", "--json"},
		"root": map[string]any{
			"scopeRoot": root, "packageRoot": "", "dependencies": map[string]string{},
		},
		"packages": map[string]any{},
	}
	encoded, err := json.Marshal(requestBody)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if exitCode := run(bytes.NewReader(encoded), &output); exitCode != 0 {
		t.Fatalf("run exit code = %d, output = %s", exitCode, output.String())
	}
	var result response
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.Version != 2 || result.ExitCode != 0 || result.Stderr != "" {
		t.Fatalf("response = %#v", result)
	}
	if !strings.Contains(result.Stdout, `"valid":true`) {
		t.Fatalf("stdout = %q", result.Stdout)
	}
}

func TestRunFormatsWorkspaceLoadFailureAsJSONWithNPMAdvice(t *testing.T) {
	root := hostTestDirectory(t, "json-load-failure")
	writeHostTestFile(t, filepath.Join(root, "locus.yaml"), "id: root\nimports:\n  missing: '@example/missing'\n")
	requestBody := map[string]any{
		"version":          2,
		"workingDirectory": root,
		"arguments":        []string{"validate", "--json"},
		"root": map[string]any{
			"scopeRoot": root, "packageRoot": "", "dependencies": map[string]string{},
		},
		"packages": map[string]any{},
	}
	encoded, err := json.Marshal(requestBody)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if exitCode := run(bytes.NewReader(encoded), &output); exitCode != 1 {
		t.Fatalf("run exit code = %d, output = %s", exitCode, output.String())
	}
	var result response
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	var failure struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(result.Stderr), &failure); err != nil {
		t.Fatalf("stderr is not one JSON failure: %v; stderr = %q", err, result.Stderr)
	}
	if !strings.Contains(failure.Error, "run pnpm add @example/missing or npm install @example/missing") ||
		strings.Contains(failure.Error, "locus-pkg") {
		t.Fatalf("error = %q", failure.Error)
	}
}

func TestRunRejectsUnknownDuplicateAndTrailingJSON(t *testing.T) {
	root := hostTestDirectory(t, "invalid")
	cases := map[string]string{
		"unknown":   `{"version":2,"workingDirectory":"` + slash(root) + `","arguments":[],"root":{"scopeRoot":"` + slash(root) + `","packageRoot":"","dependencies":{}},"packages":{},"extra":true}`,
		"duplicate": `{"version":2,"version":2,"workingDirectory":"` + slash(root) + `","arguments":[],"root":{"scopeRoot":"` + slash(root) + `","packageRoot":"","dependencies":{}},"packages":{}}`,
		"trailing":  `{"version":2,"workingDirectory":"` + slash(root) + `","arguments":[],"root":{"scopeRoot":"` + slash(root) + `","packageRoot":"","dependencies":{}},"packages":{}} {}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			var output bytes.Buffer
			if exitCode := run(strings.NewReader(body), &output); exitCode != 1 {
				t.Fatalf("exit code = %d, output = %s", exitCode, output.String())
			}
			var result response
			if err := json.Unmarshal(output.Bytes(), &result); err != nil {
				t.Fatalf("response is not valid JSON: %v", err)
			}
			if result.Version != 2 || result.ExitCode != 1 || result.Stderr == "" {
				t.Fatalf("response = %#v", result)
			}
		})
	}
}

func TestRunRejectsPackageEntryEscape(t *testing.T) {
	root := hostTestDirectory(t, "escape")
	body := `{"version":2,"workingDirectory":"` + slash(root) + `","arguments":["validate"],"root":{"scopeRoot":"` + slash(root) + `","packageRoot":"","dependencies":{}},"packages":{"npm:@example/pkg@1.0.0":{"root":"` + slash(root) + `","entry":"../locus.yaml","dependencies":{}}}}`
	var output bytes.Buffer
	if exitCode := run(strings.NewReader(body), &output); exitCode != 1 {
		t.Fatalf("exit code = %d", exitCode)
	}
	if !strings.Contains(output.String(), "entry escapes its root") {
		t.Fatalf("output = %s", output.String())
	}
}
func TestRunRejectsSecondScopeManifestInPackage(t *testing.T) {
	root := hostTestDirectory(t, "duplicate-package-scope")
	scopeRoot := filepath.Join(root, "consumer")
	packageRoot := filepath.Join(root, "package")
	if err := os.MkdirAll(filepath.Join(packageRoot, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(scopeRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	writeHostTestFile(t, filepath.Join(scopeRoot, "locus.yaml"), "id: consumer\n")
	writeHostTestFile(t, filepath.Join(packageRoot, "package.json"), `{"name":"@example/pkg","version":"1.0.0","dependencies":{},"locus":{"entry":"locus.yaml"}}`)
	writeHostTestFile(t, filepath.Join(packageRoot, "locus.yaml"), "id: package\n")
	writeHostTestFile(t, filepath.Join(packageRoot, "nested", "locus.yaml"), "id: nested\n")

	requestBody := map[string]any{
		"version":          2,
		"workingDirectory": scopeRoot,
		"arguments":        []string{"validate", "--json"},
		"root": map[string]any{
			"scopeRoot": scopeRoot, "packageRoot": "", "dependencies": map[string]string{"@example/pkg": "npm:@example/pkg@1.0.0"},
		},
		"packages": map[string]any{
			"npm:@example/pkg@1.0.0": map[string]any{"root": packageRoot, "entry": "locus.yaml", "dependencies": map[string]string{}},
		},
	}
	encoded, err := json.Marshal(requestBody)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if exitCode := run(bytes.NewReader(encoded), &output); exitCode != 1 {
		t.Fatalf("exit code = %d, output = %s", exitCode, output.String())
	}
	if !strings.Contains(output.String(), "must contain exactly the declared Scope manifest") {
		t.Fatalf("output = %s", output.String())
	}
}

func hostTestDirectory(t *testing.T, name string) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "temp", "node-host-test", name))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(root); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

func writeHostTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func slash(path string) string {
	return strings.ReplaceAll(path, `\`, `\\`)
}
