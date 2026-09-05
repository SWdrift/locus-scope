package pkg

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	locusnpm "locus-scope/internal/npm"
)

func TestOfflineMaterializationUsesIntegrityAddressedCache(t *testing.T) {
	root := pkgTestRoot(t)
	tarball := ordinaryPackageTarball(t, "helper", "1.2.3")
	integrity := locusnpm.IntegrityFor(tarball)
	stem := integrity.Algorithm() + "-" + integrity.Hex()
	cacheDirectory := filepath.Join(root, ".locus", "cache")
	if err := os.MkdirAll(cacheDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDirectory, stem+".tgz"), tarball, 0o644); err != nil {
		t.Fatal(err)
	}
	lock := emptyLock()
	identity := "npm:helper@1.2.3"
	lock.Importers["."] = importer{Dependencies: map[string]lockEdge{"helper": {Specifier: "^1.0.0", Package: identity}}}
	lock.Packages[identity] = lockPackage{
		Registry: "https://registry.example/", Resolved: "https://registry.example/helper/-/helper-1.2.3.tgz",
		Integrity: integrity.String(), Dependencies: map[string]lockEdge{},
	}
	materialized, err := materialize(context.Background(), root, lock, nil, true)
	if err != nil {
		t.Fatalf("materialize from cache: %v", err)
	}
	defer materialized.close()
	if materialized.installed != 1 || materialized.fetched != 0 || materialized.packages[identity].isLocus {
		t.Fatalf("unexpected materialization counters/content: %+v", materialized)
	}
	if err := materialized.publish(); err != nil {
		t.Fatalf("publish store: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".locus", "packages", stem, "package", "package.json")); err != nil {
		t.Fatalf("integrity-addressed package missing: %v", err)
	}
	lockData, err := encodeLock(lock)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "locus.lock"), lockData, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte("not valid json\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	listed, err := List(root)
	if err != nil {
		t.Fatalf("list must remain lock/store-only when package.json is invalid: %v", err)
	}
	if len(listed.Dependencies) != 1 || listed.Dependencies[0].Identity != identity || listed.Dependencies[0].Version != "1.2.3" {
		t.Fatalf("unexpected list result: %+v", listed)
	}
}
func TestExistingStoreMustMatchIntegrityVerifiedTarball(t *testing.T) {
	root := pkgTestRoot(t)
	tarball := ordinaryPackageTarball(t, "helper", "1.2.3")
	integrity := locusnpm.IntegrityFor(tarball)
	stem := integrity.Algorithm() + "-" + integrity.Hex()
	cacheDirectory := filepath.Join(root, ".locus", "cache")
	if err := os.MkdirAll(cacheDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDirectory, stem+".tgz"), tarball, 0o644); err != nil {
		t.Fatal(err)
	}
	lock := emptyLock()
	identity := "npm:helper@1.2.3"
	lock.Importers["."] = importer{Dependencies: map[string]lockEdge{"helper": {Specifier: "^1.0.0", Package: identity}}}
	lock.Packages[identity] = lockPackage{
		Registry: "https://registry.example/", Resolved: "https://registry.example/helper/-/helper-1.2.3.tgz",
		Integrity: integrity.String(), Dependencies: map[string]lockEdge{},
	}
	first, err := materialize(context.Background(), root, lock, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.publish(); err != nil {
		t.Fatal(err)
	}
	first.close()
	packageJSON := filepath.Join(root, ".locus", "packages", stem, "package", "package.json")
	if err := os.WriteFile(packageJSON, []byte(`{"name":"helper","version":"1.2.3","description":"tampered"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := materialize(context.Background(), root, lock, nil, true); err == nil || !strings.Contains(err.Error(), "does not match its integrity-verified tarball") {
		t.Fatalf("tampered package store error = %v", err)
	}
}

func TestOfflineMaterializationRejectsCorruptCache(t *testing.T) {
	root := pkgTestRoot(t)
	tarball := ordinaryPackageTarball(t, "helper", "1.2.3")
	integrity := locusnpm.IntegrityFor(tarball)
	cache := filepath.Join(root, ".locus", "cache", integrity.Algorithm()+"-"+integrity.Hex()+".tgz")
	if err := os.MkdirAll(filepath.Dir(cache), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cache, []byte("corrupt"), 0o644); err != nil {
		t.Fatal(err)
	}
	lock := emptyLock()
	identity := "npm:helper@1.2.3"
	lock.Importers["."] = importer{Dependencies: map[string]lockEdge{"helper": {Specifier: "*", Package: identity}}}
	lock.Packages[identity] = lockPackage{Registry: "https://registry.example/", Resolved: "https://registry.example/helper.tgz", Integrity: integrity.String(), Dependencies: map[string]lockEdge{}}
	if _, err := materialize(context.Background(), root, lock, nil, true); err == nil || !strings.Contains(err.Error(), "integrity mismatch") {
		t.Fatalf("expected integrity mismatch, got %v", err)
	}
}

func ordinaryPackageTarball(t *testing.T, name, version string) []byte {
	t.Helper()
	var output bytes.Buffer
	gzipWriter := gzip.NewWriter(&output)
	gzipWriter.Header.ModTime = time.Unix(0, 0)
	tarWriter := tar.NewWriter(gzipWriter)
	content := []byte(`{"name":"` + name + `","version":"` + version + `","dependencies":{}}`)
	header := &tar.Header{Name: "package/package.json", Mode: 0o644, Size: int64(len(content)), ModTime: time.Unix(0, 0)}
	if err := tarWriter.WriteHeader(header); err != nil {
		t.Fatal(err)
	}
	if _, err := tarWriter.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}
