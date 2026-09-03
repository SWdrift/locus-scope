package packages

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"locus-scope/internal/scope"
)

func TestInstallAndOfflineLoadLocalWorkspace(t *testing.T) {
	root := packageTestRoot(t, "local-install")
	if err := os.WriteFile(filepath.Join(root, "locus.yaml"), []byte("id: local\nexports:\n  - item\n"), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "entities.yaml"), []byte("entities:\n  - id: item\n"), 0o644); err != nil {
		t.Fatalf("write entities: %v", err)
	}
	result, err := Install(context.Background(), root, InstallOptions{CacheRoot: filepath.Join(root, "cache")})
	if err != nil {
		t.Fatalf("install local workspace: %v", err)
	}
	if !result.Valid || result.Scopes != 1 || result.Entities != 1 || result.Resolved != 0 || result.Fetched != 0 || result.Materialized != 0 {
		t.Fatalf("install result = %#v", result)
	}
	lockData, err := os.ReadFile(filepath.Join(root, "locus.lock"))
	if err != nil {
		t.Fatalf("read generated lock: %v", err)
	}
	if !strings.Contains(string(lockData), "version: 1") || !strings.Contains(string(lockData), "packages: {}") {
		t.Fatalf("generated lock = %s", lockData)
	}
	workspace, err := LoadWorkspace(root)
	if err != nil {
		t.Fatalf("load workspace offline: %v", err)
	}
	if len(workspace.Scopes) != 1 {
		t.Fatalf("offline scopes = %d", len(workspace.Scopes))
	}
}

func TestFrozenInstallRejectsStaleLockWithoutWriting(t *testing.T) {
	root := packageTestRoot(t, "frozen-stale")
	if err := os.WriteFile(filepath.Join(root, "locus.yaml"), []byte("id: local\n"), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	lock := []byte("version: 1\npackages:\n  oci://registry.example/team/package:latest:\n    resolved: oci://registry.example/team/package@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\n")
	path := filepath.Join(root, "locus.lock")
	if err := os.WriteFile(path, lock, 0o644); err != nil {
		t.Fatalf("write lock: %v", err)
	}
	_, err := Install(context.Background(), root, InstallOptions{Frozen: true, CacheRoot: filepath.Join(root, "cache")})
	if err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("frozen install error = %v", err)
	}
	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("read preserved lock: %v", readErr)
	}
	if string(got) != string(lock) {
		t.Fatalf("frozen install changed lock:\n%s", got)
	}
}

func TestOfflineLoadRequiresInstallForMissingPackage(t *testing.T) {
	root := packageTestRoot(t, "offline-missing")
	if err := os.WriteFile(filepath.Join(root, "locus.yaml"), []byte("id: local\nimports:\n  package: oci://registry.example/team/package:latest\n"), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	_, err := LoadWorkspace(root)
	if err == nil || !strings.HasSuffix(err.Error(), "run locus-pkg install") {
		t.Fatalf("offline missing-package error = %v", err)
	}
}

func TestPackageRelativeImportCannotEscapeMaterialization(t *testing.T) {
	root := packageTestRoot(t, "relative-escape")
	packageRoot := filepath.Join(root, "package")
	outside := filepath.Join(root, "outside")
	for _, directory := range []string{packageRoot, outside} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatalf("create fixture directory: %v", err)
		}
		if err := os.WriteFile(filepath.Join(directory, "locus.yaml"), []byte("id: fixture\n"), 0o644); err != nil {
			t.Fatalf("write fixture manifest: %v", err)
		}
	}
	reference, err := parsePackageReference("oci://registry.example/team/package@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if err != nil {
		t.Fatalf("parse package reference: %v", err)
	}
	from := scope.Source{Key: scope.ScopeKey(reference.Canonical), LocalPath: packageRoot}
	resolver := newResolverBase(root)
	resolver.packageSources[from.Key] = packageSource{digest: reference, packageRoot: packageRoot, relative: "."}
	if _, err := resolver.resolveLocalOrPackage(from, "../outside"); err == nil || !strings.Contains(err.Error(), "escapes package root") {
		t.Fatalf("package-relative escape error = %v", err)
	}
}
