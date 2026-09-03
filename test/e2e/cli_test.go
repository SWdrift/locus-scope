package e2e_test

import (
	"context"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestCLIResolvesProtocolExample(t *testing.T) {
	repositoryRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "go", "run", "./cmd/locus-scope",
		"--scope", "documents/design/protocol/examples/app", "--json", "resolve", "infra:database")
	command.Dir = repositoryRoot
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("run locus-scope: %v\n%s", err, output)
	}

	var result struct {
		Reference string `json:"reference"`
		Entity    struct {
			ScopeID string `json:"scope_id"`
			Scope   string `json:"scope"`
			ID      string `json:"id"`
		} `json:"entity"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatalf("decode CLI JSON %q: %v", output, err)
	}
	if result.Reference != "infra:database" || result.Entity.ScopeID != "infra" || result.Entity.ID != "database" {
		t.Fatalf("unexpected resolve result: %#v", result)
	}
	if filepath.Base(result.Entity.Scope) != "infra" {
		t.Fatalf("resolved owner source = %q, want infra directory", result.Entity.Scope)
	}
}
