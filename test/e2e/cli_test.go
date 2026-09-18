package e2e_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestNPMInstalledScopeGraph(t *testing.T) {
	h := newNPMLifecycleHarness(t)
	fixtures := filepath.Join(h.root, "fixtures")
	for _, fixture := range []struct{ name, source string }{
		{"base", repositoryPath("test", "e2e", "case", "npm", "packages", "base", "1.0.0")},
		{"helper", repositoryPath("test", "e2e", "case", "npm", "packages", "helper")},
		{"app", repositoryPath("test", "e2e", "case", "npm", "packages", "app")},
	} {
		destination := filepath.Join(fixtures, fixture.name)
		materializeFixture(t, fixture.source, destination)
		h.npmPublish("publish-"+fixture.name, destination, h.authEnv).success(t, "publish "+fixture.name)
	}

	adapter, platformName := stageAdapter(t, h)
	consumer := filepath.Join(h.root, "projects", "consumer")
	if err := os.MkdirAll(consumer, 0o755); err != nil {
		t.Fatal(err)
	}
	writeJSONFile(t, filepath.Join(consumer, "package.json"), map[string]any{
		"name": "consumer", "version": "1.0.0", "private": true,
		"dependencies": map[string]string{
			"@example/app":       "1.0.0",
			"@sundw/locus-scope": "file:" + filepath.ToSlash(adapter),
			platformName:         "file:" + filepath.ToSlash(filepath.Join(filepath.Dir(adapter), "platform")),
		},
	})
	if err := os.WriteFile(filepath.Join(consumer, "locus.yaml"), []byte("id: consumer\nimports:\n  app: '@example/app'\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h.pnpmInstall("consumer-install", consumer, h.anonEnv).success(t, "install consumer")

	cli := filepath.Join(consumer, "node_modules", "@sundw", "locus-scope", "bin", "locus-scope.mjs")
	validate := h.run("consumer-validate", consumer, h.anonEnv, h.node, cli, "validate", "--json").success(t, "validate installed graph")
	var summary struct {
		Valid                       bool `json:"valid"`
		Scopes, Entities, Relations int
	}
	decodeJSON(t, validate, &summary)
	if !summary.Valid || summary.Scopes != 3 || summary.Entities != 2 || summary.Relations != 1 {
		t.Fatalf("validation summary = %#v", summary)
	}
	entity := string(h.run("consumer-entity", consumer, h.anonEnv, h.node, cli, "entity", "app:base:database", "--json").success(t, "query transitive entity"))
	if !strings.Contains(entity, `"scope":"npm:@example/base@1.0.0"`) {
		t.Fatalf("transitive owner missing from %s", entity)
	}
	path := string(h.run("consumer-path", consumer, h.anonEnv, h.node, cli, "path", "app:application", "app:base:database", "--json").success(t, "query path"))
	if !strings.Contains(path, `"type":"uses"`) {
		t.Fatalf("path relation missing from %s", path)
	}
}

func stageAdapter(t *testing.T, h *npmLifecycleHarness) (string, string) {
	t.Helper()
	stage := filepath.Join(h.root, "fixtures", "adapter")
	platform := filepath.Join(h.root, "fixtures", "platform")
	materializeFixture(t, repositoryPath("packaging", "npm", "locus-scope"), stage)
	platformName, binary := currentPlatform()
	materializeFixture(t, repositoryPath("packaging", "npm", strings.TrimPrefix(platformName, "@sundw/")), platform)

	var adapterManifest map[string]any
	decodeJSON(t, fileBytes(t, filepath.Join(stage, "package.json")), &adapterManifest)
	adapterManifest["version"] = "0.0.0"
	delete(adapterManifest, "private")
	adapterManifest["optionalDependencies"] = map[string]string{platformName: "0.0.0"}
	writeJSONFile(t, filepath.Join(stage, "package.json"), adapterManifest)

	var platformManifest map[string]any
	decodeJSON(t, fileBytes(t, filepath.Join(platform, "package.json")), &platformManifest)
	platformManifest["version"] = "0.0.0"
	delete(platformManifest, "private")
	writeJSONFile(t, filepath.Join(platform, "package.json"), platformManifest)
	if err := os.MkdirAll(filepath.Join(platform, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := copyExecutable(h.host, filepath.Join(platform, "bin", binary)); err != nil {
		t.Fatal(err)
	}
	return stage, platformName
}

func currentPlatform() (string, string) {
	architecture := runtime.GOARCH
	if architecture == "amd64" {
		architecture = "x64"
	}
	operatingSystem := runtime.GOOS
	if operatingSystem == "windows" {
		operatingSystem = "win32"
	}
	name := "@sundw/locus-scope-" + operatingSystem + "-" + architecture
	binary := "locus-scope-node-host"
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	return name, binary
}

func copyExecutable(source, destination string) error {
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	return os.WriteFile(destination, data, 0o755)
}
