package scope_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"locus-scope/internal/scope"
)

func TestLoadScopeComposition(t *testing.T) {
	rootDirectory := materializeCase(t, "scope-composition", "app")
	workspace, err := loadLocal(rootDirectory)
	if err != nil {
		t.Fatalf("load scope composition: %v", err)
	}
	if len(workspace.Scopes) != 2 {
		t.Fatalf("loaded scopes = %d, want 2", len(workspace.Scopes))
	}
	if !strings.HasPrefix(string(workspace.Root), "file://") {
		t.Fatalf("root key = %q, want canonical file URI", workspace.Root)
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

func TestDefinitionDiscoveryRecursesHonorsIgnoreAndStopsAtNestedScope(t *testing.T) {
	rootDirectory := materializeCase(t, "discovery")
	generatedDirectory := filepath.Join(rootDirectory, ".locus")
	if err := os.MkdirAll(generatedDirectory, 0o755); err != nil {
		t.Fatalf("create generated state directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(generatedDirectory, "generated.locus.yaml"), []byte("invalid: generated state\n"), 0o644); err != nil {
		t.Fatalf("write generated state definition: %v", err)
	}

	workspace, err := loadLocal(rootDirectory)
	if err != nil {
		t.Fatalf("load recursive discovery fixture: %v", err)
	}
	root := workspace.Scopes[workspace.Root]
	if len(root.Entities) != 2 {
		t.Fatalf("discovered entities = %#v, want root and child", root.Entities)
	}
	for _, id := range []string{"root", "child"} {
		if _, exists := root.Entities[id]; !exists {
			t.Fatalf("discovered entities = %#v, missing %q", root.Entities, id)
		}
	}
}

func TestGroupedRelationKeepsExternalRootReference(t *testing.T) {
	rootDirectory := materializeCase(t, "group-fallback")
	workspace, err := loadLocal(rootDirectory)
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
	workspace, err := loadLocal(rootDirectory)
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
	workspace, err := loadLocal(rootDirectory)
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
	workspace, err := loadLocal(rootDirectory)
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

func TestResolverSourceKeyDefinesIdentityAcrossMaterializations(t *testing.T) {
	base := repoPath("temp", "e2e-run", "source-key")
	if err := os.RemoveAll(base); err != nil {
		t.Fatalf("clear source-key fixture: %v", err)
	}

	const packageKey = scope.ScopeKey("npm:@example/package@1.0.0")
	var resolved [2]scope.EntityKey
	for index, name := range []string{"first", "second"} {
		rootDirectory := filepath.Join(base, name, "root")
		packageDirectory := filepath.Join(base, name, "materialized-package")
		if err := os.MkdirAll(rootDirectory, 0o755); err != nil {
			t.Fatalf("create root fixture: %v", err)
		}
		if err := os.MkdirAll(packageDirectory, 0o755); err != nil {
			t.Fatalf("create package fixture: %v", err)
		}
		if err := os.WriteFile(filepath.Join(rootDirectory, "locus.yaml"), []byte("id: root\nimports:\n  pkg: package\nexports:\n  - pkg:item\n"), 0o644); err != nil {
			t.Fatalf("write root manifest: %v", err)
		}
		if err := os.WriteFile(filepath.Join(packageDirectory, "locus.yaml"), []byte("id: package\nimports:\n  root: root\nexports:\n  - item\n"), 0o644); err != nil {
			t.Fatalf("write package manifest: %v", err)
		}
		if err := os.WriteFile(filepath.Join(packageDirectory, "entities.locus.yaml"), []byte("entities:\n  - id: item\n"), 0o644); err != nil {
			t.Fatalf("write package entities: %v", err)
		}

		rootSource, err := scope.NewLocalSource(rootDirectory)
		if err != nil {
			t.Fatalf("create root source: %v", err)
		}
		packageSource := scope.Source{Key: packageKey, LocalPath: packageDirectory}
		resolver := testResolver{
			rootSource.Key: {"package": packageSource},
			packageKey:     {"root": rootSource},
		}
		workspace, err := scope.Load(rootSource, resolver)
		if err != nil {
			t.Fatalf("load %s materialization: %v", name, err)
		}
		resolved[index] = mustResolve(t, workspace, workspace.Root, "pkg:item")
	}

	if resolved[0] != resolved[1] || resolved[0] != (scope.EntityKey{Scope: packageKey, ID: "item"}) {
		t.Fatalf("materialized entity identities = %#v and %#v", resolved[0], resolved[1])
	}
}

func TestValidationDiagnostics(t *testing.T) {
	tests := []struct {
		name string
		want []string
	}{
		{"duplicate-expanded", []string{"team.locus.yaml", `entity "team/worker"`, "after group expansion"}},
		{"missing-relation", []string{"entities.locus.yaml", `relation "source points_to absent"`, `end reference "absent"`}},
		{"validation/missing-manifest", []string{"manifest missing", "locus.yaml"}},
		{"validation/duplicate-manifest", []string{"multiple manifests", "locus.json", "locus.yaml"}},
		{"validation/missing-scope-id", []string{"locus.yaml", "scope id is required"}},
		{"validation/missing-entity-id", []string{"entities.locus.yaml", "entity 1", "id is required"}},
		{"validation/missing-import", []string{"locus.yaml", `import "absent"`, "does-not-exist"}},
		{"validation/missing-projection", []string{"locus.yaml", `export "ghost:item"`, `does not import projection "ghost"`}},
		{"validation/malformed-relation", []string{"entities.locus.yaml", "line 4", "exactly [from, relation, to]"}},
		{"validation/invalid-locusignore", []string{".locusignore:1", "invalid ignore pattern", "syntax error in pattern"}},
	}

	for _, test := range tests {
		t.Run(strings.ReplaceAll(test.name, "/", "-"), func(t *testing.T) {
			rootDirectory := materializeCase(t, test.name)
			_, err := loadLocal(rootDirectory)
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
	workspace, err := loadLocal(found)
	if err != nil {
		t.Fatalf("load found scope: %v", err)
	}
	if workspace.Scopes[workspace.Root].Manifest.ID != "a" {
		t.Fatalf("found root scope ID = %q", workspace.Scopes[workspace.Root].Manifest.ID)
	}
}

func loadLocal(path string) (*scope.Workspace, error) {
	source, err := scope.NewLocalSource(path)
	if err != nil {
		return nil, err
	}
	return scope.Load(source, scope.LocalResolver{})
}

type testResolver map[scope.ScopeKey]map[string]scope.Source

func (r testResolver) Resolve(from scope.Source, reference string) (scope.Source, error) {
	return r[from.Key][reference], nil
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
	return filepath.Join(append([]string{"..", ".."}, parts...)...)
}
