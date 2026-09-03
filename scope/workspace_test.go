package scope_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"locus-scope/scope"
)

func TestLoadProtocolExamples(t *testing.T) {
	workspace, err := scope.Load(repoPath("documents", "design", "protocol", "examples", "app"))
	if err != nil {
		t.Fatalf("load protocol example: %v", err)
	}
	if len(workspace.Scopes) != 2 {
		t.Fatalf("loaded scopes = %d, want 2", len(workspace.Scopes))
	}
	if len(workspace.Relations) != 4 {
		t.Fatalf("resolved relations = %d, want 4", len(workspace.Relations))
	}

	root := workspace.Scopes[workspace.Root]
	infraKey := root.Imports["infra"]
	if infraKey == "" {
		t.Fatal("app import projection infra was not loaded")
	}

	database := mustResolve(t, workspace, workspace.Root, "infra:database")
	if database != (scope.EntityKey{Scope: infraKey, ID: "database"}) {
		t.Fatalf("infra:database = %#v, want owner %q and id database", database, infraKey)
	}
	worker := mustResolve(t, workspace, workspace.Root, "infra:backend/worker")
	if worker != (scope.EntityKey{Scope: infraKey, ID: "backend/worker"}) {
		t.Fatalf("infra:backend/worker = %#v", worker)
	}

	properties := workspace.Scopes[database.Scope].Entities[database.ID].Properties
	if properties["engine"] != "postgres" {
		t.Fatalf("database engine property = %#v, want postgres", properties["engine"])
	}
	monitor := scope.EntityKey{Scope: infraKey, ID: "backend/monitor"}
	anything, ok := workspace.Scopes[monitor.Scope].Entities[monitor.ID].Properties["anything"].(map[string]any)
	nested, nestedOK := anything["nested"].(map[string]any)
	if !ok || !nestedOK || nested["values"] == nil {
		t.Fatalf("monitor nested property = %#v", anything)
	}
	if !hasRelation(workspace.Relations, worker, "reports_to", monitor) {
		t.Fatalf("grouped relation worker reports_to monitor missing: %#v", workspace.Relations)
	}
	api := scope.EntityKey{Scope: workspace.Root, ID: "api"}
	if !hasRelation(workspace.Relations, api, "uses", database) {
		t.Fatalf("cross-scope relation api uses database missing: %#v", workspace.Relations)
	}
}

func TestGroupedRelationKeepsExternalRootReference(t *testing.T) {
	rootDirectory := materializeCase(t, "group-fallback")
	workspace, err := scope.Load(rootDirectory)
	if err != nil {
		t.Fatalf("load grouped fallback fixture: %v", err)
	}

	worker := scope.EntityKey{Scope: workspace.Root, ID: "backend/worker"}
	database := scope.EntityKey{Scope: workspace.Root, ID: "database"}
	if !hasRelation(workspace.Relations, worker, "uses", database) {
		t.Fatalf("grouped relation rewrote root reference: %#v", workspace.Relations)
	}
}

func TestMultilevelReexportKeepsOriginalOwnership(t *testing.T) {
	rootDirectory := materializeCase(t, "reexport", "product")
	workspace, err := scope.Load(rootDirectory)
	if err != nil {
		t.Fatalf("load reexport fixture: %v", err)
	}

	resolved := mustResolve(t, workspace, workspace.Root, "app:infra:database")
	owner := workspace.Scopes[resolved.Scope]
	if owner.Manifest.ID != "infra" || resolved.ID != "database" {
		t.Fatalf("multilevel projection resolved to %#v owned by %q", resolved, owner.Manifest.ID)
	}
	if resolved.Scope == workspace.Root {
		t.Fatalf("import changed entity ownership to root: %#v", resolved)
	}

	_, err = workspace.Resolve(workspace.Root, "app:infra:secret")
	if err == nil || !strings.Contains(err.Error(), `scope "app"`) || !strings.Contains(err.Error(), `does not export "infra:secret"`) {
		t.Fatalf("hidden imported entity error = %v", err)
	}
}

func TestCyclicImportsLoadAndResolve(t *testing.T) {
	rootDirectory := materializeCase(t, "cycle", "a")
	workspace, err := scope.Load(rootDirectory)
	if err != nil {
		t.Fatalf("load cyclic fixture: %v", err)
	}
	if len(workspace.Scopes) != 2 {
		t.Fatalf("loaded cyclic scopes = %d, want 2", len(workspace.Scopes))
	}

	beta := mustResolve(t, workspace, workspace.Root, "b:beta")
	if workspace.Scopes[beta.Scope].Manifest.ID != "b" {
		t.Fatalf("b:beta owner = %#v", beta)
	}
	alpha := scope.EntityKey{Scope: workspace.Root, ID: "alpha"}
	if !hasRelation(workspace.Relations, alpha, "reaches", beta) {
		t.Fatalf("cyclic fixture relation missing: %#v", workspace.Relations)
	}
}

func TestDistinctSourcesMayShareManifestID(t *testing.T) {
	rootDirectory := materializeCase(t, "same-manifest-id", "root")
	workspace, err := scope.Load(rootDirectory)
	if err != nil {
		t.Fatalf("load same manifest ID fixture: %v", err)
	}

	east := mustResolve(t, workspace, workspace.Root, "east:item")
	west := mustResolve(t, workspace, workspace.Root, "west:item")
	if east.Scope == west.Scope {
		t.Fatalf("distinct sources collapsed to one scope key: %q", east.Scope)
	}
	if workspace.Scopes[east.Scope].Manifest.ID != "shared" || workspace.Scopes[west.Scope].Manifest.ID != "shared" {
		t.Fatalf("fixture logical IDs were not preserved")
	}
	if got := workspace.Scopes[east.Scope].Entities[east.ID].Properties["side"]; got != "east" {
		t.Fatalf("east entity property = %#v", got)
	}
	if got := workspace.Scopes[west.Scope].Entities[west.ID].Properties["side"]; got != "west" {
		t.Fatalf("west entity property = %#v", got)
	}
}

func TestValidationDiagnostics(t *testing.T) {
	tests := []struct {
		name string
		want []string
	}{
		{"duplicate-expanded", []string{"team.yaml", `entity "team/worker"`, "after group expansion"}},
		{"missing-relation", []string{"entities.yaml", `relation "source points_to absent"`, `end reference "absent"`}},
		{"validation/missing-manifest", []string{"manifest missing", "locus.yaml"}},
		{"validation/duplicate-manifest", []string{"multiple manifests", "locus.json", "locus.yaml"}},
		{"validation/missing-scope-id", []string{"locus.yaml", "scope id is required"}},
		{"validation/missing-entity-id", []string{"entities.yaml", "entity 1", "id is required"}},
		{"validation/missing-import", []string{"locus.yaml", `import "absent"`, "does-not-exist"}},
		{"validation/missing-projection", []string{"locus.yaml", `export "ghost:item"`, `does not import projection "ghost"`}},
		{"validation/malformed-relation", []string{"entities.yaml", "line 4", "exactly [from, relation, to]"}},
	}

	for _, test := range tests {
		t.Run(strings.ReplaceAll(test.name, "/", "-"), func(t *testing.T) {
			rootDirectory := materializeCase(t, test.name)
			_, err := scope.Load(rootDirectory)
			if err == nil {
				t.Fatal("Load succeeded, want validation error")
			}
			for _, fragment := range test.want {
				if !strings.Contains(err.Error(), fragment) {
					t.Fatalf("error %q does not contain %q", err, fragment)
				}
			}
		})
	}
}

func TestFindScopeWalksToNearestAncestor(t *testing.T) {
	rootDirectory := materializeCase(t, "cycle", "a")
	nested := filepath.Join(rootDirectory, "nested", "deeper")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("create nested fixture directory: %v", err)
	}

	found, err := scope.FindScope(nested)
	if err != nil {
		t.Fatalf("find scope: %v", err)
	}
	workspace, err := scope.Load(found)
	if err != nil {
		t.Fatalf("load found scope: %v", err)
	}
	if workspace.Scopes[workspace.Root].Manifest.ID != "a" {
		t.Fatalf("found root scope ID = %q", workspace.Scopes[workspace.Root].Manifest.ID)
	}
}

func mustResolve(t *testing.T, workspace *scope.Workspace, from scope.ScopeKey, ref string) scope.EntityKey {
	t.Helper()
	resolved, err := workspace.Resolve(from, ref)
	if err != nil {
		t.Fatalf("resolve %q: %v", ref, err)
	}
	return resolved
}

func hasRelation(relations []scope.Relation, from scope.EntityKey, name string, to scope.EntityKey) bool {
	for _, relation := range relations {
		if relation.From == from && relation.Name == name && relation.To == to {
			return true
		}
	}
	return false
}

func materializeCase(t *testing.T, name string, rootParts ...string) string {
	t.Helper()
	source := repoPath("test", "e2e", "case", filepath.FromSlash(name))
	destination := repoPath("temp", "e2e-run", filepath.FromSlash(name))
	if err := os.RemoveAll(destination); err != nil {
		t.Fatalf("clear fixture destination: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		t.Fatalf("create fixture parent: %v", err)
	}
	if err := os.CopyFS(destination, os.DirFS(source)); err != nil {
		t.Fatalf("materialize fixture %q: %v", name, err)
	}
	return filepath.Join(append([]string{destination}, rootParts...)...)
}

func repoPath(parts ...string) string {
	return filepath.Join(append([]string{".."}, parts...)...)
}
