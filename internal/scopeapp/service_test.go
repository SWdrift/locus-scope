package scopeapp_test

import (
	"reflect"
	"testing"

	"locus-scope/internal/scope"
	"locus-scope/internal/scopeapp"
)

func TestServiceReturnsDeterministicSemanticViews(t *testing.T) {
	rootKey := scope.ScopeKey("file:///consumer")
	packageKey := scope.ScopeKey("npm:@example/base@1.0.0")
	workspace := &scope.Workspace{
		Root: rootKey,
		Scopes: map[scope.ScopeKey]*scope.Scope{
			packageKey: {
				Key: packageKey, Manifest: scope.Manifest{ID: "base", Imports: map[string]string{}, Exports: []string{"z", "database"}},
				Entities: map[string]scope.Entity{"z": {ID: "z"}, "database": {ID: "database", Properties: map[string]any{"engine": "postgres"}}}, Imports: map[string]scope.ScopeKey{},
			},
			rootKey: {
				Key: rootKey, Manifest: scope.Manifest{ID: "consumer", Imports: map[string]string{"base": "@example/base"}, Exports: []string{"root"}},
				Entities: map[string]scope.Entity{"root": {ID: "root", Properties: map[string]any{"kind": "consumer"}}}, Imports: map[string]scope.ScopeKey{"base": packageKey},
			},
		},
		Relations: []scope.Relation{{
			From: scope.EntityKey{Scope: rootKey, ID: "root"}, Name: "uses", To: scope.EntityKey{Scope: packageKey, ID: "database"},
		}},
	}

	app := scopeapp.New(workspace)
	scopes := app.ListScopes()
	if got := []scope.ScopeKey{scopes.Scopes[0].Source, scopes.Scopes[1].Source}; !reflect.DeepEqual(got, []scope.ScopeKey{rootKey, packageKey}) {
		t.Fatalf("Scope order = %v", got)
	}
	if got := scopes.Scopes[1].Exports; !reflect.DeepEqual(got, []string{"database", "z"}) {
		t.Fatalf("export order = %v", got)
	}
	entities := app.ListEntities()
	wantEntities := []scopeapp.EntityKey{
		{ScopeID: "consumer", Scope: rootKey, ID: "root"},
		{ScopeID: "base", Scope: packageKey, ID: "database"},
		{ScopeID: "base", Scope: packageKey, ID: "z"},
	}
	if !reflect.DeepEqual(entities.Entities, wantEntities) {
		t.Fatalf("entities = %#v, want %#v", entities.Entities, wantEntities)
	}
	entity, err := app.GetEntity("root")
	if err != nil {
		t.Fatal(err)
	}
	if entity.Entity.ScopeID != "consumer" || entity.Entity.Scope != rootKey || entity.Entity.Properties["kind"] != "consumer" {
		t.Fatalf("entity = %#v", entity)
	}
	relations := app.ListRelations()
	if len(relations.Relations) != 1 || relations.Relations[0].From.ScopeID != "consumer" || relations.Relations[0].To.ScopeID != "base" {
		t.Fatalf("relations = %#v", relations.Relations)
	}
}
