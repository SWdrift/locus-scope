package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunExecutesResolvedEnvironment(t *testing.T) {
	root := hostTestDirectory(t, "success")
	writeHostTestFile(t, filepath.Join(root, "locus.yaml"), "id: root\n")
	result := runRequest(t, map[string]any{
		"version": protocolVersion, "workingDirectory": root, "arguments": []string{"validate", "--json"},
		"environment": map[string]any{"rootScope": root, "rootDependencies": map[string]string{}, "packages": map[string]any{}},
	})
	if result.ExitCode != 0 || result.Stderr != "" || !strings.Contains(result.Stdout, `"valid":true`) {
		t.Fatalf("response = %#v", result)
	}
}

func TestRunReturnsScopeFailuresThroughCLIContract(t *testing.T) {
	root := hostTestDirectory(t, "missing-edge")
	writeHostTestFile(t, filepath.Join(root, "locus.yaml"), "id: root\nimports:\n  app: app\n")
	result := runRequest(t, map[string]any{
		"version": protocolVersion, "workingDirectory": root, "arguments": []string{"validate", "--json"},
		"environment": map[string]any{"rootScope": root, "rootDependencies": map[string]string{}, "packages": map[string]any{}},
	})
	if result.ExitCode != 1 || result.Stdout != "" {
		t.Fatalf("response = %#v", result)
	}
	var failure map[string]string
	if err := json.Unmarshal([]byte(result.Stderr), &failure); err != nil || !strings.Contains(failure["error"], "resolved environment") {
		t.Fatalf("stderr = %q, decode error = %v", result.Stderr, err)
	}
}

func TestRunRejectsUnknownProtocolFields(t *testing.T) {
	root := hostTestDirectory(t, "unknown")
	result := runRequest(t, map[string]any{
		"version": protocolVersion, "workingDirectory": root, "arguments": []string{"version"},
		"environment": map[string]any{}, "unexpected": true,
	})
	if result.ExitCode != 1 || !strings.Contains(result.Stderr, "unknown field") {
		t.Fatalf("response = %#v", result)
	}
}

func runRequest(t *testing.T, value any) response {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	run(bytes.NewReader(body), &output)
	var result response
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode response %q: %v", output.String(), err)
	}
	if result.Version != protocolVersion {
		t.Fatalf("protocol version = %d", result.Version)
	}
	return result
}

func hostTestDirectory(t *testing.T, name string) string {
	t.Helper()
	root := filepath.Join("..", "..", "temp", "node-host-test", name)
	if err := os.RemoveAll(root); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	return absolute
}

func writeHostTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
