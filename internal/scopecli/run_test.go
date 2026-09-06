package scopecli_test

import (
	"bytes"
	"strings"
	"testing"

	"locus-scope/internal/scope"
	"locus-scope/internal/scopecli"
)

func workspace() *scope.Workspace {
	key := scope.ScopeKey("file:///workspace")
	return &scope.Workspace{Root: key, Scopes: map[scope.ScopeKey]*scope.Scope{key: {
		Key: key, Manifest: scope.Manifest{ID: "app", Imports: map[string]string{}, Exports: []string{"api", "db"}}, Imports: map[string]scope.ScopeKey{}, ManifestFile: "locus.yaml",
		Entities: map[string]scope.Entity{
			"api": {ID: "api", LocalID: "api", Properties: map[string]any{"type": "service"}, Source: scope.Provenance{Scope: key, File: "scope.locus.yaml", Line: 2, Index: 1}},
			"db":  {ID: "db", LocalID: "db", Properties: map[string]any{"type": "database"}, Source: scope.Provenance{Scope: key, File: "scope.locus.yaml", Line: 4, Index: 2}},
		},
	}}, Relations: []scope.Relation{{From: scope.EntityKey{Scope: key, ID: "api"}, Type: "depends_on", To: scope.EntityKey{Scope: key, ID: "db"}, FromRef: "api", ToRef: "db", Properties: map[string]any{"critical": true}, Source: scope.Provenance{Scope: key, File: "scope.locus.yaml", Line: 7, Index: 1}}}}
}

func run(t *testing.T, arguments ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	loaded := workspace()
	context := scopecli.Context{
		Stdin:    strings.NewReader(""),
		Load:     func() (*scope.Workspace, error) { return loaded, nil },
		LoadPath: func(string) (*scope.Workspace, error) { return loaded, nil },
	}
	code := scopecli.Run(context, arguments, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func TestEntityQueryJSONGolden(t *testing.T) {
	code, stdout, stderr := run(t, "entity", "type=service", "--source", "--json")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	const golden = `[{"key":{"scope":"file:///workspace","id":"api"},"ref":"api","object":{"id":"api","type":"service"},"source":{"scope":"file:///workspace","scopeId":"app","file":"scope.locus.yaml","line":2,"index":1}}]
`
	if stdout != golden {
		t.Fatalf("stdout:\n%s\nwant:\n%s", stdout, golden)
	}
}

func TestGraphAndPathJSONGolden(t *testing.T) {
	code, stdout, stderr := run(t, "graph", "api", "--depth", "1", "--via", "type=depends_on", "--json")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	const graphGolden = `{"seeds":[{"key":{"scope":"file:///workspace","id":"api"},"ref":"api","object":{"id":"api","type":"service"}}],"depth":1,"nodes":[{"key":{"scope":"file:///workspace","id":"api"},"ref":"api","object":{"id":"api","type":"service"}},{"key":{"scope":"file:///workspace","id":"db"},"ref":"db","object":{"id":"db","type":"database"}}],"relations":[{"fromKey":{"scope":"file:///workspace","id":"api"},"toKey":{"scope":"file:///workspace","id":"db"},"object":{"critical":true,"from":"api","to":"db","type":"depends_on"}}]}
`
	if stdout != graphGolden {
		t.Fatalf("graph stdout:\n%s\nwant:\n%s", stdout, graphGolden)
	}
	code, stdout, stderr = run(t, "path", "api", "db", "--json")
	if code != 0 || stderr != "" || !strings.Contains(stdout, `"relations":[{"fromKey"`) {
		t.Fatalf("path code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func TestRemainingCommandJSONGoldens(t *testing.T) {
	tests := []struct {
		name      string
		arguments []string
		golden    string
	}{
		{"validate", []string{"validate", "--json"}, "{\"valid\":true,\"root\":\"file:///workspace\",\"scopes\":1,\"entities\":2,\"relations\":1}\n"},
		{"scope-show", []string{"scope", ".", "--json"}, "{\"key\":\"file:///workspace\",\"ref\":\".\",\"object\":{\"exports\":[\"api\",\"db\"],\"id\":\"app\",\"imports\":{},\"root\":true}}\n"},
		{"group-list", []string{"group", "--json"}, "[]\n"},
		{"relation-find", []string{"relation", "type=depends_on", "--json"}, "[{\"fromKey\":{\"scope\":\"file:///workspace\",\"id\":\"api\"},\"toKey\":{\"scope\":\"file:///workspace\",\"id\":\"db\"},\"object\":{\"critical\":true,\"from\":\"api\",\"to\":\"db\",\"type\":\"depends_on\"}}]\n"},
		{"diff", []string{"diff", "path:left", "path:right", "--json"}, "{\"added\":{\"entities\":[],\"relations\":[]},\"removed\":{\"entities\":[],\"relations\":[]},\"changed\":{\"entities\":[],\"relations\":[]}}\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			code, stdout, stderr := run(t, test.arguments...)
			if code != 0 || stderr != "" || stdout != test.golden {
				t.Fatalf("code=%d stderr=%q\nstdout:\n%s\nwant:\n%s", code, stderr, stdout, test.golden)
			}
		})
	}
}

func TestCommandAndOperationalErrorsUseDifferentExitCodes(t *testing.T) {
	code, _, stderr := run(t, "graph", "api", "--depth", "bad", "--json")
	if code != 2 || !strings.Contains(stderr, "non-negative integer") {
		t.Fatalf("command error code=%d stderr=%q", code, stderr)
	}
	code, _, stderr = run(t, "path", "db", "api", "--json")
	if code != 1 || !strings.Contains(stderr, "no directed path") {
		t.Fatalf("operational error code=%d stderr=%q", code, stderr)
	}
}
