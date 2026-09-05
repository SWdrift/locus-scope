package pkg

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestPackResultContract(t *testing.T) {
	root := writePackFixture(t)
	result, err := Pack(root)
	if err != nil {
		t.Fatalf("pack: %v", err)
	}
	if result.Name != "pkg" || result.Version != "1.2.3" || result.Filename != "pkg-1.2.3.tgz" || result.Integrity == "" || len(result.Files) != 2 {
		t.Fatalf("unexpected pack result: %+v", result)
	}
	if _, err := os.Stat(filepath.Join(root, result.Filename)); err != nil {
		t.Fatalf("pack archive not written: %v", err)
	}
}

func TestPublishResultContractAndTemporaryArchiveCleanup(t *testing.T) {
	root := writePackFixture(t)
	t.Setenv("NPM_CONFIG_USERCONFIG", filepath.Join(root, "user.npmrc"))
	t.Setenv("NPM_TOKEN", "")
	var requestBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPut || request.URL.Path != "/pkg" {
			http.NotFound(writer, request)
			return
		}
		requestBody, _ = io.ReadAll(request.Body)
		writer.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()
	result, err := Publish(context.Background(), root, Options{Registry: server.URL + "/"})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if result.Name != "pkg" || result.Version != "1.2.3" || result.Registry != server.URL+"/" || result.Integrity == "" || len(requestBody) == 0 {
		t.Fatalf("unexpected publish result: %+v", result)
	}
	entries, err := os.ReadDir(filepath.Join(root, ".locus", "tmp"))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("publish left temporary operation directories: %v", entries)
	}
}

func writePackFixture(t *testing.T) string {
	t.Helper()
	root := pkgTestRoot(t)
	manifest := `{
  "name": "pkg",
  "version": "1.2.3",
  "files": ["locus.yaml"],
  "locus": {"entry": "locus.yaml"}
}
`
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "locus.yaml"), []byte("id: pkg\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}
