package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"locus-scope/internal/buildinfo"
	"locus-scope/internal/packages"
)

func TestRunInstallsLocalScopeWithFlagsAfterCommand(t *testing.T) {
	root := filepath.Join("..", "..", "temp", "e2e-run", "pkg-cli-unit")
	if err := os.RemoveAll(root); err != nil {
		t.Fatalf("clear test root: %v", err)
	}
	project := filepath.Join(root, "project")
	dockerConfig := filepath.Join(root, "docker")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatalf("create project: %v", err)
	}
	if err := os.MkdirAll(dockerConfig, 0o755); err != nil {
		t.Fatalf("create Docker config directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(project, "locus.yaml"), []byte("id: cli\n"), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dockerConfig, "config.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write Docker config: %v", err)
	}
	t.Setenv("HOME", root)
	t.Setenv("USERPROFILE", root)
	t.Setenv("DOCKER_CONFIG", dockerConfig)

	var stdout, stderr bytes.Buffer
	if code := run([]string{"install", "--scope", project, "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("run exit code = %d, stderr = %s", code, stderr.String())
	}
	var result packages.InstallResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode output %q: %v", stdout.String(), err)
	}
	if !result.Valid || result.Scopes != 1 {
		t.Fatalf("install output = %#v", result)
	}
	if _, err := os.Stat(filepath.Join(project, "locus.lock")); err != nil {
		t.Fatalf("generated lock: %v", err)
	}
}

func TestRunReportsJSONUsageFailure(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--json", "unknown"}, &stdout, &stderr); code != 2 {
		t.Fatalf("run exit code = %d", code)
	}
	var failure struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(stderr.Bytes(), &failure); err != nil || failure.Error == "" {
		t.Fatalf("failure output = %q, error = %v", stderr.String(), err)
	}
}

func TestRunRejectsInvalidPublishInvocation(t *testing.T) {
	tests := []struct {
		name      string
		arguments []string
		want      string
	}{
		{name: "missing target", arguments: []string{"publish"}, want: "requires exactly one OCI target"},
		{name: "multiple targets", arguments: []string{"publish", "oci://registry.example/one:v1", "oci://registry.example/two:v1"}, want: "requires exactly one OCI target"},
		{name: "frozen", arguments: []string{"--frozen", "publish", "oci://registry.example/package:v1"}, want: "only valid with install"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := run(test.arguments, &stdout, &stderr); code != 2 {
				t.Fatalf("run exit code = %d, stderr = %s", code, stderr.String())
			}
			if !strings.Contains(stderr.String(), test.want) {
				t.Fatalf("stderr = %q, want fragment %q", stderr.String(), test.want)
			}
		})
	}
}

func TestHelpIncludesPublishUsage(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"help"}, &stdout, &stderr); code != 0 {
		t.Fatalf("run exit code = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "publish <oci-tag>") {
		t.Fatalf("help output = %q", stdout.String())
	}
}

func TestRunShowsVersionWithoutScope(t *testing.T) {
	for _, arguments := range [][]string{{"version"}, {"--version"}} {
		var stdout, stderr bytes.Buffer
		if code := run(arguments, &stdout, &stderr); code != 0 {
			t.Fatalf("run %v exit code = %d, stderr = %s", arguments, code, stderr.String())
		}
		want := "locus-pkg " + buildinfo.Version + "\n"
		if stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("run %v output = %q, stderr = %q, want %q", arguments, stdout.String(), stderr.String(), want)
		}
	}

	var stdout, stderr bytes.Buffer
	if code := run([]string{"--json", "version"}, &stdout, &stderr); code != 0 {
		t.Fatalf("JSON version exit code = %d, stderr = %s", code, stderr.String())
	}
	var info struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &info); err != nil {
		t.Fatalf("decode version output %q: %v", stdout.String(), err)
	}
	if info.Name != "locus-pkg" || info.Version != buildinfo.Version {
		t.Fatalf("version output = %#v", info)
	}
}
