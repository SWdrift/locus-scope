package pkg

import (
	"bytes"
	"crypto/sha512"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testIntegrity(content string) string {
	digest := sha512.Sum512([]byte(content))
	return "sha512-" + base64.StdEncoding.EncodeToString(digest[:])
}
func pkgTestRoot(t *testing.T) string {
	t.Helper()
	base := filepath.Join("..", "..", "temp", "unit-pkg")
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatalf("create Pure package test base: %v", err)
	}
	root, err := os.MkdirTemp(base, "case-")
	if err != nil {
		t.Fatalf("create Pure package test root: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	return root
}

func TestLockStrictCanonicalAndMultiVersionEdges(t *testing.T) {
	root := pkgTestRoot(t)
	lock := lockFile{
		Version: lockVersion,
		Importers: map[string]importer{".": {Dependencies: map[string]lockEdge{
			"@example/app":    {Specifier: "^1.0.0", Package: "npm:@example/app@1.0.0"},
			"@example/modern": {Specifier: "^1.0.0", Package: "npm:@example/modern@1.0.0"},
		}}},
		Packages: map[string]lockPackage{
			"npm:@example/app@1.0.0":    packageRecord("@example/app", testIntegrity("app"), map[string]lockEdge{"@example/base": {Specifier: "^1.0.0", Package: "npm:@example/base@1.1.0"}}),
			"npm:@example/modern@1.0.0": packageRecord("@example/modern", testIntegrity("modern"), map[string]lockEdge{"@example/base": {Specifier: "^2.0.0", Package: "npm:@example/base@2.0.0"}}),
			"npm:@example/base@1.1.0":   packageRecord("@example/base", testIntegrity("base1"), map[string]lockEdge{}),
			"npm:@example/base@2.0.0":   packageRecord("@example/base", testIntegrity("base2"), map[string]lockEdge{}),
		},
	}
	encoded, err := encodeLock(lock)
	if err != nil {
		t.Fatalf("encode lock: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "locus.lock"), encoded, 0o644); err != nil {
		t.Fatal(err)
	}
	decoded, original, err := readLock(root, true)
	if err != nil {
		t.Fatalf("read canonical lock: %v", err)
	}
	if !bytes.Equal(encoded, original) || decoded.Packages["npm:@example/app@1.0.0"].Dependencies["@example/base"].Package != "npm:@example/base@1.1.0" {
		t.Fatal("canonical lock did not preserve importer-relative multiversion edges")
	}

	nonCanonical := append([]byte("\n"), encoded...)
	if err := os.WriteFile(filepath.Join(root, "locus.lock"), nonCanonical, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := readLock(root, true); err == nil || !strings.Contains(err.Error(), "not canonical") {
		t.Fatalf("expected non-canonical lock rejection, got %v", err)
	}

	duplicate := strings.Replace(string(encoded), "version: 1", "version: 1\nversion: 1", 1)
	if err := os.WriteFile(filepath.Join(root, "locus.lock"), []byte(duplicate), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := readLock(root, true); err == nil {
		t.Fatal("expected duplicate YAML field rejection")
	}
}

func TestLockRejectsConflictingOrUnreachableEdges(t *testing.T) {
	lock := emptyLock()
	lock.Packages["npm:a@1.0.0"] = packageRecord("a", testIntegrity("a"), map[string]lockEdge{})
	if err := validateLock(lock); err == nil || !strings.Contains(err.Error(), "unreachable") {
		t.Fatalf("expected unreachable package rejection, got %v", err)
	}
	lock.Importers["."] = importer{Dependencies: map[string]lockEdge{"b": {Specifier: "^1", Package: "npm:a@1.0.0"}}}
	if err := validateLock(lock); err == nil || !strings.Contains(err.Error(), "targets package") {
		t.Fatalf("expected edge/name mismatch rejection, got %v", err)
	}
}

func packageRecord(name, integrity string, dependencies map[string]lockEdge) lockPackage {
	return lockPackage{
		Registry:     "https://registry.example/",
		Resolved:     "https://registry.example/" + name + "/-/package.tgz",
		Integrity:    integrity,
		Dependencies: dependencies,
	}
}
