package scopecli_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"locus-scope/internal/scope"
	"locus-scope/internal/scopecli"
)

func TestRunPreservesValidationOutputs(t *testing.T) {
	workspace := commandWorkspace()

	var stdout, stderr bytes.Buffer
	if code := scopecli.Run(workspace, []string{"validate"}, &stdout, &stderr); code != 0 {
		t.Fatalf("text validate exit code = %d, stderr = %q", code, stderr.String())
	}
	wantText := "valid: file:///consumer (2 scopes, 2 entities, 1 relations)\n"
	if stdout.String() != wantText || stderr.Len() != 0 {
		t.Fatalf("text validate stdout = %q, stderr = %q, want %q", stdout.String(), stderr.String(), wantText)
	}

	stdout.Reset()
	stderr.Reset()
	if code := scopecli.Run(workspace, []string{"--json", "validate"}, &stdout, &stderr); code != 0 {
		t.Fatalf("JSON validate exit code = %d, stderr = %q", code, stderr.String())
	}
	wantJSON := "{\n  \"valid\": true,\n  \"root\": \"file:///consumer\",\n  \"scopes\": 2,\n  \"entities\": 2,\n  \"relations\": 1\n}\n"
	if stdout.String() != wantJSON || stderr.Len() != 0 {
		t.Fatalf("JSON validate stdout = %q, stderr = %q, want %q", stdout.String(), stderr.String(), wantJSON)
	}
}

func TestRunDispatchesEveryQueryWithJSONShapes(t *testing.T) {
	workspace := commandWorkspace()
	tests := []struct {
		arguments []string
		field     string
	}{
		{[]string{"scope", "show", "--json"}, "id"},
		{[]string{"scope", "list", "--json"}, "scopes"},
		{[]string{"entity", "list", "--json"}, "entities"},
		{[]string{"entity", "show", "root", "--json"}, "entity"},
		{[]string{"relation", "list", "--json"}, "relations"},
		{[]string{"resolve", "root", "--json"}, "entity"},
	}
	for _, test := range tests {
		t.Run(strings.Join(test.arguments[:len(test.arguments)-1], " "), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := scopecli.Run(workspace, test.arguments, &stdout, &stderr); code != 0 {
				t.Fatalf("exit code = %d, stderr = %q", code, stderr.String())
			}
			var result map[string]any
			if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
				t.Fatalf("decode output %q: %v", stdout.String(), err)
			}
			if _, exists := result[test.field]; !exists {
				t.Fatalf("output %q has no %q field", stdout.String(), test.field)
			}
			if stderr.Len() != 0 {
				t.Fatalf("stderr = %q", stderr.String())
			}
		})
	}
}

func TestRunKeepsUsageErrorsOnStderr(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := scopecli.Run(commandWorkspace(), []string{"--json", "unknown"}, &stdout, &stderr)
	if code != 2 || stdout.Len() != 0 {
		t.Fatalf("exit code = %d, stdout = %q", code, stdout.String())
	}
	var failure struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(stderr.Bytes(), &failure); err != nil {
		t.Fatalf("decode failure %q: %v", stderr.String(), err)
	}
	if failure.Error != `unknown command "unknown"; run locus-scope help` {
		t.Fatalf("error = %q", failure.Error)
	}
}

func commandWorkspace() *scope.Workspace {
	rootKey := scope.ScopeKey("file:///consumer")
	packageKey := scope.ScopeKey("npm:@example/base@1.0.0")
	return &scope.Workspace{
		Root: rootKey,
		Scopes: map[scope.ScopeKey]*scope.Scope{
			rootKey: {
				Key:      rootKey,
				Manifest: scope.Manifest{ID: "consumer", Imports: map[string]string{"base": "@example/base"}, Exports: []string{"root"}},
				Entities: map[string]scope.Entity{"root": {ID: "root", Properties: map[string]any{"kind": "consumer"}}},
				Imports:  map[string]scope.ScopeKey{"base": packageKey},
			},
			packageKey: {
				Key:      packageKey,
				Manifest: scope.Manifest{ID: "base", Imports: map[string]string{}, Exports: []string{"database"}},
				Entities: map[string]scope.Entity{"database": {ID: "database", Properties: map[string]any{"engine": "postgres"}}},
				Imports:  map[string]scope.ScopeKey{},
			},
		},
		Relations: []scope.Relation{{
			From: scope.EntityKey{Scope: rootKey, ID: "root"},
			Name: "uses",
			To:   scope.EntityKey{Scope: packageKey, ID: "database"},
		}},
	}
}
