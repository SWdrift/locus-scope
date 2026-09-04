package packages

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"locus-scope/internal/scope"
)

func TestBuildPackageLayerIsDeterministicAndExcludesGeneratedState(t *testing.T) {
	root := filepath.Join(packageTestRoot(t, "publish-layer"), "source")
	for _, directory := range []string{root, filepath.Join(root, "sub"), filepath.Join(root, ".locus", "packages")} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatalf("create source directory: %v", err)
		}
	}
	files := map[string]string{
		"locus.yaml":                 "id: package\n",
		"entities.locus.yaml":        "entities:\n  - id: service\n",
		filepath.Join("sub", "data"): "payload\n",
		"locus.lock":                 "version: 1\npackages: {}\n",
		filepath.Join(".locus", "packages", "ignored"): "generated\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o644); err != nil {
			t.Fatalf("write source file %s: %v", name, err)
		}
	}

	firstFile, first, err := buildPackageLayer(root)
	if err != nil {
		t.Fatalf("build first package layer: %v", err)
	}
	firstNames := readPackageLayerNames(t, firstFile)
	firstPath := firstFile.Name()
	if err := firstFile.Close(); err != nil {
		t.Fatalf("close first package layer: %v", err)
	}
	if err := os.Remove(firstPath); err != nil {
		t.Fatalf("remove first package layer: %v", err)
	}

	changedTime := time.Now().Add(24 * time.Hour)
	if err := os.Chtimes(filepath.Join(root, "entities.locus.yaml"), changedTime, changedTime); err != nil {
		t.Fatalf("change source timestamp: %v", err)
	}
	secondFile, second, err := buildPackageLayer(root)
	if err != nil {
		t.Fatalf("build second package layer: %v", err)
	}
	secondPath := secondFile.Name()
	if err := secondFile.Close(); err != nil {
		t.Fatalf("close second package layer: %v", err)
	}
	if err := os.Remove(secondPath); err != nil {
		t.Fatalf("remove second package layer: %v", err)
	}

	if first.Digest != second.Digest || first.Size != second.Size {
		t.Fatalf("package layer changed with source timestamps: first=%s/%d second=%s/%d", first.Digest, first.Size, second.Digest, second.Size)
	}
	wantNames := []string{"entities.locus.yaml", "locus.yaml", "sub/", "sub/data"}
	if !reflect.DeepEqual(firstNames, wantNames) {
		t.Fatalf("package entries = %#v, want %#v", firstNames, wantNames)
	}
}

func TestValidatePackageSourceTreeRejectsNonPortableAndEscapingImports(t *testing.T) {
	tests := []struct {
		name      string
		importRef string
		want      string
	}{
		{name: "backslash", importRef: `sub\\scope`, want: "portable relative path"},
		{name: "escape", importRef: "../outside", want: "escapes package root"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			base := packageTestRoot(t, "publish-import-"+test.name)
			root := filepath.Join(base, "source")
			outside := filepath.Join(base, "outside")
			for _, directory := range []string{root, outside} {
				if err := os.MkdirAll(directory, 0o755); err != nil {
					t.Fatalf("create scope directory: %v", err)
				}
			}
			manifest := "id: package\nimports:\n  dependency: '" + test.importRef + "'\n"
			if err := os.WriteFile(filepath.Join(root, "locus.yaml"), []byte(manifest), 0o644); err != nil {
				t.Fatalf("write package manifest: %v", err)
			}
			if err := os.WriteFile(filepath.Join(outside, "locus.yaml"), []byte("id: outside\n"), 0o644); err != nil {
				t.Fatalf("write outside manifest: %v", err)
			}
			source, err := scope.NewLocalSource(root)
			if err != nil {
				t.Fatalf("create package source: %v", err)
			}
			if err := validatePackageSourceTree(source); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validation error = %v, want fragment %q", err, test.want)
			}
		})
	}
}

func TestPublishRejectsDigestTarget(t *testing.T) {
	root := packageTestRoot(t, "publish-digest-target")
	if err := os.WriteFile(filepath.Join(root, "locus.yaml"), []byte("id: package\n"), 0o644); err != nil {
		t.Fatalf("write package manifest: %v", err)
	}
	target := "oci://registry.example/locus/package@sha256:" + strings.Repeat("a", 64)
	if _, err := Publish(context.Background(), root, target, PublishOptions{}); err == nil || !strings.Contains(err.Error(), "must use a tag") {
		t.Fatalf("publish error = %v", err)
	}
}

func readPackageLayerNames(t *testing.T, file *os.File) []string {
	t.Helper()
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		t.Fatalf("rewind package layer: %v", err)
	}
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		t.Fatalf("open package gzip: %v", err)
	}
	defer gzipReader.Close()
	tarReader := tar.NewReader(gzipReader)
	var names []string
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			return names
		}
		if err != nil {
			t.Fatalf("read package tar: %v", err)
		}
		names = append(names, header.Name)
	}
}
