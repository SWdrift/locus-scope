package npm

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

func TestIntegrityParsingChoosesSHA512(t *testing.T) {
	content := []byte("package")
	sha256Digest := sha256.Sum256(content)
	sha512Digest := sha512.Sum512(content)
	value := "sha256-" + base64.StdEncoding.EncodeToString(sha256Digest[:]) + " sha512-" + base64.StdEncoding.EncodeToString(sha512Digest[:])
	integrity, err := ParseIntegrity(value)
	if err != nil {
		t.Fatal(err)
	}
	if integrity.Algorithm() != "sha512" || integrity.Hex() == "" {
		t.Fatalf("integrity = %s/%s", integrity.Algorithm(), integrity.Hex())
	}
	if err := integrity.Verify(content); err != nil {
		t.Fatal(err)
	}
	if err := integrity.Verify([]byte("changed")); err == nil {
		t.Fatal("changed content passed integrity")
	}
	for _, invalid := range []string{"", "sha1-Zm9v", "sha512-not-base64", "sha256-Zm9v", value + "?option"} {
		if _, err := ParseIntegrity(invalid); err == nil {
			t.Errorf("ParseIntegrity(%q) succeeded", invalid)
		}
	}
}

func TestExtractTarballAcceptsRegularPackageTree(t *testing.T) {
	archive := testTarball(t, []tar.Header{
		{Name: "package/", Typeflag: tar.TypeDir, Mode: 0o755},
		{Name: "package/locus.yaml", Typeflag: tar.TypeReg, Mode: 0o644, Size: 4},
	}, [][]byte{nil, []byte("id:x")})
	destination := filepath.Join(npmTestRoot(t), "extract")
	if err := ExtractTarball(archive, destination); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(destination, "package", "locus.yaml"))
	if err != nil || string(content) != "id:x" {
		t.Fatalf("extracted content = %q, %v", content, err)
	}
}

func TestExtractTarballRejectsUnsafeEntries(t *testing.T) {
	tests := []struct {
		name    string
		headers []tar.Header
		bodies  [][]byte
	}{
		{name: "escape", headers: []tar.Header{{Name: "package/../outside", Typeflag: tar.TypeReg, Size: 1}}, bodies: [][]byte{[]byte("x")}},
		{name: "backslash", headers: []tar.Header{{Name: `package\\outside`, Typeflag: tar.TypeReg, Size: 1}}, bodies: [][]byte{[]byte("x")}},
		{name: "absolute", headers: []tar.Header{{Name: "/package/file", Typeflag: tar.TypeReg, Size: 1}}, bodies: [][]byte{[]byte("x")}},
		{name: "link", headers: []tar.Header{{Name: "package/link", Typeflag: tar.TypeSymlink, Linkname: "outside"}}, bodies: [][]byte{nil}},
		{name: "duplicate", headers: []tar.Header{{Name: "package/file", Typeflag: tar.TypeReg, Size: 1}, {Name: "package/file", Typeflag: tar.TypeReg, Size: 1}}, bodies: [][]byte{[]byte("a"), []byte("b")}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			archive := testTarball(t, test.headers, test.bodies)
			err := ExtractTarball(archive, filepath.Join(npmTestRoot(t), "extract"))
			if err == nil {
				t.Fatal("unsafe archive extracted")
			}
		})
	}
}

func testTarball(t *testing.T, headers []tar.Header, bodies [][]byte) []byte {
	t.Helper()
	var output bytes.Buffer
	gzipWriter := gzip.NewWriter(&output)
	tarWriter := tar.NewWriter(gzipWriter)
	for index := range headers {
		header := headers[index]
		if err := tarWriter.WriteHeader(&header); err != nil {
			t.Fatalf("write tar header: %v", err)
		}
		if len(bodies[index]) > 0 {
			if _, err := tarWriter.Write(bodies[index]); err != nil {
				t.Fatalf("write tar body: %v", err)
			}
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}
