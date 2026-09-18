package packageenv_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"locus-scope/internal/packageenv"
	"locus-scope/internal/scope"
)

func TestLoadResolvesEveryPackageImportFromItsImporterEdge(t *testing.T) {
	root := testRoot(t, "root")
	writeFile(t, filepath.Join(root, "locus.yaml"), "id: consumer\nimports:\n  app: '@example/app'\n  base: '@example/base'\n")
	app := testRoot(t, "app")
	writeFile(t, filepath.Join(app, "scope", "locus.yaml"), "id: app\nimports:\n  base: '@example/base'\nexports: [service]\n")
	writeFile(t, filepath.Join(app, "scope", "entities.locus.yaml"), "entities:\n  - id: service\n")
	appBase := testRoot(t, "app-base")
	writeFile(t, filepath.Join(appBase, "locus.yaml"), "id: app-base\n")
	rootBase := testRoot(t, "root-base")
	writeFile(t, filepath.Join(rootBase, "locus.yaml"), "id: root-base\n")

	appID := packageenv.Identity("npm:@example/app@1.0.0")
	appBaseID := packageenv.Identity("npm:@example/base@2.0.0")
	rootBaseID := packageenv.Identity("npm:@example/base@3.0.0")
	workspace, err := packageenv.Load(packageenv.Environment{
		RootScope: root,
		RootDependencies: map[string]packageenv.Identity{
			"@example/app":  appID,
			"@example/base": rootBaseID,
		},
		Packages: map[packageenv.Identity]packageenv.PackageDescriptor{
			appID:      {Root: app, Entry: "scope/locus.yaml", Dependencies: map[string]packageenv.Identity{"@example/base": appBaseID}},
			appBaseID:  {Root: appBase, Entry: "locus.yaml"},
			rootBaseID: {Root: rootBase, Entry: "locus.yaml"},
		},
	})
	if err != nil {
		t.Fatalf("load resolved environment: %v", err)
	}
	if got := workspace.Scopes[workspace.Root].Imports["app"]; got != scope.ScopeKey(appID) {
		t.Fatalf("root app owner = %q, want %q", got, appID)
	}
	if got := workspace.Scopes[workspace.Root].Imports["base"]; got != scope.ScopeKey(rootBaseID) {
		t.Fatalf("root base owner = %q, want %q", got, rootBaseID)
	}
	if got := workspace.Scopes[scope.ScopeKey(appID)].Imports["base"]; got != scope.ScopeKey(appBaseID) {
		t.Fatalf("app base owner = %q, want importer-relative %q instead of root %q", got, appBaseID, rootBaseID)
	}
}

func TestLoadRejectsMissingTargetAndEscapingEntry(t *testing.T) {
	root := testRoot(t, "invalid-root")
	writeFile(t, filepath.Join(root, "locus.yaml"), "id: consumer\n")
	_, err := packageenv.Load(packageenv.Environment{RootScope: root, RootDependencies: map[string]packageenv.Identity{"app": "npm:app@1.0.0"}, Packages: map[packageenv.Identity]packageenv.PackageDescriptor{}})
	if err == nil || !strings.Contains(err.Error(), "targets missing package") {
		t.Fatalf("missing target error = %v", err)
	}

	packageRoot := testRoot(t, "escape-package")
	_, err = packageenv.Load(packageenv.Environment{RootScope: root, Packages: map[packageenv.Identity]packageenv.PackageDescriptor{"npm:app@1.0.0": {Root: packageRoot, Entry: "../locus.yaml"}}})
	if err == nil || !strings.Contains(err.Error(), "remain inside") {
		t.Fatalf("entry escape error = %v", err)
	}
}

func testRoot(t *testing.T, name string) string {
	t.Helper()
	root := filepath.Join("..", "..", "temp", "unit-packageenv", name)
	if err := os.RemoveAll(root); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	return absolute
}

func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
