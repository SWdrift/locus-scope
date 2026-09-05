package npm

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestPackIsDeterministicAndUsesReducedPacklist(t *testing.T) {
	root := npmTestRoot(t)
	for _, directory := range []string{"src", ".locus", "node_modules"} {
		if err := os.MkdirAll(filepath.Join(root, directory), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		"package.json":            `{"name":"@example/app","version":"1.2.3","files":["locus.yaml","src"],"locus":{"entry":"locus.yaml"}}`,
		"locus.yaml":              "id: app\n",
		"src/data.txt":            "data\n",
		"README.md":               "read me\n",
		"unlisted.txt":            "exclude\n",
		"locus.lock":              "exclude\n",
		"old.tgz":                 "exclude\n",
		".locus/state":            "exclude\n",
		"node_modules/dependency": "exclude\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(name)), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	first, err := Pack(root)
	if err != nil {
		t.Fatal(err)
	}
	changed := time.Now().Add(48 * time.Hour)
	if err := os.Chtimes(filepath.Join(root, "src", "data.txt"), changed, changed); err != nil {
		t.Fatal(err)
	}
	second, err := Pack(root)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.Tarball, second.Tarball) || first.Integrity != second.Integrity {
		t.Fatal("tarball changed with source timestamps")
	}
	wantFiles := []string{"README.md", "locus.yaml", "package.json", "src/data.txt"}
	if !reflect.DeepEqual(first.Files, wantFiles) {
		t.Fatalf("packed files = %#v, want %#v", first.Files, wantFiles)
	}
	if first.Filename != "example-app-1.2.3.tgz" {
		t.Fatalf("filename = %q", first.Filename)
	}
	if got := packedTarNames(t, first.Tarball); !reflect.DeepEqual(got, []string{"package/", "package/README.md", "package/locus.yaml", "package/package.json", "package/src/", "package/src/data.txt"}) {
		t.Fatalf("tar entries = %#v", got)
	}
}

func TestPackRejectsUnsupportedSelectionFeatures(t *testing.T) {
	tests := []struct {
		name        string
		packageJSON string
		extra       map[string]string
	}{
		{name: "glob", packageJSON: `{"name":"app","version":"1.0.0","files":["*.yaml"],"locus":{"entry":"locus.yaml"}}`},
		{name: "lifecycle", packageJSON: `{"name":"app","version":"1.0.0","files":["locus.yaml"],"locus":{"entry":"locus.yaml"},"scripts":{"prepack":"generate"}}`},
		{name: "bundled", packageJSON: `{"name":"app","version":"1.0.0","files":["locus.yaml"],"locus":{"entry":"locus.yaml"},"bundledDependencies":[]}`},
		{name: "npmignore", packageJSON: `{"name":"app","version":"1.0.0","files":["locus.yaml"],"locus":{"entry":"locus.yaml"}}`, extra: map[string]string{".npmignore": "locus.lock\n"}},
		{name: "duplicate-scope", packageJSON: `{"name":"app","version":"1.0.0","files":["locus.yaml","nested/locus.json"],"locus":{"entry":"locus.yaml"}}`, extra: map[string]string{"nested/locus.json": `{}`}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := npmTestRoot(t)
			if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(test.packageJSON), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "locus.yaml"), []byte("id: app\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			for name, content := range test.extra {
				filename := filepath.Join(root, filepath.FromSlash(name))
				if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filename, []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := Pack(root); err == nil {
				t.Fatal("unsupported pack feature accepted")
			}
		})
	}
}

func TestBuildPublishBodyContainsNPMIntegrityAndAttachment(t *testing.T) {
	pack := testPackedPackage(t)
	registry, err := url.Parse("http://127.0.0.1:4873/")
	if err != nil {
		t.Fatal(err)
	}
	body, err := BuildPublishBody(pack, registry)
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		DistTags map[string]string `json:"dist-tags"`
		Versions map[string]struct {
			Dist Distribution `json:"dist"`
		} `json:"versions"`
		Attachments map[string]struct {
			Data   string `json:"data"`
			Length int    `json:"length"`
		} `json:"_attachments"`
	}
	if err := json.Unmarshal(body, &document); err != nil {
		t.Fatal(err)
	}
	if document.DistTags["latest"] != pack.Version {
		t.Fatalf("latest = %q", document.DistTags["latest"])
	}
	dist := document.Versions[pack.Version].Dist
	if dist.Integrity != pack.Integrity || len(dist.Shasum) != 40 || !strings.HasSuffix(dist.Tarball, "/app/-/app-1.0.0.tgz") {
		t.Fatalf("dist = %#v", dist)
	}
	attachment := document.Attachments[pack.Filename]
	decoded, err := base64.StdEncoding.DecodeString(attachment.Data)
	if err != nil || !bytes.Equal(decoded, pack.Tarball) || attachment.Length != len(pack.Tarball) {
		t.Fatalf("attachment length/data = %d/%q, %v", attachment.Length, decoded, err)
	}
}

func TestPublishReportsImmutableVersionConflict(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPut || request.URL.Path != "/app" || request.Header.Get("Content-Type") != "application/json" {
			t.Errorf("publish request = %s %s %s", request.Method, request.URL.Path, request.Header.Get("Content-Type"))
		}
		writer.WriteHeader(http.StatusConflict)
		_, _ = writer.Write([]byte("body must not enter diagnostics"))
	}))
	defer server.Close()
	config, err := LoadConfigWithOptions(ConfigOptions{Registry: server.URL, SkipUserConfig: true})
	if err != nil {
		t.Fatal(err)
	}
	err = NewClient(config, server.Client()).Publish(context.Background(), testPackedPackage(t))
	if !errors.Is(err, ErrImmutableVersion) || strings.Contains(err.Error(), "body must not") {
		t.Fatalf("publish error = %v", err)
	}
}

func testPackedPackage(t *testing.T) PackedPackage {
	t.Helper()
	root := npmTestRoot(t)
	packageJSON := `{"name":"app","version":"1.0.0","files":["locus.yaml"],"locus":{"entry":"locus.yaml"}}`
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(packageJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "locus.yaml"), []byte("id: app\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	pack, err := Pack(root)
	if err != nil {
		t.Fatal(err)
	}
	return pack
}

func packedTarNames(t *testing.T, content []byte) []string {
	t.Helper()
	gzipReader, err := gzip.NewReader(bytes.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}
	defer gzipReader.Close()
	reader := tar.NewReader(gzipReader)
	var names []string
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, header.Name)
	}
	sort.Strings(names)
	return names
}
