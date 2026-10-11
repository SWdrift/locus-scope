package scopecli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"locus-scope/internal/scope"
	"locus-scope/internal/scopecli"
)

func TestMutationRoundTripAndRollback(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", "temp", "e2e-run", "scopecli-mutation"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(root); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "locus.yaml"), []byte("id: app\nexports: [api]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "entities.locus.yaml"), []byte("entities:\n  - id: api\n    type: service\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	load := func() (*scope.Workspace, error) {
		source, err := scope.NewLocalSource(root)
		if err != nil {
			return nil, err
		}
		return scope.Load(source, scope.LocalResolver{})
	}
	runMutation := func(stdin string, arguments ...string) (int, string, string) {
		var stdout, stderr bytes.Buffer
		code := scopecli.Run(scopecli.Context{Stdin: strings.NewReader(stdin), WorkingDirectory: root, Load: load, Reload: load}, arguments, &stdout, &stderr)
		return code, stdout.String(), stderr.String()
	}
	if code, _, stderr := runMutation(`{"id":"db","type":"database","network":{"region":"tokyo"}}`, "entity", "add", "-", "--json"); code != 0 {
		t.Fatalf("entity add code=%d stderr=%s", code, stderr)
	}
	if code, _, stderr := runMutation("", "relation", "add", "api", "depends_on", "db", "critical=true", "--json"); code != 0 {
		t.Fatalf("relation add code=%d stderr=%s", code, stderr)
	}
	if code, _, _ := runMutation("", "entity", "remove", "db", "--json"); code != 1 {
		t.Fatalf("dangling removal exit=%d", code)
	}
	if code, stdout, stderr := runMutation("", "entity", "db", "--json"); code != 0 || !strings.Contains(stdout, `"region":"tokyo"`) || stderr != "" {
		t.Fatalf("rollback query code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if code, _, stderr := runMutation("", "relation", "remove", "api", "depends_on", "db", "--json"); code != 0 {
		t.Fatalf("relation remove code=%d stderr=%s", code, stderr)
	}
	if code, _, stderr := runMutation("", "entity", "remove", "db", "--json"); code != 0 {
		t.Fatalf("entity remove code=%d stderr=%s", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(root, "scope.locus.yaml")); err != nil {
		t.Fatalf("default definition not retained: %v", err)
	}
}

func TestMutationRejectsSymlinkOutsideRoot(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", "temp", "e2e-run", "scopecli-symlink"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(root); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "inside"), 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(filepath.Dir(root), "scopecli-symlink-outside.locus.yaml")
	if err := os.WriteFile(outside, []byte("entities:\n  - id: keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(root)
		_ = os.Remove(outside)
	})
	link := filepath.Join(root, "inside", "target.locus.yaml")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "locus.yaml"), []byte("id: app\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	load := func() (*scope.Workspace, error) {
		source, err := scope.NewLocalSource(root)
		if err != nil {
			return nil, err
		}
		return scope.Load(source, scope.LocalResolver{})
	}
	var stdout, stderr bytes.Buffer
	code := scopecli.Run(scopecli.Context{WorkingDirectory: root, Load: load, Reload: load},
		[]string{"entity", "add", "new", "type=service", "--file", "inside/target.locus.yaml", "--json"},
		&stdout, &stderr)
	if code != 1 || stdout.String() != "" || stderr.String() != "{\"error\":\"scope operation failed\"}\n" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if strings.Contains(stderr.String(), outside) {
		t.Fatalf("stderr exposed outside path: %q", stderr.String())
	}
	data, err := os.ReadFile(outside)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "entities:\n  - id: keep\n" {
		t.Fatalf("outside target was modified: %q", data)
	}
}
