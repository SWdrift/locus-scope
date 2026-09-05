package packageenv_test

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"locus-scope/internal/packageenv"
)

func TestValidatePackageAcceptsOrdinaryAndLocusPackages(t *testing.T) {
	ordinary := packageTestRoot(t)
	writeFile(t, filepath.Join(ordinary, "package.json"), `{"name":"@example/helper","version":"1.0.0","main":"index.js"}`)
	metadata, isLocus, err := packageenv.ValidatePackage(ordinary)
	if err != nil {
		t.Fatalf("validate ordinary package: %v", err)
	}
	if isLocus || metadata.Name != "@example/helper" || metadata.Version != "1.0.0" || metadata.Entry != "" {
		t.Fatalf("ordinary package result = %#v, isLocus=%v", metadata, isLocus)
	}

	locus := packageTestRoot(t)
	writeFile(t, filepath.Join(locus, "package.json"), `{
  "name": "@example/app",
  "version": "1.0.0",
  "dependencies": {"@example/base": "^1.0.0"},
  "exports": {".": "./index.js", "./package.json": "./package.json"},
  "locus": {"entry": "scope/locus.yaml"}
}`)
	writeFile(t, filepath.Join(locus, "scope", "locus.yaml"), "id: app\nimports:\n  base: '@example/base'\n")
	writeFile(t, filepath.Join(locus, "node_modules", "@example", "base", "locus.yaml"), "id: installed-base\n")
	metadata, isLocus, err = packageenv.ValidatePackage(locus)
	if err != nil {
		t.Fatalf("validate Locus package: %v", err)
	}
	if !isLocus || metadata.Entry != "scope/locus.yaml" || metadata.Dependencies["@example/base"] != "^1.0.0" {
		t.Fatalf("Locus package result = %#v, isLocus=%v", metadata, isLocus)
	}
}

func TestValidatePackageRejectsLocusAndDependencyBoundaries(t *testing.T) {
	tests := []struct {
		name        string
		packageJSON string
		manifest    string
		extra       map[string]string
		want        string
	}{
		{
			name:        "unknown locus field",
			packageJSON: packageJSON(`{"entry":"locus.yaml","extra":true}`, `{}`),
			manifest:    "id: app\n",
			want:        "unknown field",
		},
		{
			name:        "escaping entry",
			packageJSON: packageJSON(`{"entry":"../locus.yaml"}`, `{}`),
			want:        "remain inside the package root",
		},
		{
			name:        "duplicate manifest",
			packageJSON: packageJSON(`{"entry":"locus.yaml"}`, `{}`),
			manifest:    "id: app\n",
			extra:       map[string]string{"nested/locus.yml": "id: nested\n"},
			want:        "exactly the declared Scope manifest",
		},
		{
			name:        "local import",
			packageJSON: packageJSON(`{"entry":"locus.yaml"}`, `{}`),
			manifest:    "id: app\nimports:\n  child: ./child\n",
			want:        "cannot use local source",
		},
		{
			name:        "undeclared package import",
			packageJSON: packageJSON(`{"entry":"locus.yaml"}`, `{}`),
			manifest:    "id: app\nimports:\n  base: '@example/base'\n",
			want:        "undeclared dependency",
		},
		{
			name:        "blocked package json export",
			packageJSON: packageJSON(`{"entry":"locus.yaml"}`, `{},"exports":{".":"./index.js"}`),
			manifest:    "id: app\n",
			want:        "explicitly expose ./package.json",
		},
		{
			name:        "unsupported dependency source",
			packageJSON: packageJSON(`{"entry":"locus.yaml"}`, `{"@example/base":"workspace:*"}`),
			manifest:    "id: app\n",
			want:        "Registry SemVer constraint",
		},
		{
			name:        "peer dependencies",
			packageJSON: packageJSON(`{"entry":"locus.yaml"}`, `{},"peerDependencies":{}`),
			manifest:    "id: app\n",
			want:        "peerDependencies are not supported",
		},
		{
			name:        "duplicate json field",
			packageJSON: `{"name":"@example/app","name":"@example/other","version":"1.0.0"}`,
			want:        "duplicate object field",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := packageTestRoot(t)
			writeFile(t, filepath.Join(root, "package.json"), test.packageJSON)
			if test.manifest != "" {
				writeFile(t, filepath.Join(root, "locus.yaml"), test.manifest)
			}
			for name, contents := range test.extra {
				writeFile(t, filepath.Join(root, filepath.FromSlash(name)), contents)
			}
			_, _, err := packageenv.ValidatePackage(root)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validation error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func packageJSON(locus, dependenciesAndFields string) string {
	return fmt.Sprintf(`{"name":"@example/app","version":"1.0.0","dependencies":%s,"locus":%s}`, dependenciesAndFields, locus)
}
