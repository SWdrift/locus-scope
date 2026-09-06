package scopeapp_test

import (
	"reflect"
	"testing"

	"locus-scope/internal/scope"
	"locus-scope/internal/scopeapp"
)

func testWorkspace() *scope.Workspace {
	rootKey := scope.ScopeKey("file:///root")
	return &scope.Workspace{
		Root: rootKey,
		Scopes: map[scope.ScopeKey]*scope.Scope{
			rootKey: {
				Key: rootKey, Manifest: scope.Manifest{ID: "app", Imports: map[string]string{}, Exports: []string{"api", "db"}},
				Imports: map[string]scope.ScopeKey{}, ManifestFile: "locus.yaml",
				Entities: map[string]scope.Entity{
					"api": {ID: "api", LocalID: "api", Properties: map[string]any{"type": "service", "network": map[string]any{"region": "tokyo"}}, Source: scope.Provenance{Scope: rootKey, File: "services.locus.yaml", Line: 3, Index: 1}},
					"db":  {ID: "db", LocalID: "db", Properties: map[string]any{"type": "database"}, Source: scope.Provenance{Scope: rootKey, File: "resources.locus.yaml", Index: 1}},
				},
			},
		},
		Relations: []scope.Relation{{From: scope.EntityKey{Scope: rootKey, ID: "api"}, Type: "depends_on", To: scope.EntityKey{Scope: rootKey, ID: "db"}, FromRef: "api", ToRef: "db", Properties: map[string]any{"critical": true}, Source: scope.Provenance{Scope: rootKey, File: "services.locus.yaml", Index: 1}}},
	}
}

func TestUnifiedQueriesAndProvenance(t *testing.T) {
	app := scopeapp.New(testWorkspace())
	predicates, err := scope.ParseFilters([]string{"network.region=tokyo", "@scope=."})
	if err != nil {
		t.Fatal(err)
	}
	entities := app.QueryEntities(predicates, true)
	if len(entities) != 1 || entities[0].Ref != "api" || entities[0].Object["type"] != "service" || entities[0].Source.File != "services.locus.yaml" {
		t.Fatalf("entities = %#v", entities)
	}
	relations := app.QueryRelations(nil, true)
	if len(relations) != 1 || relations[0].Object["critical"] != true || relations[0].ToKey.ID != "db" || relations[0].Source.ScopeID != "app" {
		t.Fatalf("relations = %#v", relations)
	}
	dependency, err := app.GetEntity("db", false)
	if err != nil {
		t.Fatal(err)
	}
	if dependency.Key.Scope != "file:///root" || dependency.Ref != "db" {
		t.Fatalf("dependency = %#v", dependency)
	}
}

func TestGraphPathAndImpact(t *testing.T) {
	app := scopeapp.New(testWorkspace())
	api, _ := app.GetEntity("api", false)
	database, _ := app.GetEntity("db", false)
	via, _ := scope.ParseFilters([]string{"type=depends_on"})
	graph, err := app.Graph([]scope.EntityKey{api.Key}, 1, via, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(graph.Nodes) != 2 || len(graph.Relations) != 1 {
		t.Fatalf("graph = %#v", graph)
	}
	path, err := app.Path(api.Key, database.Key, via, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := []string{path.Nodes[0].Key.ID, path.Nodes[1].Key.ID}; !reflect.DeepEqual(got, []string{"api", "db"}) {
		t.Fatalf("path = %#v", path)
	}
	impact, err := app.Impact([]scope.EntityKey{database.Key}, via, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(impact.Affected) != 1 || impact.Affected[0].Key.ID != "api" {
		t.Fatalf("impact = %#v", impact)
	}
}
