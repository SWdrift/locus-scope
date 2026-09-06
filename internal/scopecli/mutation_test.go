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
