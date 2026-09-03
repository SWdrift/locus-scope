package packages

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/oci"
)

func TestValidateAndMaterializePackage(t *testing.T) {
	root := packageTestRoot(t, "materialize")
	store, manifest := testPackageArtifact(t, filepath.Join(root, "store"), ArtifactType, []archiveEntry{
		{name: "locus.yaml", body: "id: package\nexports:\n  - tool\n"},
		{name: "bin/tool", body: "#!/bin/sh\n", mode: 0o755},
		{name: "entities.yaml", body: "entities:\n  - id: tool\n"},
	})
	cached, err := validateCachedPackage(context.Background(), store, manifest)
	if err != nil {
		t.Fatalf("validate cached package: %v", err)
	}
	cached.store = store
	reference, err := parsePackageReference("oci://registry.example/team/package@" + manifest.Digest.String())
	if err != nil {
		t.Fatalf("parse digest reference: %v", err)
	}
	project := filepath.Join(root, "project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatalf("create project: %v", err)
	}
	target, created, err := materializePackage(context.Background(), project, reference, cached)
	if err != nil {
		t.Fatalf("materialize package: %v", err)
	}
	if !created {
		t.Fatal("first materialization was not reported as created")
	}
	if info, err := os.Stat(filepath.Join(target, "bin", "tool")); err != nil {
		t.Fatalf("inspect materialized executable: %v", err)
	} else if runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("materialized executable mode = %v", info.Mode())
	}
	if _, created, err := materializePackage(context.Background(), project, reference, cached); err != nil || created {
		t.Fatalf("reuse result created=%v error=%v", created, err)
	}
}

func TestArchiveExtractionRejectsUnsafeEntries(t *testing.T) {
	tests := []struct {
		name    string
		entries []archiveEntry
		want    string
	}{
		{name: "parent traversal", entries: []archiveEntry{{name: "../outside", body: "bad"}}, want: "safe relative"},
		{name: "backslash", entries: []archiveEntry{{name: `dir\\file`, body: "bad"}}, want: "backslash"},
		{name: "duplicate", entries: []archiveEntry{{name: "same", body: "one"}, {name: "./same", body: "two"}}, want: "duplicate"},
		{name: "symlink", entries: []archiveEntry{{name: "link", typeflag: tar.TypeSymlink}}, want: "unsupported type"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := packageTestRoot(t, "unsafe-"+strings.ReplaceAll(test.name, " ", "-"))
			store, err := oci.New(filepath.Join(root, "store"))
			if err != nil {
				t.Fatalf("create store: %v", err)
			}
			layer := pushTestLayer(t, store, test.entries)
			destination := filepath.Join(root, "destination")
			if err := os.MkdirAll(destination, 0o755); err != nil {
				t.Fatalf("create destination: %v", err)
			}
			if err := extractPackageLayer(context.Background(), store, layer, destination); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("extract error = %v, want fragment %q", err, test.want)
			}
		})
	}
}

func TestInvalidArtifactAndMaterializationAreNotPublished(t *testing.T) {
	root := packageTestRoot(t, "invalid-artifact")
	store, manifest := testPackageArtifact(t, filepath.Join(root, "store"), "application/example", []archiveEntry{
		{name: "readme.txt", body: "not a scope"},
	})
	if _, err := validateCachedPackage(context.Background(), store, manifest); err == nil || !strings.Contains(err.Error(), "artifact type") {
		t.Fatalf("artifact validation error = %v", err)
	}

	store, manifest = testPackageArtifact(t, filepath.Join(root, "invalid-store"), ArtifactType, []archiveEntry{
		{name: "readme.txt", body: "not a scope"},
	})
	cached, err := validateCachedPackage(context.Background(), store, manifest)
	if err != nil {
		t.Fatalf("validate package envelope: %v", err)
	}
	cached.store = store
	reference, err := parsePackageReference("oci://registry.example/team/package@" + manifest.Digest.String())
	if err != nil {
		t.Fatalf("parse reference: %v", err)
	}
	project := filepath.Join(root, "project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatalf("create project: %v", err)
	}
	target, pathErr := packageDirectory(project, reference)
	if pathErr != nil {
		t.Fatalf("package path: %v", pathErr)
	}
	if _, _, err := materializePackage(context.Background(), project, reference, cached); err == nil {
		t.Fatal("invalid scope package materialized successfully")
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("invalid package target was published: %v", err)
	}
}

func TestValidateCachedPackageRejectsNonEmptyConfig(t *testing.T) {
	root := packageTestRoot(t, "invalid-config")
	store, err := oci.New(filepath.Join(root, "store"))
	if err != nil {
		t.Fatalf("create OCI store: %v", err)
	}
	layer := pushTestLayer(t, store, []archiveEntry{{name: "locus.yaml", body: "id: package\n"}})
	config, err := oras.PushBytes(context.Background(), store, "application/vnd.example.config.v1+json", []byte("{}"))
	if err != nil {
		t.Fatalf("push config: %v", err)
	}
	manifest, err := oras.PackManifest(context.Background(), store, oras.PackManifestVersion1_1, ArtifactType, oras.PackManifestOptions{
		ConfigDescriptor: &config,
		Layers:           []ocispec.Descriptor{layer},
	})
	if err != nil {
		t.Fatalf("pack manifest: %v", err)
	}
	if _, err := validateCachedPackage(context.Background(), store, manifest); err == nil || !strings.Contains(err.Error(), "empty JSON descriptor") {
		t.Fatalf("artifact validation error = %v", err)
	}
}

func TestCachePathEncodingPreservesRepositorySegments(t *testing.T) {
	reference, err := parsePackageReference("oci://localhost:18080/team/package:latest")
	if err != nil {
		t.Fatalf("parse reference: %v", err)
	}
	got := cacheRepositoryPath("cache", reference)
	want := filepath.Join("cache", "r-localhost%3A18080", "p-team", "p-package")
	if got != want {
		t.Fatalf("cache path = %q, want %q", got, want)
	}
	if got := encodeCacheSegment("p-", "CON."); got != "p-CON%2E" {
		t.Fatalf("reserved-name encoding = %q", got)
	}
}

type archiveEntry struct {
	name     string
	body     string
	mode     int64
	typeflag byte
}

func testPackageArtifact(t *testing.T, storePath, artifactType string, entries []archiveEntry) (*oci.Store, ocispec.Descriptor) {
	t.Helper()
	store, err := oci.New(storePath)
	if err != nil {
		t.Fatalf("create OCI store: %v", err)
	}
	layer := pushTestLayer(t, store, entries)
	manifest, err := oras.PackManifest(context.Background(), store, oras.PackManifestVersion1_1, artifactType, oras.PackManifestOptions{
		Layers: []ocispec.Descriptor{layer},
	})
	if err != nil {
		t.Fatalf("pack manifest: %v", err)
	}
	return store, manifest
}

func pushTestLayer(t *testing.T, store *oci.Store, entries []archiveEntry) ocispec.Descriptor {
	t.Helper()
	var payload bytes.Buffer
	gzipWriter := gzip.NewWriter(&payload)
	tarWriter := tar.NewWriter(gzipWriter)
	for _, entry := range entries {
		mode := entry.mode
		if mode == 0 {
			mode = 0o644
		}
		typeflag := entry.typeflag
		if typeflag == 0 {
			typeflag = tar.TypeReg
		}
		header := &tar.Header{Name: entry.name, Mode: mode, Typeflag: typeflag}
		if typeflag == tar.TypeReg {
			header.Size = int64(len(entry.body))
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			t.Fatalf("write tar header: %v", err)
		}
		if entry.body != "" {
			if _, err := tarWriter.Write([]byte(entry.body)); err != nil {
				t.Fatalf("write tar body: %v", err)
			}
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	layer, err := oras.PushBytes(context.Background(), store, ocispec.MediaTypeImageLayerGzip, payload.Bytes())
	if err != nil {
		t.Fatalf("push layer: %v", err)
	}
	return layer
}
