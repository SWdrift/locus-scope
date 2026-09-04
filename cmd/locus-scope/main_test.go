package main

import (
	"bytes"
	"encoding/json"
	"testing"

	"locus-scope/internal/buildinfo"
)

func TestRunShowsVersionWithoutScope(t *testing.T) {
	for _, arguments := range [][]string{{"version"}, {"--version"}} {
		var stdout, stderr bytes.Buffer
		if code := run(arguments, &stdout, &stderr); code != 0 {
			t.Fatalf("run %v exit code = %d, stderr = %s", arguments, code, stderr.String())
		}
		want := "locus-scope " + buildinfo.Version + "\n"
		if stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("run %v output = %q, stderr = %q, want %q", arguments, stdout.String(), stderr.String(), want)
		}
	}

	var stdout, stderr bytes.Buffer
	if code := run([]string{"--json", "version"}, &stdout, &stderr); code != 0 {
		t.Fatalf("JSON version exit code = %d, stderr = %s", code, stderr.String())
	}
	var info struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &info); err != nil {
		t.Fatalf("decode version output %q: %v", stdout.String(), err)
	}
	if info.Name != "locus-scope" || info.Version != buildinfo.Version {
		t.Fatalf("version output = %#v", info)
	}
}
