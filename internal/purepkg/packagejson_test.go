package purepkg

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectManifestMutationPreservesOtherFields(t *testing.T) {
	root := purepkgTestRoot(t)
	original := []byte("{\n  \"name\": \"consumer\",\n  \"private\": true,\n  \"scripts\": {\"check\": \"echo ok\"},\n  \"dependencies\": {\"z\": \"^1.0.0\"}\n}\n")
	if err := os.WriteFile(filepath.Join(root, "package.json"), original, 0o644); err != nil {
		t.Fatal(err)
	}
	manifest, _, err := readProjectManifest(root)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	mutated := manifest.withDependencies(map[string]string{"a": "^2.0.0", "z": "^1.0.0"})
	encoded, err := encodeProjectManifest(mutated)
	if err != nil {
		t.Fatalf("encode manifest: %v", err)
	}
	if !bytes.HasSuffix(encoded, []byte("\n")) || !bytes.Contains(encoded, []byte("\"private\": true")) || !bytes.Contains(encoded, []byte("\"check\": \"echo ok\"")) {
		t.Fatalf("mutation did not preserve fields and canonical newline:\n%s", encoded)
	}
	if strings.Index(string(encoded), "\"a\"") > strings.Index(string(encoded), "\"z\"") {
		t.Fatalf("dependencies are not lexically ordered:\n%s", encoded)
	}
}

func TestProjectManifestRejectsDuplicateAndUnsupportedDependencies(t *testing.T) {
	root := purepkgTestRoot(t)
	path := filepath.Join(root, "package.json")
	if err := os.WriteFile(path, []byte(`{"dependencies":{},"dependencies":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := readProjectManifest(root); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("expected duplicate field rejection, got %v", err)
	}
	if err := os.WriteFile(path, []byte(`{"dependencies":{"a":"file:../a"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := readProjectManifest(root); err == nil || !strings.Contains(err.Error(), "unsupported specifier") {
		t.Fatalf("expected unsupported dependency rejection, got %v", err)
	}
}
