package scope_test

import (
	"testing"

	"locus-scope/internal/scope"
)

func TestFilterStrictNestedAndMetadataSemantics(t *testing.T) {
	object := map[string]any{"id": "api", "network": map[string]any{"region": "tokyo"}, "replicas": float64(3), "labels": []any{"prod"}}
	metadata := map[string]any{"scope": ".", "group": "backend"}
	for _, filters := range [][]string{{"network.region=tokyo"}, {"id~=^a.*"}, {"@scope=.", "@group=backend"}, {"replicas=3"}} {
		predicates, err := scope.ParseFilters(filters)
		if err != nil {
			t.Fatal(err)
		}
		if !scope.Match(object, metadata, predicates) {
			t.Fatalf("filters %v did not match", filters)
		}
	}
	for _, filters := range [][]string{{"missing!=value"}, {"replicas*=three"}, {"labels*=prod"}, {"@scope=dependency"}} {
		predicates, err := scope.ParseFilters(filters)
		if err != nil {
			t.Fatal(err)
		}
		if scope.Match(object, metadata, predicates) {
			t.Fatalf("filters %v unexpectedly matched", filters)
		}
	}
	if _, err := scope.ParseFilters([]string{"id~=["}); err == nil {
		t.Fatal("invalid regexp was accepted")
	}
}
