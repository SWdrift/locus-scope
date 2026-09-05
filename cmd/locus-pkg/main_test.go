package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"locus-scope/internal/buildinfo"
	"locus-scope/internal/purepkg"
)

func TestRunInstallsEmptyProjectWithFlagsAfterCommand(t *testing.T) {
	root := cliTestDirectory(t, "install")
	project := filepath.Join(root, "project")
	writeCLITestFile(t, filepath.Join(project, "package.json"), "{\n  \"name\": \"example-project\",\n  \"version\": \"1.0.0\",\n  \"dependencies\": {},\n  \"locus\": {\"entry\": \"locus.yaml\"}\n}\n")
	writeCLITestFile(t, filepath.Join(project, "locus.yaml"), "id: cli\n")
	t.Setenv("HOME", root)
	t.Setenv("USERPROFILE", root)

	original, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(project); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(original) })
	var stdout, stderr bytes.Buffer
	if code := run([]string{"install", "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("run exit code = %d, stderr = %s", code, stderr.String())
	}
	var result purepkg.InstallResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode output %q: %v", stdout.String(), err)
	}
	if !result.Valid || result.Scopes != 1 || result.Packages != 0 {
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

func TestRunRejectsInvalidMutationOptions(t *testing.T) {
	tests := []struct {
		name      string
		arguments []string
		want      string
	}{
		{name: "explicit frozen install", arguments: []string{"install", "@example/app@^1", "--frozen-lockfile"}, want: "cannot be used with explicit install"},
		{name: "missing uninstall name", arguments: []string{"uninstall"}, want: "requires at least one"},
		{name: "offline publish", arguments: []string{"publish", "--offline"}, want: "does not support"},
		{name: "frozen publish", arguments: []string{"--frozen-lockfile", "publish"}, want: "does not support"},
		{name: "list argument", arguments: []string{"list", "extra"}, want: "does not accept positional"},
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

func TestFindRootsWalksToMatchingAncestor(t *testing.T) {
	root := cliTestDirectory(t, "roots")
	project := filepath.Join(root, "project")
	nested := filepath.Join(project, "nested", "work")
	writeCLITestFile(t, filepath.Join(project, "package.json"), "{\"name\":\"project\",\"version\":\"1.0.0\",\"locus\":{\"entry\":\"locus.yaml\"}}\n")
	writeCLITestFile(t, filepath.Join(project, "locus.yaml"), "id: project\n")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	projectRoot, err := findProjectRoot(nested)
	if err != nil || projectRoot != project {
		t.Fatalf("findProjectRoot = %q, %v", projectRoot, err)
	}
	packageRoot, err := findPackageRoot(nested)
	if err != nil || packageRoot != project {
		t.Fatalf("findPackageRoot = %q, %v", packageRoot, err)
	}
}

func TestHelpIncludesNpmLifecycleCommands(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"help"}, &stdout, &stderr); code != 0 {
		t.Fatalf("run exit code = %d, stderr = %s", code, stderr.String())
	}
	for _, fragment := range []string{"install [<package-spec>...]", "uninstall <package>...", "--frozen-lockfile", "--registry <url>"} {
		if !strings.Contains(stdout.String(), fragment) {
			t.Fatalf("help output %q does not contain %q", stdout.String(), fragment)
		}
	}
}

func TestRunShowsVersionWithoutProject(t *testing.T) {
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

func cliTestDirectory(t *testing.T, name string) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "temp", "pkg-cli-unit", name))
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

func writeCLITestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
