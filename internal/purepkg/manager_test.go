package purepkg

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	locusnpm "locus-scope/internal/npm"
)

func TestNamedUpdatePreservesOtherRootClosure(t *testing.T) {
	integrity := testIntegrity("tarball")
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		base := server.URL + "/"
		versions := map[string]any{}
		switch request.URL.Path {
		case "/a":
			versions["1.0.0"] = registryVersion("a", "1.0.0", map[string]string{"shared": "^1.0.0"}, base+"a-1.0.0.tgz", integrity)
			versions["1.1.0"] = registryVersion("a", "1.1.0", map[string]string{"shared": "^2.0.0"}, base+"a-1.1.0.tgz", integrity)
			_ = json.NewEncoder(writer).Encode(map[string]any{"name": "a", "versions": versions})
		case "/a/1.1.0":
			_ = json.NewEncoder(writer).Encode(registryVersion("a", "1.1.0", map[string]string{"shared": "^2.0.0"}, base+"a-1.1.0.tgz", integrity))
		case "/shared":
			versions["1.5.0"] = registryVersion("shared", "1.5.0", map[string]string{}, base+"shared-1.5.0.tgz", integrity)
			versions["2.1.0"] = registryVersion("shared", "2.1.0", map[string]string{}, base+"shared-2.1.0.tgz", integrity)
			_ = json.NewEncoder(writer).Encode(map[string]any{"name": "shared", "versions": versions})
		case "/shared/2.1.0":
			_ = json.NewEncoder(writer).Encode(registryVersion("shared", "2.1.0", map[string]string{}, base+"shared-2.1.0.tgz", integrity))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	root := purepkgTestRoot(t)
	t.Setenv("NPM_CONFIG_USERCONFIG", filepath.Join(root, "user.npmrc"))
	t.Setenv("NPM_TOKEN", "")
	config, err := locusnpm.LoadConfig(root, server.URL+"/")
	if err != nil {
		t.Fatalf("load Registry config: %v", err)
	}
	client := locusnpm.NewClient(config, server.Client())
	old := emptyLock()
	old.Importers["."] = importer{Dependencies: map[string]lockEdge{
		"a": {Specifier: "^1.0.0", Package: "npm:a@1.0.0"},
		"b": {Specifier: "^1.0.0", Package: "npm:b@1.0.0"},
	}}
	old.Packages["npm:a@1.0.0"] = lockPackage{Registry: server.URL + "/", Resolved: server.URL + "/a-1.0.0.tgz", Integrity: integrity, Dependencies: map[string]lockEdge{"shared": {Specifier: "^1.0.0", Package: "npm:shared@1.5.0"}}}
	old.Packages["npm:b@1.0.0"] = lockPackage{Registry: server.URL + "/", Resolved: server.URL + "/b-1.0.0.tgz", Integrity: integrity, Dependencies: map[string]lockEdge{"shared": {Specifier: "^1.0.0", Package: "npm:shared@1.5.0"}}}
	old.Packages["npm:shared@1.5.0"] = lockPackage{Registry: server.URL + "/", Resolved: server.URL + "/shared-1.5.0.tgz", Integrity: integrity, Dependencies: map[string]lockEdge{}}
	updated, err := resolveLock(context.Background(), map[string]string{"a": "^1.0.0", "b": "^1.0.0"}, map[string]bool{"a": true}, old, client)
	if err != nil {
		t.Fatalf("resolve named update: %v", err)
	}
	if updated.Importers["."].Dependencies["a"].Package != "npm:a@1.1.0" || updated.Importers["."].Dependencies["b"].Package != "npm:b@1.0.0" {
		t.Fatalf("named update changed wrong roots: %+v", updated.Importers["."].Dependencies)
	}
	if _, ok := updated.Packages["npm:shared@1.5.0"]; !ok {
		t.Fatal("unchanged root closure was not preserved")
	}
	if _, ok := updated.Packages["npm:shared@2.1.0"]; !ok {
		t.Fatal("updated root closure did not include its importer-relative version")
	}
}

func TestFrozenAndOfflineUseExactEmptyLockWithoutWrites(t *testing.T) {
	root := purepkgTestRoot(t)
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte("{\n  \"dependencies\": {}\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "locus.yaml"), []byte("id: root\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	lockData, err := encodeLock(emptyLock())
	if err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(root, "locus.lock")
	if err := os.WriteFile(lockPath, lockData, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, options := range []Options{{Offline: true}, {FrozenLockfile: true}} {
		before, _ := os.ReadFile(lockPath)
		result, err := Install(context.Background(), root, nil, options)
		if err != nil {
			t.Fatalf("install with %+v: %v", options, err)
		}
		after, _ := os.ReadFile(lockPath)
		if !result.Valid || !bytes.Equal(before, after) {
			t.Fatalf("install with %+v changed frozen/offline lock", options)
		}
	}
}

func TestCommitFailureRestoresPackageJSON(t *testing.T) {
	root := purepkgTestRoot(t)
	oldManifest := []byte("{\"dependencies\":{}}\n")
	if err := os.WriteFile(filepath.Join(root, "package.json"), oldManifest, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "locus.lock"), 0o755); err != nil {
		t.Fatal(err)
	}
	err := commitProjectFiles(root, []byte("{\"dependencies\":{\"a\":\"^1\"}}\n"), true, []byte("version: 1\n"), oldManifest, nil)
	if err == nil {
		t.Fatal("expected lock commit failure")
	}
	restored, readErr := os.ReadFile(filepath.Join(root, "package.json"))
	if readErr != nil || !bytes.Equal(restored, oldManifest) {
		t.Fatalf("package.json was not restored: %q, %v", restored, readErr)
	}
}

func TestConflictingSemanticIdentityEdgesAreRejected(t *testing.T) {
	packages := map[string]lockPackage{"npm:a@1.0.0": {Registry: "https://one.example/", Resolved: "https://one.example/a.tgz", Integrity: testIntegrity("a"), Dependencies: map[string]lockEdge{}}}
	conflict := packages["npm:a@1.0.0"]
	conflict.Dependencies = map[string]lockEdge{"b": {Specifier: "^1", Package: "npm:b@1.0.0"}}
	if err := mergeLockedPackage("npm:a@1.0.0", conflict, packages); err == nil || !strings.Contains(err.Error(), "conflicting package environment") {
		t.Fatalf("expected identity conflict, got %v", err)
	}
}

func registryVersion(name, version string, dependencies map[string]string, tarball, integrity string) map[string]any {
	return map[string]any{"name": name, "version": version, "dependencies": dependencies, "dist": map[string]string{"tarball": tarball, "integrity": integrity}}
}
