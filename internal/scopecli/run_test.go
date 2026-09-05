package scopecli_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"locus-scope/internal/scope"
	"locus-scope/internal/scopecli"
)

func TestRunMapsUsageErrorToJSON(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := scopecli.Run(&scope.Workspace{}, []string{"--json", "unknown"}, &stdout, &stderr)
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
