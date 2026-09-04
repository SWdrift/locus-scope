package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/distribution/distribution/v3/configuration"
	"github.com/distribution/distribution/v3/registry/handlers"
	_ "github.com/distribution/distribution/v3/registry/storage/driver/inmemory"
	"gopkg.in/yaml.v3"
	"locus-scope/internal/packages"
)

func TestPackageCLIClosure(t *testing.T) {
	configurationData := "version: 0.1\nlog:\n  level: error\nstorage:\n  inmemory: {}\n"
	registryConfiguration, err := configuration.Parse(strings.NewReader(configurationData))
	if err != nil {
		t.Fatalf("parse registry configuration: %v", err)
	}
	requests := &requestRecorder{next: handlers.NewApp(context.Background(), registryConfiguration)}
	server := httptest.NewServer(requests)
	runRoot := repositoryPath("temp", "e2e-run", "package")
	defer func() {
		server.Close()
		requests.write(t, filepath.Join(runRoot, "registry", "requests.log"))
	}()

	runPackageCLIClosure(t, server.URL, runRoot)
}

func TestZotPackageCLIClosure(t *testing.T) {
	endpoint := os.Getenv("LOCUS_TEST_REGISTRY")
	if endpoint == "" {
		t.Skip("LOCUS_TEST_REGISTRY is not set")
	}
	endpoint = requireLoopbackHTTP(t, endpoint)
	runPackageCLIClosure(t, endpoint, repositoryPath("temp", "e2e-run", "zot-package"))
}

func runPackageCLIClosure(t *testing.T, endpoint, runRoot string) {
	t.Helper()
	endpoint = requireLoopbackHTTP(t, endpoint)
	if err := os.RemoveAll(runRoot); err != nil {
		t.Fatalf("clear package E2E root: %v", err)
	}
	for _, directory := range []string{
		filepath.Join(runRoot, "bin"),
		filepath.Join(runRoot, "registry", "sources"),
		filepath.Join(runRoot, "results"),
		filepath.Join(runRoot, "docker"),
		filepath.Join(runRoot, "home"),
	} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatalf("create E2E directory: %v", err)
		}
	}
	if err := os.WriteFile(filepath.Join(runRoot, "docker", "config.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write isolated Docker config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runRoot, "registry", "endpoint.txt"), []byte(endpoint+"\n"), 0o644); err != nil {
		t.Fatalf("write registry endpoint: %v", err)
	}

	registryHost := strings.TrimPrefix(endpoint, "http://")
	fixtureRoot := repositoryPath("test", "e2e", "case", "package")
	project := filepath.Join(runRoot, "project")
	packageA := filepath.Join(runRoot, "registry", "sources", "package-a")
	packageB := filepath.Join(runRoot, "registry", "sources", "package-b")
	materializeSource(t, filepath.Join(fixtureRoot, "project"), project, registryHost)
	materializeSource(t, filepath.Join(fixtureRoot, "package-a"), packageA, registryHost)
	materializeSource(t, filepath.Join(fixtureRoot, "package-b"), packageB, registryHost)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	suffix := ""
	if runtime.GOOS == "windows" {
		suffix = ".exe"
	}
	pkgBinary := filepath.Join(runRoot, "bin", "locus-pkg"+suffix)
	scopeBinary := filepath.Join(runRoot, "bin", "locus-scope"+suffix)
	buildBinary(t, ctx, pkgBinary, "./cmd/locus-pkg")
	buildBinary(t, ctx, scopeBinary, "./cmd/locus-scope")
	environment := isolatedEnvironment(runRoot)

	targetA := "oci://" + registryHost + "/locus/package-a:latest"
	targetB := "oci://" + registryHost + "/locus/package-b:latest"
	publishWorkingDirectory := filepath.Join(packageB, ".locus", "publish-working-directory")
	if err := os.MkdirAll(publishWorkingDirectory, 0o755); err != nil {
		t.Fatalf("create nested publish working directory: %v", err)
	}
	firstBOutput := runBinaryAt(t, ctx, environment, publishWorkingDirectory, runRoot, "publish-b-first", pkgBinary, "--json", "publish", targetB)
	var firstB packages.PublishResult
	decodeJSON(t, firstBOutput, &firstB)
	if firstB.Target != targetB {
		t.Fatalf("first package B target = %q, want %q", firstB.Target, targetB)
	}
	repeatedBOutput := runBinaryAt(t, ctx, environment, publishWorkingDirectory, runRoot, "publish-b-repeat", pkgBinary, "publish", targetB)
	if !bytes.Contains(repeatedBOutput, []byte("published: "+targetB+"\n")) ||
		!bytes.Contains(repeatedBOutput, []byte("digest: "+firstB.Digest+"\n")) {
		t.Fatalf("unchanged package B output = %q", repeatedBOutput)
	}
	entitiesPath := filepath.Join(packageB, "entities.locus.yaml")
	entitiesData, err := os.ReadFile(entitiesPath)
	if err != nil {
		t.Fatalf("read package B entities: %v", err)
	}
	entitiesData = bytes.ReplaceAll(entitiesData, []byte("postgres"), []byte("cockroachdb"))
	if err := os.WriteFile(entitiesPath, entitiesData, 0o644); err != nil {
		t.Fatalf("update package B entities: %v", err)
	}
	secondBOutput := runBinary(t, ctx, environment, runRoot, "publish-b-update", pkgBinary, "--scope", packageB, "publish", targetB, "--json")
	var secondB packages.PublishResult
	decodeJSON(t, secondBOutput, &secondB)
	if secondB.Target != targetB || secondB.Digest == firstB.Digest {
		t.Fatalf("updated package B result = %#v, first digest = %q", secondB, firstB.Digest)
	}
	packageAOutput := runBinary(t, ctx, environment, runRoot, "publish-a", pkgBinary, "--scope", packageA, "publish", targetA, "--json")
	var publishedA packages.PublishResult
	decodeJSON(t, packageAOutput, &publishedA)
	if publishedA.Target != targetA {
		t.Fatalf("package A target = %q, want %q", publishedA.Target, targetA)
	}
	packageADigest := "oci://" + registryHost + "/locus/package-a@" + publishedA.Digest
	packageBDigest := "oci://" + registryHost + "/locus/package-b@" + secondB.Digest
	firstOutput := runBinary(t, ctx, environment, runRoot, "install-first", pkgBinary, "install", "--scope", project, "--json")
	var first packages.InstallResult
	decodeJSON(t, firstOutput, &first)
	assertInstallResult(t, first, 2, 0, 2, 2)

	lockPath := filepath.Join(project, "locus.lock")
	lockData, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatalf("read installed lock: %v", err)
	}
	var lock packages.Lock
	if err := yaml.Unmarshal(lockData, &lock); err != nil {
		t.Fatalf("decode installed lock: %v", err)
	}
	if lock.Version != 1 || len(lock.Packages) != 2 {
		t.Fatalf("installed lock = %#v\n%s", lock, lockData)
	}
	keyA := "oci://" + registryHost + "/locus/package-a:latest"
	keyB := "oci://" + registryHost + "/locus/package-b:latest"
	if lock.Packages[keyA].Resolved != packageADigest || lock.Packages[keyB].Resolved != packageBDigest {
		t.Fatalf("installed lock resolutions = %#v", lock.Packages)
	}
	materializedB := filepath.Join(project, ".locus", "packages", strings.Replace(secondB.Digest, ":", "-", 1), "entities.locus.yaml")
	materializedBData, err := os.ReadFile(materializedB)
	if err != nil {
		t.Fatalf("read updated materialized package B: %v", err)
	}
	if !bytes.Contains(materializedBData, []byte("cockroachdb")) {
		t.Fatalf("materialized package B did not contain the published update: %s", materializedBData)
	}
	if strings.Index(string(lockData), keyA) > strings.Index(string(lockData), keyB) {
		t.Fatalf("lock entries are not sorted:\n%s", lockData)
	}
	if err := os.WriteFile(filepath.Join(runRoot, "results", "locus.lock"), lockData, 0o644); err != nil {
		t.Fatalf("persist lock result: %v", err)
	}

	entries, err := os.ReadDir(filepath.Join(project, ".locus", "packages"))
	if err != nil {
		t.Fatalf("read materialized packages: %v", err)
	}
	directories := 0
	for _, entry := range entries {
		if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".extract-") {
			directories++
		}
	}
	if directories != 2 {
		t.Fatalf("materialized package directories = %d, want 2", directories)
	}

	secondOutput := runBinary(t, ctx, environment, runRoot, "install-second", pkgBinary, "--scope", project, "install", "--json")
	var second packages.InstallResult
	decodeJSON(t, secondOutput, &second)
	assertInstallResult(t, second, 0, 2, 0, 0)

	beforeInfo, err := os.Stat(lockPath)
	if err != nil {
		t.Fatalf("stat lock before frozen install: %v", err)
	}
	beforeData := append([]byte(nil), lockData...)
	frozenOutput := runBinary(t, ctx, environment, runRoot, "install-frozen", pkgBinary, "install", "--frozen", "--json", "--scope", project)
	var frozen packages.InstallResult
	decodeJSON(t, frozenOutput, &frozen)
	assertInstallResult(t, frozen, 0, 2, 0, 0)
	afterData, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatalf("read lock after frozen install: %v", err)
	}
	afterInfo, err := os.Stat(lockPath)
	if err != nil {
		t.Fatalf("stat lock after frozen install: %v", err)
	}
	if !bytes.Equal(beforeData, afterData) || !beforeInfo.ModTime().Equal(afterInfo.ModTime()) {
		t.Fatalf("frozen install changed lock bytes or timestamp")
	}

	validateOutput := runBinary(t, ctx, environment, runRoot, "scope-validate", scopeBinary, "--scope", project, "--json", "validate")
	var validation struct {
		Valid     bool `json:"valid"`
		Scopes    int  `json:"scopes"`
		Entities  int  `json:"entities"`
		Relations int  `json:"relations"`
	}
	decodeJSON(t, validateOutput, &validation)
	if !validation.Valid || validation.Scopes != 4 || validation.Entities != 2 || validation.Relations != 1 {
		t.Fatalf("validation output = %#v", validation)
	}

	resolveOutput := runBinary(t, ctx, environment, runRoot, "scope-resolve", scopeBinary, "--scope", project, "--json", "resolve", "a:sub:b:database")
	var resolution struct {
		Entity struct {
			Scope string `json:"scope"`
			ID    string `json:"id"`
		} `json:"entity"`
	}
	decodeJSON(t, resolveOutput, &resolution)
	if resolution.Entity.Scope != packageBDigest || resolution.Entity.ID != "database" {
		t.Fatalf("resolution output = %#v, want owner %q", resolution, packageBDigest)
	}
}

func materializeSource(t *testing.T, source, destination, registryHost string) {
	t.Helper()
	if err := os.CopyFS(destination, os.DirFS(source)); err != nil {
		t.Fatalf("copy source fixture %s: %v", source, err)
	}
	if err := filepath.WalkDir(destination, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		data = bytes.ReplaceAll(data, []byte("{{REGISTRY}}"), []byte(registryHost))
		return os.WriteFile(path, data, 0o644)
	}); err != nil {
		t.Fatalf("materialize source fixture %s: %v", source, err)
	}
}

func buildBinary(t *testing.T, ctx context.Context, output, packagePath string) {
	t.Helper()
	command := exec.CommandContext(ctx, "go", "build", "-o", output, packagePath)
	command.Dir = repositoryPath()
	if buildOutput, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build %s: %v\n%s", packagePath, err, buildOutput)
	}
}

func runBinary(t *testing.T, ctx context.Context, environment []string, runRoot, name, binary string, arguments ...string) []byte {
	t.Helper()
	return runBinaryAt(t, ctx, environment, repositoryPath(), runRoot, name, binary, arguments...)
}

func runBinaryAt(t *testing.T, ctx context.Context, environment []string, workingDirectory, runRoot, name, binary string, arguments ...string) []byte {
	t.Helper()
	command := exec.CommandContext(ctx, binary, arguments...)
	command.Dir = workingDirectory
	command.Env = environment
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	if writeErr := os.WriteFile(filepath.Join(runRoot, "results", name+".stdout"), stdout.Bytes(), 0o644); writeErr != nil {
		t.Fatalf("persist %s stdout: %v", name, writeErr)
	}
	if writeErr := os.WriteFile(filepath.Join(runRoot, "results", name+".stderr"), stderr.Bytes(), 0o644); writeErr != nil {
		t.Fatalf("persist %s stderr: %v", name, writeErr)
	}
	if err != nil {
		t.Fatalf("run %s: %v\nstdout:\n%s\nstderr:\n%s", name, err, stdout.Bytes(), stderr.Bytes())
	}
	return stdout.Bytes()
}

func isolatedEnvironment(runRoot string) []string {
	overrides := map[string]string{
		"HOME":           filepath.Join(runRoot, "home"),
		"USERPROFILE":    filepath.Join(runRoot, "home"),
		"DOCKER_CONFIG":  filepath.Join(runRoot, "docker"),
		"XDG_CACHE_HOME": filepath.Join(runRoot, "home", ".cache"),
		"APPDATA":        filepath.Join(runRoot, "home", "AppData", "Roaming"),
		"LOCALAPPDATA":   filepath.Join(runRoot, "home", "AppData", "Local"),
		"HTTP_PROXY":     "",
		"HTTPS_PROXY":    "",
		"ALL_PROXY":      "",
		"NO_PROXY":       "127.0.0.1,localhost,::1",
	}
	environment := make([]string, 0, len(os.Environ())+len(overrides))
	for _, value := range os.Environ() {
		name, _, _ := strings.Cut(value, "=")
		matched := false
		for override := range overrides {
			if strings.EqualFold(name, override) {
				matched = true
				break
			}
		}
		if !matched {
			environment = append(environment, value)
		}
	}
	for name, value := range overrides {
		environment = append(environment, name+"="+value)
	}
	return environment
}

func assertInstallResult(t *testing.T, result packages.InstallResult, resolved, reused, fetched, materialized int) {
	t.Helper()
	if !result.Valid || result.Resolved != resolved || result.Reused != reused || result.Fetched != fetched || result.Materialized != materialized || result.Scopes != 4 || result.Entities != 2 || result.Relations != 1 {
		t.Fatalf("install result = %#v", result)
	}
}

func decodeJSON(t *testing.T, data []byte, destination any) {
	t.Helper()
	if err := json.Unmarshal(data, destination); err != nil {
		t.Fatalf("decode JSON %q: %v", data, err)
	}
}

func requireLoopbackHTTP(t *testing.T, endpoint string) string {
	t.Helper()
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme != "http" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Path != "" && parsed.Path != "/" {
		t.Fatalf("registry endpoint must be a loopback http URL, got %q", endpoint)
	}
	host := parsed.Hostname()
	if !strings.EqualFold(host, "localhost") {
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			t.Fatalf("registry endpoint must be loopback, got %q", endpoint)
		}
	}
	return strings.TrimSuffix(endpoint, "/")
}

func repositoryPath(parts ...string) string {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		panic(err)
	}
	return filepath.Join(append([]string{root}, parts...)...)
}

type requestRecorder struct {
	mu    sync.Mutex
	next  http.Handler
	lines []string
}

func (r *requestRecorder) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	r.mu.Lock()
	r.lines = append(r.lines, request.Method+" "+request.URL.RequestURI())
	r.mu.Unlock()
	r.next.ServeHTTP(response, request)
}

func (r *requestRecorder) write(t *testing.T, path string) {
	t.Helper()
	r.mu.Lock()
	data := []byte(strings.Join(r.lines, "\n") + "\n")
	r.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Errorf("create request log directory: %v", err)
		return
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Errorf("write request log: %v", err)
	}
}
