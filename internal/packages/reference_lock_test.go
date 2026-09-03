package packages

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParsePackageReference(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		canonical string
		mutable   bool
		wantError string
	}{
		{name: "default tag", value: "oci://registry.example/team/package", canonical: "oci://registry.example/team/package:latest", mutable: true},
		{name: "tag", value: "oci://localhost:18080/team/package:dev", canonical: "oci://localhost:18080/team/package:dev", mutable: true},
		{name: "digest", value: "oci://registry.example/team/package@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", canonical: "oci://registry.example/team/package@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		{name: "uppercase scheme", value: "OCI://registry.example/team/package", wantError: "exact"},
		{name: "unknown scheme", value: "https://registry.example/team/package", wantError: "unsupported"},
		{name: "fragment", value: "oci://registry.example/team/package:latest#/child", wantError: "fragment"},
		{name: "tag and digest", value: "oci://registry.example/team/package:dev@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", wantError: "combine"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			parsed, err := parsePackageReference(test.value)
			if test.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantError) {
					t.Fatalf("parse error = %v, want fragment %q", err, test.wantError)
				}
				return
			}
			if err != nil {
				t.Fatalf("parse reference: %v", err)
			}
			if parsed.Canonical != test.canonical || parsed.Mutable != test.mutable {
				t.Fatalf("parsed reference = %#v", parsed)
			}
		})
	}
}

func TestLockEncodingSortsKeysAndReadIsStrict(t *testing.T) {
	const first = "oci://registry.example/a/package:latest"
	const second = "oci://registry.example/z/package:stable"
	const firstDigest = "oci://registry.example/a/package@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const secondDigest = "oci://registry.example/z/package@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

	data, err := encodeLock(Lock{Version: 1, Packages: map[string]LockedPackage{
		second: {Resolved: secondDigest},
		first:  {Resolved: firstDigest},
	}})
	if err != nil {
		t.Fatalf("encode lock: %v", err)
	}
	if strings.Index(string(data), first) > strings.Index(string(data), second) {
		t.Fatalf("lock keys are not sorted:\n%s", data)
	}

	root := packageTestRoot(t, "lock-strict")
	path := filepath.Join(root, "locus.lock")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write valid lock: %v", err)
	}
	lock, err := readLock(root)
	if err != nil {
		t.Fatalf("read lock: %v", err)
	}
	if len(lock.Packages) != 2 || lock.Packages[first].Resolved != firstDigest {
		t.Fatalf("decoded lock = %#v", lock)
	}

	invalid := []struct {
		name    string
		content string
	}{
		{name: "unknown field", content: "version: 1\npackages: {}\nextra: true\n"},
		{name: "duplicate field", content: "version: 1\nversion: 1\npackages: {}\n"},
		{name: "mutable resolved", content: "version: 1\npackages:\n  oci://registry.example/a/package:latest:\n    resolved: oci://registry.example/a/package:other\n"},
		{name: "different repository", content: "version: 1\npackages:\n  oci://registry.example/a/package:latest:\n    resolved: oci://registry.example/b/package@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\n"},
	}
	for _, test := range invalid {
		t.Run(test.name, func(t *testing.T) {
			if err := os.WriteFile(path, []byte(test.content), 0o644); err != nil {
				t.Fatalf("write invalid lock: %v", err)
			}
			if _, err := readLock(root); err == nil {
				t.Fatal("readLock succeeded for invalid lock")
			}
		})
	}
}

func packageTestRoot(t *testing.T, name string) string {
	t.Helper()
	root := filepath.Join("..", "..", "temp", "e2e-run", "packages-tests", name)
	if err := os.RemoveAll(root); err != nil {
		t.Fatalf("clear package test root: %v", err)
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("create package test root: %v", err)
	}
	return root
}
