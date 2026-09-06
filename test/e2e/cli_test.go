package e2e_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

func TestNPMPackageLifecycle(t *testing.T) {
	h := newNPMLifecycleHarness(t)
	fixtureRoot := repositoryPath("test", "e2e", "case", "npm")
	publishedRoot := filepath.Join(h.root, "fixtures", "published")

	base10 := publishFixture(t, h, "base-1.0.0", filepath.Join(fixtureRoot, "packages", "base", "1.0.0"), filepath.Join(publishedRoot, "base-1.0.0"))
	base20 := publishFixture(t, h, "base-2.0.0", filepath.Join(fixtureRoot, "packages", "base", "2.0.0"), filepath.Join(publishedRoot, "base-2.0.0"))
	publishFixture(t, h, "helper-1.0.0", filepath.Join(fixtureRoot, "packages", "helper"), filepath.Join(publishedRoot, "helper"))
	publishFixture(t, h, "app-1.0.0", filepath.Join(fixtureRoot, "packages", "app"), filepath.Join(publishedRoot, "app"))
	publishFixture(t, h, "modern-1.0.0", filepath.Join(fixtureRoot, "packages", "modern"), filepath.Join(publishedRoot, "modern"))

	basePackument := getPackument(t, h.endpoint, "@example/base", "")
	if len(basePackument.Versions) != 2 || basePackument.Versions["1.0.0"].Dist.Integrity == "" || basePackument.Versions["2.0.0"].Dist.Integrity == "" {
		t.Fatalf("base packument does not expose the initially published versions: %#v", basePackument.Versions)
	}
	appPackument := getPackument(t, h.endpoint, "@example/app", "")
	appMetadata := appPackument.Versions["1.0.0"]
	if appMetadata.Dependencies["@example/base"] != "^1.0.0" || appMetadata.Dependencies["@example/helper"] != "1.0.0" {
		t.Fatalf("app dependency metadata = %#v", appMetadata.Dependencies)
	}
	baseArchive := fetchBytes(t, basePackument.Versions["1.0.0"].Dist.Tarball)
	verifyIntegrity(t, basePackument.Versions["1.0.0"].Dist.Integrity, baseArchive)
	if !containsString(tarFileNames(t, baseArchive), "package/package.json") || !containsString(tarFileNames(t, baseArchive), "package/locus.yaml") {
		t.Fatalf("downloaded base tarball does not contain its npm and Scope manifests")
	}
	if !bytes.Contains(base10.stdout, []byte("@example/base@1.0.0")) || !bytes.Contains(base20.stdout, []byte("@example/base@2.0.0")) {
		t.Fatalf("standard npm publish output did not identify published base versions")
	}
	h.npmPublish("immutable-version-conflict", filepath.Join(publishedRoot, "base-1.0.0"), h.authEnv).failure(t, "immutable version conflict")

	existingProject := filepath.Join(h.root, "projects", "pure-existing")
	materializeFixture(t, filepath.Join(fixtureRoot, "consumer"), existingProject)
	initialInstall := h.pure("pure-install-initial", existingProject, h.authEnv, h.pkg,
		"--json", "--registry", h.endpoint, "install", "@example/app@^1", "@example/modern@^1").success(t, "initial Pure install")
	assertInstallResult(t, initialInstall, 5, 5, 5, 4)
	initialLock := fileBytes(t, filepath.Join(existingProject, "locus.lock"))
	assertLockContains(t, initialLock,
		"npm:@example/app@1.0.0", "npm:@example/modern@1.0.0", "npm:@example/helper@1.0.0",
		"npm:@example/base@1.0.0", "npm:@example/base@2.0.0")
	if bytes.Contains(initialLock, []byte("npm:@example/base@1.1.0")) {
		t.Fatalf("initial lock unexpectedly selected unpublished base 1.1.0")
	}
	if directories := directoryNames(t, filepath.Join(existingProject, ".locus", "packages")); len(directories) != 5 {
		t.Fatalf("Pure store directories = %d, want 5: %v", len(directories), directories)
	}
	initialSnapshot := pureQuerySnapshot(t, h, "pure-initial", existingProject)
	if bytes.Contains(initialSnapshot, []byte("@example/helper")) {
		t.Fatalf("ordinary helper package entered the Scope graph: %s", initialSnapshot)
	}
	assertResolvedOwner(t, h, existingProject, "app:base:database", "npm:@example/base@1.0.0", "resolve-app-base-v1")
	assertResolvedOwner(t, h, existingProject, "modern:base:database", "npm:@example/base@2.0.0", "resolve-modern-base-v2")
	if err := os.WriteFile(filepath.Join(h.results, "initial-locus.lock"), initialLock, 0o644); err != nil {
		t.Fatalf("persist initial lock: %v", err)
	}

	base11Published := filepath.Join(publishedRoot, "base-1.1.0")
	materializeFixture(t, filepath.Join(fixtureRoot, "packages", "base", "1.1.0"), base11Published)
	h.npmPublish("publish-base-1.1.0", base11Published, h.authEnv, "--tag", "legacy").success(t, "publish base-1.1.0")
	newProject := filepath.Join(h.root, "projects", "pure-new")
	materializeFixture(t, filepath.Join(fixtureRoot, "consumer"), newProject)
	h.pure("pure-install-new", newProject, h.authEnv, h.pkg,
		"--json", "--registry", h.endpoint, "install").success(t, "new Pure install")
	newLock := fileBytes(t, filepath.Join(newProject, "locus.lock"))
	assertLockContains(t, newLock, "npm:@example/base@1.1.0", "npm:@example/base@2.0.0")

	h.pure("pure-existing-locked-install", existingProject, h.authEnv, h.pkg,
		"--json", "--registry", h.endpoint, "install").success(t, "locked Pure reinstall")
	if after := fileBytes(t, filepath.Join(existingProject, "locus.lock")); !bytes.Equal(initialLock, after) {
		t.Fatalf("ordinary install drifted an existing valid lock\nbefore:\n%s\nafter:\n%s", initialLock, after)
	}
	updateOutput := h.pure("pure-update-app", existingProject, h.authEnv, h.pkg,
		"--json", "--registry", h.endpoint, "update", "@example/app").success(t, "named Pure update")
	var updateResult struct {
		Added, Removed, Updated []string
	}
	decodeJSON(t, updateOutput, &updateResult)
	for _, unchanged := range []string{"npm:@example/modern@1.0.0", "npm:@example/base@2.0.0"} {
		if containsString(updateResult.Added, unchanged) || containsString(updateResult.Removed, unchanged) || containsString(updateResult.Updated, unchanged) {
			t.Fatalf("named app update reported unchanged modern closure %q: %#v", unchanged, updateResult)
		}
	}
	updatedLock := fileBytes(t, filepath.Join(existingProject, "locus.lock"))
	assertLockContains(t, updatedLock, "npm:@example/base@1.1.0", "npm:@example/base@2.0.0")
	if bytes.Contains(updatedLock, []byte("npm:@example/base@1.0.0")) {
		t.Fatalf("named app update retained its obsolete base 1.0.0 closure:\n%s", updatedLock)
	}
	assertResolvedOwner(t, h, existingProject, "app:base:database", "npm:@example/base@1.1.0", "resolve-app-base-v1-1")
	updatedSnapshot := pureQuerySnapshot(t, h, "pure-updated", existingProject)

	projectManifestPath := filepath.Join(existingProject, "package.json")
	stableManifest := fileBytes(t, projectManifestPath)
	stableLock := append([]byte(nil), updatedLock...)
	var changedManifest map[string]any
	decodeJSON(t, stableManifest, &changedManifest)
	changedManifest["dependencies"].(map[string]any)["@example/app"] = "^9.0.0"
	writeJSONFile(t, projectManifestPath, changedManifest)
	frozenManifest := fileBytes(t, projectManifestPath)
	h.pure("pure-frozen-mismatch", existingProject, h.authEnv, h.pkg,
		"--json", "--registry", h.endpoint, "--frozen-lockfile", "install").failure(t, "frozen mismatch")
	if !bytes.Equal(frozenManifest, fileBytes(t, projectManifestPath)) || !bytes.Equal(stableLock, fileBytes(t, filepath.Join(existingProject, "locus.lock"))) {
		t.Fatalf("frozen mismatch changed package.json or locus.lock")
	}
	if err := os.WriteFile(projectManifestPath, stableManifest, 0o644); err != nil {
		t.Fatalf("restore stable package.json after frozen assertion: %v", err)
	}

	packedSource := filepath.Join(h.root, "fixtures", "pure-published")
	writeLocusPackage(t, packedSource, "@example/pure-published", "1.0.0", nil,
		map[string]string{".": "./entities.locus.yaml", "./package.json": "./package.json"}, false)
	packOutput := h.pure("locus-pkg-pack", packedSource, h.authEnv, h.pkg, "--json", "pack").success(t, "locus-pkg pack")
	var packResult struct {
		Name, Version, Filename, Integrity string
		Files                              []string
	}
	decodeJSON(t, packOutput, &packResult)
	if packResult.Name != "@example/pure-published" || packResult.Version != "1.0.0" || packResult.Filename == "" || len(packResult.Files) < 3 {
		t.Fatalf("pack result = %#v", packResult)
	}
	packedArchive := fileBytes(t, filepath.Join(packedSource, packResult.Filename))
	verifyIntegrity(t, packResult.Integrity, packedArchive)
	if !containsString(tarFileNames(t, packedArchive), "package/entities.locus.yaml") {
		t.Fatalf("locus-pkg pack omitted declared Scope content")
	}
	publishOutput := h.pure("locus-pkg-publish", packedSource, h.authEnv, h.pkg,
		"--json", "--registry", h.endpoint, "publish").success(t, "locus-pkg publish")
	var publishResult struct {
		Name, Version, Registry, Integrity string
	}
	decodeJSON(t, publishOutput, &publishResult)
	if publishResult.Name != packResult.Name || publishResult.Version != packResult.Version || publishResult.Integrity != packResult.Integrity {
		t.Fatalf("publish result = %#v, pack result = %#v", publishResult, packResult)
	}
	h.pure("locus-pkg-publish-conflict", packedSource, h.authEnv, h.pkg,
		"--json", "--registry", h.endpoint, "publish").failure(t, "locus-pkg immutable version conflict")
	publishedPackument := getPackument(t, h.endpoint, "@example/pure-published", "")
	publishedArchive := fetchBytes(t, publishedPackument.Versions["1.0.0"].Dist.Tarball)
	verifyIntegrity(t, publishResult.Integrity, publishedArchive)
	installPackedWithManagers(t, h)

	publishFixture(t, h, "private-1.0.0", filepath.Join(fixtureRoot, "packages", "private"), filepath.Join(publishedRoot, "private"))
	h.run("private-npm-view-missing-token", h.root, h.anonEnv, h.npm,
		"view", "@example/private", "version", "--registry", h.endpoint).failure(t, "private npm view without token")
	h.run("private-npm-view-wrong-token", h.root, h.wrongEnv, h.npm,
		"view", "@example/private", "version", "--registry", h.endpoint).failure(t, "private npm view with wrong token")
	privateVersion := h.run("private-npm-view-authenticated", h.root, h.authEnv, h.npm,
		"view", "@example/private", "version", "--registry", h.endpoint).success(t, "authenticated private npm view")
	if strings.TrimSpace(string(privateVersion)) != "1.0.0" {
		t.Fatalf("authenticated private version = %q", privateVersion)
	}
	privateMissing := createLocalProject(t, filepath.Join(h.root, "projects", "private-missing"), "private-missing", nil)
	beforeMissing := snapshotTransaction(t, privateMissing)
	h.pure("private-pure-missing-token", privateMissing, h.anonEnv, h.pkg,
		"--json", "--registry", h.endpoint, "install", "@example/private@1.0.0").failure(t, "private Pure install without token")
	assertTransactionUnchanged(t, privateMissing, beforeMissing)
	privateWrong := createLocalProject(t, filepath.Join(h.root, "projects", "private-wrong"), "private-wrong", nil)
	beforeWrong := snapshotTransaction(t, privateWrong)
	h.pure("private-pure-wrong-token", privateWrong, h.wrongEnv, h.pkg,
		"--json", "--registry", h.endpoint, "install", "@example/private@1.0.0").failure(t, "private Pure install with wrong token")
	assertTransactionUnchanged(t, privateWrong, beforeWrong)
	privateAuth := createLocalProject(t, filepath.Join(h.root, "projects", "private-authenticated"), "private-authenticated", nil)
	h.pure("private-pure-authenticated", privateAuth, h.authEnv, h.pkg,
		"--json", "--registry", h.endpoint, "install", "@example/private@1.0.0").success(t, "authenticated private Pure install")
	assertLockContains(t, fileBytes(t, filepath.Join(privateAuth, "locus.lock")), "npm:@example/private@1.0.0")

	publishFailureFixtures(t, h)
	failureProject := createLocalProject(t, filepath.Join(h.root, "projects", "failed-installs"), "failed-installs", nil)
	assertFailedInstallRollback(t, h, failureProject, h.endpoint, "@failure/unsupported@1.0.0", "failure-unsupported-spec")
	assertFailedInstallRollback(t, h, failureProject, h.endpoint, "@failure/duplicate@1.0.0", "failure-duplicate-scope")
	assertFailedInstallRollback(t, h, failureProject, h.endpoint, "@failure/blocked@1.0.0", "failure-blocked-exports")
	assertFailedInstallRollback(t, h, failureProject, h.endpoint, "@failure/unsafe@1.0.0", "failure-unsafe-tar")
	mismatchRegistry := integrityMismatchRegistry(t, h.endpoint, "@failure/integrity", "1.0.0")
	if err := os.WriteFile(filepath.Join(h.root, "registry", "integrity-proxy.txt"), []byte(mismatchRegistry.URL+"\n"), 0o644); err != nil {
		t.Fatalf("persist integrity mismatch endpoint: %v", err)
	}
	assertFailedInstallRollback(t, h, failureProject, mismatchRegistry.URL, "@failure/integrity@1.0.0", "failure-integrity-mismatch")
	mismatchRegistry.Close()

	platformPackage, platformHostName := currentPlatformPackage(t)
	releaseVersion := strings.TrimSpace(string(fileBytes(t, repositoryPath("VERSION"))))
	platformSource := filepath.Join(h.root, "fixtures", "node-platform")
	materializeFixture(t, repositoryPath("packaging", "npm", platformPackage), platformSource)
	stageNodePublishFixture(t, platformSource, releaseVersion, platformHostName)
	platformBin := filepath.Join(platformSource, "bin")
	if err := os.MkdirAll(platformBin, 0o755); err != nil {
		t.Fatalf("create platform package bin: %v", err)
	}
	if err := copyFile(h.host, filepath.Join(platformBin, platformHostName), 0o755); err != nil {
		t.Fatalf("stage platform host: %v", err)
	}
	h.npmPublish("publish-node-platform", platformSource, h.authEnv).success(t, "publish current Node platform package")
	nodePackageSource := filepath.Join(h.root, "fixtures", "locus-scope-node")
	materializeFixture(t, repositoryPath("packaging", "npm", "locus-scope"), nodePackageSource)
	stageNodePublishFixture(t, nodePackageSource, releaseVersion, "")
	h.npmPublish("publish-locus-scope-node", nodePackageSource, h.authEnv).success(t, "publish @sundw/locus-scope")
	var nodePackageManifest struct {
		Version string `json:"version"`
	}
	decodeJSON(t, fileBytes(t, filepath.Join(nodePackageSource, "package.json")), &nodePackageManifest)

	npmNodeConsumer := createNodeConsumer(t, filepath.Join(h.root, "projects", "node-npm"), filepath.Join(fixtureRoot, "consumer"), nodePackageManifest.Version)
	h.npmInstall("node-npm-install", npmNodeConsumer, h.authEnv).success(t, "npm Node consumer install")
	npmSnapshot := nodeQuerySnapshot(t, h, "node-npm", npmNodeConsumer)
	pnpmNodeConsumer := createNodeConsumer(t, filepath.Join(h.root, "projects", "node-pnpm"), filepath.Join(fixtureRoot, "consumer"), nodePackageManifest.Version)
	h.pnpmInstall("node-pnpm-install", pnpmNodeConsumer, h.authEnv).success(t, "pnpm Node consumer install")
	assertSymlink(t, filepath.Join(pnpmNodeConsumer, "node_modules", "@sundw", "locus-scope"), "pnpm @sundw/locus-scope installation")
	pnpmSnapshot := nodeQuerySnapshot(t, h, "node-pnpm", pnpmNodeConsumer)
	if !bytes.Equal(updatedSnapshot, npmSnapshot) || !bytes.Equal(updatedSnapshot, pnpmSnapshot) {
		t.Fatalf("Pure/npm/pnpm normalized query results differ\nPure: %s\nnpm: %s\npnpm: %s", updatedSnapshot, npmSnapshot, pnpmSnapshot)
	}
	if err := os.WriteFile(filepath.Join(h.results, "normalized-query.json"), updatedSnapshot, 0o644); err != nil {
		t.Fatalf("persist normalized query comparison: %v", err)
	}

	pureManagement := exerciseManagementCLI(t, existingProject, func(name string, arguments ...string) commandResult {
		return h.pure("management-pure-"+name, existingProject, h.authEnv, h.scope,
			append([]string{"--scope", existingProject, "--json"}, arguments...)...)
	})
	nodeAdapter := filepath.Join(npmNodeConsumer, "node_modules", "@sundw", "locus-scope", "bin", "locus-scope-node.mjs")
	nodeManagement := exerciseManagementCLI(t, npmNodeConsumer, func(name string, arguments ...string) commandResult {
		return h.run("management-node-"+name, npmNodeConsumer, h.authEnv, h.node,
			append([]string{nodeAdapter, "--scope", npmNodeConsumer, "--json"}, arguments...)...)
	})
	if !bytes.Equal(pureManagement, nodeManagement) {
		t.Fatalf("Pure and Node management results differ\nPure: %s\nNode: %s", pureManagement, nodeManagement)
	}

	blockedNodeConsumer := createLocalProject(t, filepath.Join(h.root, "projects", "node-blocked-exports"), "node-blocked-exports",
		map[string]string{"blocked": "@failure/blocked"})
	var blockedManifest map[string]any
	decodeJSON(t, fileBytes(t, filepath.Join(blockedNodeConsumer, "package.json")), &blockedManifest)
	blockedManifest["dependencies"] = map[string]string{"@failure/blocked": "1.0.0", "@sundw/locus-scope": nodePackageManifest.Version}
	writeJSONFile(t, filepath.Join(blockedNodeConsumer, "package.json"), blockedManifest)
	h.npmInstall("node-blocked-install", blockedNodeConsumer, h.authEnv).success(t, "install blocked-export Node fixture")
	h.run("node-blocked-exports", blockedNodeConsumer, h.authEnv, h.node,
		filepath.Join(blockedNodeConsumer, "node_modules", "@sundw", "locus-scope", "bin", "locus-scope-node.mjs"),
		"--scope", blockedNodeConsumer, "--json", "validate").failure(t, "Node blocked package.json export")

	h.registry.stop(t)
	offlineLock := fileBytes(t, filepath.Join(existingProject, "locus.lock"))
	h.pure("pure-offline-frozen", existingProject, h.authEnv, h.pkg,
		"--json", "--offline", "--frozen-lockfile", "install").success(t, "offline frozen Pure install")
	if !bytes.Equal(offlineLock, fileBytes(t, filepath.Join(existingProject, "locus.lock"))) {
		t.Fatalf("offline frozen install changed locus.lock")
	}
	offlineSnapshot := pureQuerySnapshot(t, h, "pure-offline", existingProject)
	if !bytes.Equal(updatedSnapshot, offlineSnapshot) {
		t.Fatalf("offline queries differ from online Pure result\nonline: %s\noffline: %s", updatedSnapshot, offlineSnapshot)
	}

	assertNoSecret(t, h.token, filepath.Join(h.root, "projects"), h.results)
}

func exerciseManagementCLI(t *testing.T, project string, run func(string, ...string) commandResult) []byte {
	t.Helper()
	run("entity-add", "entity", "add", "cache", "type=service", "network.region=tokyo", "--file", "management.locus.yaml").success(t, "entity add")
	run("entity-set", "entity", "set", "cache", "replicas=3").success(t, "entity set")
	run("relation-add", "relation", "add", "root", "manages", "cache", "critical=true").success(t, "relation add")
	run("relation-set", "relation", "set", "root", "manages", "cache", "weight=2").success(t, "relation set")

	var relations []struct {
		Object map[string]any `json:"object"`
	}
	relationFilters := []string{"relation", "from=root", "type=manages", "to=cache"}
	decodeJSON(t, run("relation-find-after-set", relationFilters...).success(t, "relation find after set"), &relations)
	if len(relations) != 1 {
		t.Fatalf("Relation query after set = %#v", relations)
	}
	if object := relations[0].Object; object["from"] != "root" || object["type"] != "manages" || object["to"] != "cache" ||
		object["critical"] != true || object["weight"] != float64(2) {
		t.Fatalf("Relation object after set = %#v", object)
	}

	run("relation-unset", "relation", "unset", "root", "manages", "cache", "weight").success(t, "relation unset")
	relations = nil
	decodeJSON(t, run("relation-find-after-unset", relationFilters...).success(t, "relation find after unset"), &relations)
	if len(relations) != 1 {
		t.Fatalf("Relation query after unset = %#v", relations)
	}
	if _, exists := relations[0].Object["weight"]; exists {
		t.Fatalf("Relation unset retained weight: %#v", relations[0].Object)
	}
	definition := fileBytes(t, filepath.Join(project, "scope.locus.yaml"))
	for _, fragment := range [][]byte{[]byte("from: root"), []byte("type: manages"), []byte("to: cache"), []byte("critical: true")} {
		if !bytes.Contains(definition, fragment) {
			t.Fatalf("canonical Relation declaration %q missing from %s:\n%s", fragment, filepath.Join(project, "scope.locus.yaml"), definition)
		}
	}
	if bytes.Contains(definition, []byte("[root, manages, cache]")) {
		t.Fatalf("Relation writer emitted legacy tuple:\n%s", definition)
	}

	var diff any
	decodeJSON(t, run("diff", "diff", "scope:.", "path:"+filepath.ToSlash(project)).success(t, "diff"), &diff)

	results := make(map[string]any)
	for name, arguments := range map[string][]string{
		"entity-find":   {"entity", "network.region=tokyo", "--source"},
		"relation-find": {"relation", "type=manages", "critical=true", "--source"},
		"graph":         {"graph", "root", "--depth", "2", "--via", "type=manages"},
		"path":          {"path", "root", "cache", "--via", "type=manages"},
		"impact":        {"impact", "cache", "--via", "type=manages"},
		"scope":         {"scope", "."},
		"group-list":    {"group"},
		"validate":      {"validate"},
	} {
		var decoded any
		decodeJSON(t, run(name, arguments...).success(t, name), &decoded)
		results[name] = normalizeQueryValue(decoded)
	}

	run("dangling-remove", "entity", "remove", "cache").failure(t, "dangling relation rollback")
	run("rollback-show", "entity", "cache").success(t, "entity remains after rollback")
	run("relation-remove", "relation", "remove", "root", "manages", "cache").success(t, "relation remove")
	run("entity-remove", "entity", "remove", "cache").success(t, "entity remove")

	body, err := json.Marshal(results)
	if err != nil {
		t.Fatalf("encode normalized management snapshot: %v", err)
	}
	return body
}

type npmPackument struct {
	Versions map[string]struct {
		Dependencies map[string]string `json:"dependencies"`
		Dist         struct {
			Tarball   string `json:"tarball"`
			Integrity string `json:"integrity"`
		} `json:"dist"`
	} `json:"versions"`
}

func publishFixture(t *testing.T, h *npmLifecycleHarness, name, source, destination string) commandResult {
	t.Helper()
	materializeFixture(t, source, destination)
	result := h.npmPublish("publish-"+name, destination, h.authEnv)
	result.success(t, "publish "+name)
	return result
}

func getPackument(t *testing.T, endpoint, name, token string) npmPackument {
	t.Helper()
	var packument npmPackument
	fetchJSON(t, endpoint+"/"+strings.ReplaceAll(name, "/", "%2f"), token, &packument)
	return packument
}

func assertInstallResult(t *testing.T, output []byte, packages, scopes, entities, relations int) {
	t.Helper()
	var result struct {
		Valid     bool `json:"valid"`
		Packages  int  `json:"packages"`
		Scopes    int  `json:"scopes"`
		Entities  int  `json:"entities"`
		Relations int  `json:"relations"`
	}
	decodeJSON(t, output, &result)
	if !result.Valid || result.Packages != packages || result.Scopes != scopes || result.Entities != entities || result.Relations != relations {
		t.Fatalf("install result = %#v", result)
	}
}

func assertLockContains(t *testing.T, lock []byte, identities ...string) {
	t.Helper()
	for _, identity := range identities {
		if !bytes.Contains(lock, []byte(identity)) {
			t.Fatalf("lock does not contain %q:\n%s", identity, lock)
		}
	}
}

func assertResolvedOwner(t *testing.T, h *npmLifecycleHarness, project, reference, owner, name string) {
	t.Helper()
	output := h.pure(name, project, h.authEnv, h.scope,
		"--scope", project, "--json", "entity", reference).success(t, name)
	var result struct {
		Key struct {
			Scope string `json:"scope"`
			ID    string `json:"id"`
		} `json:"key"`
	}
	decodeJSON(t, output, &result)
	if result.Key.Scope != owner || result.Key.ID != "database" {
		t.Fatalf("resolve %s = %#v, want database owned by %s", reference, result.Key, owner)
	}
}

func pureQuerySnapshot(t *testing.T, h *npmLifecycleHarness, name, project string) []byte {
	t.Helper()
	return querySnapshot(t, func(suffix string, arguments ...string) []byte {
		return h.pure(name+"-"+suffix, project, h.authEnv, h.scope,
			append([]string{"--scope", project, "--json"}, arguments...)...).success(t, name+" "+suffix)
	})
}

func nodeQuerySnapshot(t *testing.T, h *npmLifecycleHarness, name, project string) []byte {
	t.Helper()
	adapter := filepath.Join(project, "node_modules", "@sundw", "locus-scope", "bin", "locus-scope-node.mjs")
	return querySnapshot(t, func(suffix string, arguments ...string) []byte {
		return h.run(name+"-"+suffix, project, h.authEnv, h.node,
			append([]string{adapter, "--scope", project, "--json"}, arguments...)...).success(t, name+" "+suffix)
	})
}

func querySnapshot(t *testing.T, run func(string, ...string) []byte) []byte {
	t.Helper()
	queries := []struct {
		name      string
		arguments []string
	}{
		{"scopes", []string{"scope"}},
		{"entities", []string{"entity"}},
		{"relations", []string{"relation"}},
	}
	result := make(map[string]any, len(queries))
	for _, query := range queries {
		var decoded any
		decodeJSON(t, run(query.name, query.arguments...), &decoded)
		result[query.name] = normalizeQueryValue(decoded)
	}
	body, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("encode normalized query snapshot: %v", err)
	}
	return body
}

func normalizeQueryValue(value any) any {
	switch typed := value.(type) {
	case string:
		if strings.HasPrefix(typed, "file://") {
			return "file://<root>"
		}
		return typed
	case []any:
		for index := range typed {
			typed[index] = normalizeQueryValue(typed[index])
		}
		return typed
	case map[string]any:
		for key := range typed {
			typed[key] = normalizeQueryValue(typed[key])
		}
		return typed
	default:
		return value
	}
}

func installPackedWithManagers(t *testing.T, h *npmLifecycleHarness) {
	t.Helper()
	for _, manager := range []struct {
		name    string
		install func(string, string, []string, ...string) commandResult
	}{
		{"npm", h.npmInstall},
		{"pnpm", h.pnpmInstall},
	} {
		root := filepath.Join(h.root, "projects", "packed-"+manager.name)
		writeJSONFile(t, filepath.Join(root, "package.json"), map[string]any{"name": "packed-" + manager.name, "version": "1.0.0", "private": true})
		manager.install("packed-"+manager.name+"-install", root, h.authEnv, "@example/pure-published@1.0.0").success(t, manager.name+" packed-package install")
		manifest := fileBytes(t, filepath.Join(root, "node_modules", "@example", "pure-published", "package.json"))
		if !bytes.Contains(manifest, []byte(`"version": "1.0.0"`)) && !bytes.Contains(manifest, []byte(`"version":"1.0.0"`)) {
			t.Fatalf("%s installed unexpected packed package manifest: %s", manager.name, manifest)
		}
		if manager.name == "pnpm" {
			assertSymlink(t, filepath.Join(root, "node_modules", "@example", "pure-published"), "pnpm packed package installation")
		}
	}
}

func createLocalProject(t *testing.T, root, name string, imports map[string]string) string {
	t.Helper()
	writeJSONFile(t, filepath.Join(root, "package.json"), map[string]any{"name": name, "version": "1.0.0", "private": true})
	aliases := make([]string, 0, len(imports))
	for alias := range imports {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	manifest := "id: " + name + "\n"
	if len(aliases) != 0 {
		manifest += "imports:\n"
		for _, alias := range aliases {
			manifest += fmt.Sprintf("  %s: %q\n", alias, imports[alias])
		}
	}
	manifest += "exports:\n  - root\n"
	if err := os.WriteFile(filepath.Join(root, "locus.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write local project Scope: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "entities.locus.yaml"), []byte("entities:\n  - id: root\n"), 0o644); err != nil {
		t.Fatalf("write local project entity: %v", err)
	}
	return root
}

type transactionSnapshot struct {
	packageJSON []byte
	lockExists  bool
	lock        []byte
	store       []string
}

func snapshotTransaction(t *testing.T, project string) transactionSnapshot {
	t.Helper()
	snapshot := transactionSnapshot{packageJSON: fileBytes(t, filepath.Join(project, "package.json")), store: directoryNames(t, filepath.Join(project, ".locus", "packages"))}
	lock, err := os.ReadFile(filepath.Join(project, "locus.lock"))
	if err == nil {
		snapshot.lockExists = true
		snapshot.lock = lock
	} else if !os.IsNotExist(err) {
		t.Fatalf("read transaction lock: %v", err)
	}
	return snapshot
}

func assertTransactionUnchanged(t *testing.T, project string, before transactionSnapshot) {
	t.Helper()
	if !bytes.Equal(before.packageJSON, fileBytes(t, filepath.Join(project, "package.json"))) {
		t.Fatalf("failed operation changed package.json in %s", project)
	}
	afterLock, err := os.ReadFile(filepath.Join(project, "locus.lock"))
	if before.lockExists {
		if err != nil || !bytes.Equal(before.lock, afterLock) {
			t.Fatalf("failed operation changed locus.lock in %s: %v", project, err)
		}
	} else if !os.IsNotExist(err) {
		t.Fatalf("failed operation created locus.lock in %s", project)
	}
	if afterStore := directoryNames(t, filepath.Join(project, ".locus", "packages")); !equalStrings(before.store, afterStore) {
		t.Fatalf("failed operation changed committed store in %s: before=%v after=%v", project, before.store, afterStore)
	}
}

func assertFailedInstallRollback(t *testing.T, h *npmLifecycleHarness, project, registry, spec, name string) {
	t.Helper()
	before := snapshotTransaction(t, project)
	result := h.pure(name, project, h.authEnv, h.pkg, "--json", "--registry", registry, "install", spec)
	result.failure(t, name)
	if len(bytes.TrimSpace(result.stderr)) == 0 {
		t.Fatalf("%s returned no observable error", name)
	}
	assertTransactionUnchanged(t, project, before)
}

func publishFailureFixtures(t *testing.T, h *npmLifecycleHarness) {
	t.Helper()
	root := filepath.Join(h.root, "fixtures", "failures")
	integrity := filepath.Join(root, "integrity")
	writeLocusPackage(t, integrity, "@failure/integrity", "1.0.0", nil,
		map[string]string{".": "./entities.locus.yaml", "./package.json": "./package.json"}, false)
	h.npmPublish("publish-failure-integrity", integrity, h.authEnv).success(t, "publish integrity fixture")
	unsupported := filepath.Join(root, "unsupported")
	writeLocusPackage(t, unsupported, "@failure/unsupported", "1.0.0", map[string]string{"@example/helper": "file:../helper"},
		map[string]string{".": "./entities.locus.yaml", "./package.json": "./package.json"}, false)
	h.npmPublish("publish-failure-unsupported", unsupported, h.authEnv).success(t, "publish unsupported spec fixture")
	duplicate := filepath.Join(root, "duplicate")
	writeLocusPackage(t, duplicate, "@failure/duplicate", "1.0.0", nil,
		map[string]string{".": "./entities.locus.yaml", "./package.json": "./package.json"}, true)
	h.npmPublish("publish-failure-duplicate", duplicate, h.authEnv).success(t, "publish duplicate Scope fixture")
	blocked := filepath.Join(root, "blocked")
	writeLocusPackage(t, blocked, "@failure/blocked", "1.0.0", nil,
		map[string]string{".": "./entities.locus.yaml"}, false)
	h.npmPublish("publish-failure-blocked", blocked, h.authEnv).success(t, "publish blocked exports fixture")
	unsafeArchive, unsafeManifest := makeUnsafeTarball(t, "@failure/unsafe", "1.0.0")
	publishRawPackage(t, h.endpoint, h.token, "@failure/unsafe", "1.0.0", unsafeArchive, unsafeManifest)
}

func stageNodePublishFixture(t *testing.T, root, version, platformHostName string) {
	t.Helper()
	manifestPath := filepath.Join(root, "package.json")
	var manifest map[string]any
	decodeJSON(t, fileBytes(t, manifestPath), &manifest)
	if private, exists := manifest["private"].(bool); !exists || !private {
		t.Fatalf("Node package source manifest must be private")
	}
	delete(manifest, "private")
	manifest["version"] = version
	if optional, exists := manifest["optionalDependencies"].(map[string]any); exists {
		for name := range optional {
			optional[name] = version
		}
	}
	if platformHostName != "" && runtime.GOOS != "windows" {
		manifest["bin"] = map[string]string{"locus-scope-node-host": "bin/" + platformHostName}
	}
	writeJSONFile(t, manifestPath, manifest)
}

func createNodeConsumer(t *testing.T, destination, source, locusVersion string) string {
	t.Helper()
	materializeFixture(t, source, destination)
	var manifest map[string]any
	decodeJSON(t, fileBytes(t, filepath.Join(destination, "package.json")), &manifest)
	dependencies := manifest["dependencies"].(map[string]any)
	dependencies["@sundw/locus-scope"] = locusVersion
	writeJSONFile(t, filepath.Join(destination, "package.json"), manifest)
	return destination
}

func currentPlatformPackage(t *testing.T) (string, string) {
	t.Helper()
	suffix := ""
	if runtime.GOOS == "windows" {
		suffix = ".exe"
	}
	var platform string
	switch runtime.GOOS + "/" + runtime.GOARCH {
	case "windows/amd64":
		platform = "win32-x64"
	case "linux/amd64":
		platform = "linux-x64"
	case "linux/arm64":
		platform = "linux-arm64"
	case "darwin/amd64":
		platform = "darwin-x64"
	case "darwin/arm64":
		platform = "darwin-arm64"
	default:
		t.Fatalf("current platform %s/%s is outside the published @sundw/locus-scope matrix", runtime.GOOS, runtime.GOARCH)
	}
	return "locus-scope-" + platform, "locus-scope-node-host" + suffix
}

func copyFile(source, destination string, mode os.FileMode) error {
	body, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	return os.WriteFile(destination, body, mode)
}

func assertSymlink(t *testing.T, path, description string) {
	t.Helper()
	information, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("inspect %s: %v", description, err)
	}
	if information.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("%s at %s is not a pnpm symlink/junction", description, path)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
