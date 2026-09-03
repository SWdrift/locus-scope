package e2e_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/distribution/distribution/v3/configuration"
	"github.com/distribution/distribution/v3/registry/handlers"
	_ "github.com/distribution/distribution/v3/registry/storage/driver/inmemory"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"gopkg.in/yaml.v3"
	"locus-scope/internal/packages"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/retry"
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
		filepath.Join(runRoot, "registry", "payloads"),
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
	packageADigest := publishPackage(t, ctx, endpoint, "locus/package-a", packageA, filepath.Join(runRoot, "registry", "payloads", "package-a"))
	packageBDigest := publishPackage(t, ctx, endpoint, "locus/package-b", packageB, filepath.Join(runRoot, "registry", "payloads", "package-b"))

	suffix := ""
	if runtime.GOOS == "windows" {
		suffix = ".exe"
	}
	pkgBinary := filepath.Join(runRoot, "bin", "locus-pkg"+suffix)
	scopeBinary := filepath.Join(runRoot, "bin", "locus-scope"+suffix)
	buildBinary(t, ctx, pkgBinary, "./cmd/locus-pkg")
	buildBinary(t, ctx, scopeBinary, "./cmd/locus-scope")
	environment := isolatedEnvironment(runRoot)

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

func publishPackage(t *testing.T, ctx context.Context, endpoint, repositoryName, source, resultRoot string) string {
	t.Helper()
	parsed, err := url.Parse(endpoint)
	if err != nil {
		t.Fatalf("parse registry endpoint: %v", err)
	}
	repository, err := remote.NewRepository(parsed.Host + "/" + repositoryName)
	if err != nil {
		t.Fatalf("create repository %s: %v", repositoryName, err)
	}
	repository.PlainHTTP = true
	repository.Client = &auth.Client{Client: retry.DefaultClient, Cache: auth.NewCache()}

	layerData := packageLayer(t, source)
	if err := os.MkdirAll(resultRoot, 0o755); err != nil {
		t.Fatalf("create payload result directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(resultRoot, "layer.tar.gz"), layerData, 0o644); err != nil {
		t.Fatalf("persist package layer: %v", err)
	}
	layer, err := oras.PushBytes(ctx, repository, ocispec.MediaTypeImageLayerGzip, layerData)
	if err != nil {
		t.Fatalf("push %s layer: %v", repositoryName, err)
	}
	manifest, err := oras.PackManifest(ctx, repository, oras.PackManifestVersion1_1, packages.ArtifactType, oras.PackManifestOptions{
		Layers: []ocispec.Descriptor{layer},
	})
	if err != nil {
		t.Fatalf("pack %s manifest: %v", repositoryName, err)
	}
	if err := repository.Tag(ctx, manifest, "latest"); err != nil {
		t.Fatalf("tag %s manifest: %v", repositoryName, err)
	}
	reader, err := repository.Fetch(ctx, manifest)
	if err != nil {
		t.Fatalf("fetch %s manifest: %v", repositoryName, err)
	}
	manifestData, readErr := content.ReadAll(reader, manifest)
	closeErr := reader.Close()
	if readErr != nil {
		t.Fatalf("read %s manifest: %v", repositoryName, readErr)
	}
	if closeErr != nil {
		t.Fatalf("close %s manifest: %v", repositoryName, closeErr)
	}
	if err := os.WriteFile(filepath.Join(resultRoot, "manifest.json"), manifestData, 0o644); err != nil {
		t.Fatalf("persist package manifest: %v", err)
	}
	return "oci://" + parsed.Host + "/" + repositoryName + "@" + manifest.Digest.String()
}

func packageLayer(t *testing.T, source string) []byte {
	t.Helper()
	var payload bytes.Buffer
	gzipWriter := gzip.NewWriter(&payload)
	gzipWriter.Header.ModTime = time.Unix(0, 0)
	gzipWriter.Header.OS = 255
	tarWriter := tar.NewWriter(gzipWriter)
	var paths []string
	if err := filepath.WalkDir(source, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path != source {
			paths = append(paths, path)
		}
		return nil
	}); err != nil {
		t.Fatalf("walk package source: %v", err)
	}
	sort.Strings(paths)
	for _, filePath := range paths {
		info, err := os.Lstat(filePath)
		if err != nil {
			t.Fatalf("inspect package source: %v", err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			t.Fatalf("package source contains symlink: %s", filePath)
		}
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			t.Fatalf("create tar header: %v", err)
		}
		relative, err := filepath.Rel(source, filePath)
		if err != nil {
			t.Fatalf("make package path relative: %v", err)
		}
		header.Name = filepath.ToSlash(relative)
		if info.IsDir() {
			header.Name += "/"
		}
		header.ModTime = time.Unix(0, 0)
		header.AccessTime = time.Time{}
		header.ChangeTime = time.Time{}
		header.Uid, header.Gid = 0, 0
		header.Uname, header.Gname = "", ""
		if err := tarWriter.WriteHeader(header); err != nil {
			t.Fatalf("write package header: %v", err)
		}
		if info.Mode().IsRegular() {
			file, err := os.Open(filePath)
			if err != nil {
				t.Fatalf("open package source file: %v", err)
			}
			_, copyErr := io.Copy(tarWriter, file)
			closeErr := file.Close()
			if copyErr != nil {
				t.Fatalf("copy package source file: %v", copyErr)
			}
			if closeErr != nil {
				t.Fatalf("close package source file: %v", closeErr)
			}
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatalf("close package tar: %v", err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatalf("close package gzip: %v", err)
	}
	return payload.Bytes()
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
	command := exec.CommandContext(ctx, binary, arguments...)
	command.Dir = repositoryPath()
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
