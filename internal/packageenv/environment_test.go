package packageenv_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"locus-scope/internal/packageenv"
	"locus-scope/internal/scope"
)

func TestLoadUsesImporterRelativePackageEdgesAndSemanticOwnership(t *testing.T) {
	root := packageTestRoot(t)
	writeFile(t, filepath.Join(root, "locus.yaml"), "id: consumer\nimports:\n  local: ./local\n  app: '@example/app'\n  modern: '@example/modern'\n")
	writeFile(t, filepath.Join(root, "local", "locus.yaml"), "id: local\n")

	app := packageScope(t, "app", "imports:\n  base: '@example/base'\n")
	modern := packageScope(t, "modern", "imports:\n  base: '@example/base'\n")
	baseOne := packageScope(t, "base-one", "")
	baseTwo := packageScope(t, "base-two", "")

	appID := packageenv.Identity("npm:@example/app@1.0.0")
	modernID := packageenv.Identity("npm:@example/modern@1.0.0")
	baseOneID := packageenv.Identity("npm:@example/base@1.1.0")
	baseTwoID := packageenv.Identity("npm:@example/base@2.0.0")
	workspace, err := packageenv.Load(root, packageenv.Environment{
		RootDependencies: map[string]packageenv.Identity{
			"@example/app":    appID,
			"@example/modern": modernID,
		},
		Packages: map[packageenv.Identity]packageenv.Package{
			appID:     {Identity: appID, Root: app, Entry: "scope/locus.yaml", Dependencies: map[string]packageenv.Identity{"@example/base": baseOneID}},
			modernID:  {Identity: modernID, Root: modern, Entry: "scope/locus.yaml", Dependencies: map[string]packageenv.Identity{"@example/base": baseTwoID}},
			baseOneID: {Identity: baseOneID, Root: baseOne, Entry: "scope/locus.yaml"},
			baseTwoID: {Identity: baseTwoID, Root: baseTwo, Entry: "scope/locus.yaml"},
		},
	})
	if err != nil {
		t.Fatalf("load package environment: %v", err)
	}

	rootScope := workspace.Scopes[workspace.Root]
	if got := rootScope.Imports["app"]; got != scope.ScopeKey(appID) {
		t.Fatalf("root app owner = %q, want %q", got, appID)
	}
	if got := rootScope.Imports["modern"]; got != scope.ScopeKey(modernID) {
		t.Fatalf("root modern owner = %q, want %q", got, modernID)
	}
	if got := workspace.Scopes[scope.ScopeKey(appID)].Imports["base"]; got != scope.ScopeKey(baseOneID) {
		t.Fatalf("app base owner = %q, want %q", got, baseOneID)
	}
	if got := workspace.Scopes[scope.ScopeKey(modernID)].Imports["base"]; got != scope.ScopeKey(baseTwoID) {
		t.Fatalf("modern base owner = %q, want %q", got, baseTwoID)
	}
	localKey := rootScope.Imports["local"]
	if !strings.HasPrefix(string(localKey), "file://") {
		t.Fatalf("local import owner = %q, want file URI", localKey)
	}
	if len(workspace.Scopes) != 6 {
		t.Fatalf("loaded scopes = %d, want 6", len(workspace.Scopes))
	}
}

func TestLoadRejectsLocalImportFromDistributedPackage(t *testing.T) {
	root := packageTestRoot(t)
	writeFile(t, filepath.Join(root, "locus.yaml"), "id: consumer\nimports:\n  app: '@example/app'\n")
	app := packageScope(t, "app", "imports:\n  nested: ./nested\n")
	writeFile(t, filepath.Join(app, "scope", "nested", "locus.yaml"), "id: nested\n")
	appID := packageenv.Identity("npm:@example/app@1.0.0")
	_, err := packageenv.Load(root, packageenv.Environment{
		RootDependencies: map[string]packageenv.Identity{"@example/app": appID},
		Packages: map[packageenv.Identity]packageenv.Package{
			appID: {Identity: appID, Root: app, Entry: "scope/locus.yaml"},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "cannot import local Scope") {
		t.Fatalf("local package import error = %v", err)
	}
}

func TestLoadReportsEnvironmentSpecificMissingRootEdge(t *testing.T) {
	root := packageTestRoot(t)
	writeFile(t, filepath.Join(root, "locus.yaml"), "id: consumer\nimports:\n  app: '@example/app'\n")
	for _, test := range []struct {
		name        string
		mode        packageenv.EnvironmentMode
		want        string
		notExpected string
	}{
		{name: "pure", mode: packageenv.PureEnvironment, want: "run locus-pkg install", notExpected: "pnpm add"},
		{name: "npm", mode: packageenv.NPMEnvironment, want: "run pnpm add @example/app or npm install @example/app", notExpected: "locus-pkg"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := packageenv.Load(root, packageenv.Environment{Mode: test.mode})
			if err == nil || !strings.Contains(err.Error(), test.want) || strings.Contains(err.Error(), test.notExpected) {
				t.Fatalf("missing root edge error = %v", err)
			}
		})
	}
}

func TestLoadRejectsVersionedAndSubpathPackageImports(t *testing.T) {
	for _, reference := range []string{"app@1.0.0", "@example/app/subpath"} {
		t.Run(reference, func(t *testing.T) {
			root := packageTestRoot(t)
			writeFile(t, filepath.Join(root, "locus.yaml"), "id: consumer\nimports:\n  invalid: '"+reference+"'\n")
			_, err := packageenv.Load(root, packageenv.Environment{})
			if err == nil || !strings.Contains(err.Error(), "without versions or subpaths") {
				t.Fatalf("invalid package import error = %v", err)
			}
		})
	}
}

func packageScope(t *testing.T, id, declarations string) string {
	t.Helper()
	root := packageTestRoot(t)
	writeFile(t, filepath.Join(root, "scope", "locus.yaml"), "id: "+id+"\n"+declarations)
	return root
}

func packageTestRoot(t *testing.T) string {
	t.Helper()
	base := filepath.Join("..", "..", "temp", "unit-packageenv")
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatalf("create package environment test base: %v", err)
	}
	root, err := os.MkdirTemp(base, "case-")
	if err != nil {
		t.Fatalf("create package environment test root: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	return root
}

func writeFile(t *testing.T, path string, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create %s parent: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
