package e2e_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha1"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

type commandResult struct {
	stdout []byte
	stderr []byte
	err    error
}

func (r commandResult) success(t *testing.T, name string) []byte {
	t.Helper()
	if r.err != nil {
		t.Fatalf("%s failed: %v\nstdout:\n%s\nstderr:\n%s", name, r.err, r.stdout, r.stderr)
	}
	return r.stdout
}

func (r commandResult) failure(t *testing.T, name string) {
	t.Helper()
	if r.err == nil {
		t.Fatalf("%s unexpectedly succeeded\nstdout:\n%s\nstderr:\n%s", name, r.stdout, r.stderr)
	}
}

type verdaccioProcess struct {
	cmd      *exec.Cmd
	endpoint string
	config   string
	pidFile  string
	logFile  *os.File
	stopped  bool
}

type npmLifecycleHarness struct {
	t        *testing.T
	ctx      context.Context
	root     string
	results  string
	endpoint string
	node     string
	npm      string
	pnpm     string
	pkg      string
	scope    string
	host     string
	token    string
	authEnv  []string
	anonEnv  []string
	wrongEnv []string
	registry *verdaccioProcess
}

func newNPMLifecycleHarness(t *testing.T) *npmLifecycleHarness {
	t.Helper()
	root := repositoryPath("temp", "e2e-run", "npm")
	if err := os.RemoveAll(root); err != nil {
		t.Fatalf("clear npm E2E state: %v", err)
	}
	for _, directory := range []string{
		filepath.Join(root, "bin"), filepath.Join(root, "fixtures"), filepath.Join(root, "projects"),
		filepath.Join(root, "registry", "storage"), filepath.Join(root, "registry", "auth"),
		filepath.Join(root, "registry", "home"), filepath.Join(root, "results"), filepath.Join(root, "config"),
	} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatalf("create %s: %v", directory, err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "config", "global.npmrc"), nil, 0o600); err != nil {
		t.Fatalf("write isolated global npm config: %v", err)
	}

	requireTool := func(name string) string {
		path, err := exec.LookPath(name)
		if err != nil {
			t.Fatalf("required external prerequisite %s is unavailable: %v", name, err)
		}
		absolute, err := filepath.Abs(path)
		if err != nil {
			t.Fatalf("resolve %s: %v", name, err)
		}
		return absolute
	}
	node := requireTool("node")
	npm := requireTool("npm")
	pnpm := requireTool("pnpm")
	verdaccio := repositoryPath("node_modules", "verdaccio", "bin", "verdaccio")
	if information, err := os.Stat(verdaccio); err != nil || information.IsDir() {
		t.Fatalf("root-installed Verdaccio 6.10.2 binary is unavailable at %s", verdaccio)
	}
	var verdaccioManifest struct {
		Version string `json:"version"`
	}
	decodeJSON(t, fileBytes(t, repositoryPath("node_modules", "verdaccio", "package.json")), &verdaccioManifest)
	if verdaccioManifest.Version != "6.10.2" {
		t.Fatalf("root-installed Verdaccio version = %q, want 6.10.2", verdaccioManifest.Version)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	t.Cleanup(cancel)
	suffix := ""
	if runtime.GOOS == "windows" {
		suffix = ".exe"
	}
	h := &npmLifecycleHarness{
		t: t, ctx: ctx, root: root, results: filepath.Join(root, "results"),
		node: node, npm: npm, pnpm: pnpm,
		pkg:   filepath.Join(root, "bin", "locus-pkg"+suffix),
		scope: filepath.Join(root, "bin", "locus-scope"+suffix),
		host:  filepath.Join(root, "bin", "locus-scope-node-host"+suffix),
	}
	h.build("locus-pkg", h.pkg, "./cmd/locus-pkg")
	h.build("locus-scope", h.scope, "./cmd/locus-scope")
	h.build("locus-scope-node-host", h.host, "./cmd/locus-scope-node-host")
	h.registry = startVerdaccio(t, ctx, root, node, verdaccio)
	h.endpoint = h.registry.endpoint
	t.Cleanup(func() { h.registry.stop(t) })

	h.token = createVerdaccioUser(t, h.endpoint, "locus-e2e", "locus-e2e-password")
	authConfig := filepath.Join(root, "auth", "npmrc")
	anonymousConfig := filepath.Join(root, "anonymous", "npmrc")
	wrongConfig := filepath.Join(root, "wrong-token", "npmrc")
	writeNPMConfig(t, authConfig, h.endpoint, h.token)
	writeNPMConfig(t, anonymousConfig, h.endpoint, "")
	writeNPMConfig(t, wrongConfig, h.endpoint, "not-the-e2e-token")
	h.authEnv = packageManagerEnvironment(root, "auth", authConfig, h.endpoint)
	h.anonEnv = packageManagerEnvironment(root, "anonymous", anonymousConfig, h.endpoint)
	h.wrongEnv = packageManagerEnvironment(root, "wrong-token", wrongConfig, h.endpoint)
	return h
}

func (h *npmLifecycleHarness) build(name, output, packagePath string) {
	h.t.Helper()
	command := exec.CommandContext(h.ctx, "go", "build", "-o", output, packagePath)
	command.Dir = repositoryPath()
	command.Env = packageManagerEnvironment(h.root, "build", "", "")
	combined, err := command.CombinedOutput()
	if err != nil {
		h.t.Fatalf("build %s: %v\n%s", packagePath, err, combined)
	}
	if err := os.WriteFile(filepath.Join(h.results, "build-"+name+".log"), combined, 0o644); err != nil {
		h.t.Fatalf("persist %s build output: %v", name, err)
	}
}

func (h *npmLifecycleHarness) run(name, directory string, environment []string, executable string, arguments ...string) commandResult {
	h.t.Helper()
	command := executableCommand(h.ctx, executable, arguments...)
	command.Dir = directory
	command.Env = environment
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	for extension, data := range map[string][]byte{"stdout": stdout.Bytes(), "stderr": stderr.Bytes()} {
		if writeErr := os.WriteFile(filepath.Join(h.results, name+"."+extension), data, 0o644); writeErr != nil {
			h.t.Fatalf("persist %s %s: %v", name, extension, writeErr)
		}
	}
	exit := 0
	if err != nil {
		exit = -1
		if exitError, ok := err.(*exec.ExitError); ok {
			exit = exitError.ExitCode()
		}
	}
	if writeErr := os.WriteFile(filepath.Join(h.results, name+".exit"), []byte(strconv.Itoa(exit)+"\n"), 0o644); writeErr != nil {
		h.t.Fatalf("persist %s exit code: %v", name, writeErr)
	}
	return commandResult{stdout: stdout.Bytes(), stderr: stderr.Bytes(), err: err}
}

func (h *npmLifecycleHarness) pure(name, directory string, environment []string, executable string, arguments ...string) commandResult {
	h.t.Helper()
	return h.run(name, directory, pureEnvironment(environment), executable, arguments...)
}

func (h *npmLifecycleHarness) npmPublish(name, directory string, environment []string, options ...string) commandResult {
	h.t.Helper()
	arguments := []string{"publish", ".", "--access", "public", "--ignore-scripts", "--registry", h.endpoint}
	arguments = append(arguments, options...)
	return h.run(name, directory, environment, h.npm, arguments...)
}

func (h *npmLifecycleHarness) npmInstall(name, directory string, environment []string, specs ...string) commandResult {
	h.t.Helper()
	arguments := append([]string{"install", "--ignore-scripts", "--no-audit", "--no-fund", "--registry", h.endpoint}, specs...)
	return h.run(name, directory, environment, h.npm, arguments...)
}

func (h *npmLifecycleHarness) pnpmInstall(name, directory string, environment []string, specs ...string) commandResult {
	h.t.Helper()
	workspace := "packages:\n  - \".\"\nminimumReleaseAge: 0\n"
	if err := os.WriteFile(filepath.Join(directory, "pnpm-workspace.yaml"), []byte(workspace), 0o644); err != nil {
		h.t.Fatalf("write isolated pnpm workspace: %v", err)
	}
	arguments := append([]string{"install", "--ignore-scripts", "--registry", h.endpoint}, specs...)
	return h.run(name, directory, environment, h.pnpm, arguments...)
}

func startVerdaccio(t *testing.T, ctx context.Context, root, node, verdaccio string) *verdaccioProcess {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve Verdaccio port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatalf("release Verdaccio port: %v", err)
	}
	endpoint := fmt.Sprintf("http://127.0.0.1:%d", port)
	registryRoot := filepath.Join(root, "registry")
	config := filepath.Join(registryRoot, "config.yaml")
	logPath := filepath.Join(registryRoot, "verdaccio.log")
	configBody := fmt.Sprintf(`storage: %s
auth:
  htpasswd:
    file: %s
    max_users: 10
uplinks: {}
packages:
  '@example/private':
    access: $authenticated
    publish: $authenticated
    unpublish: $authenticated
  '**':
    access: $all
    publish: $authenticated
    unpublish: $authenticated
log:
  type: stdout
  format: pretty
  level: http
`, yamlString(filepath.Join(registryRoot, "storage")), yamlString(filepath.Join(registryRoot, "auth", "htpasswd")))
	if err := os.WriteFile(config, []byte(configBody), 0o644); err != nil {
		t.Fatalf("write Verdaccio config: %v", err)
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		t.Fatalf("open Verdaccio log: %v", err)
	}
	command := exec.CommandContext(ctx, node, verdaccio, "--config", config, "--listen", fmt.Sprintf("127.0.0.1:%d", port))
	command.Dir = repositoryPath()
	command.Env = packageManagerEnvironment(root, "registry", "", endpoint)
	command.Stdout = logFile
	command.Stderr = logFile
	if err := command.Start(); err != nil {
		logFile.Close()
		t.Fatalf("start Verdaccio: %v", err)
	}
	pidFile := filepath.Join(registryRoot, "verdaccio.pid")
	if err := os.WriteFile(pidFile, []byte(strconv.Itoa(command.Process.Pid)+"\n"), 0o644); err != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		logFile.Close()
		t.Fatalf("write Verdaccio pid: %v", err)
	}
	registry := &verdaccioProcess{cmd: command, endpoint: endpoint, config: config, pidFile: pidFile, logFile: logFile}
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		response, requestErr := loopbackClient().Get(endpoint + "/-/ping")
		if requestErr == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				if err := os.WriteFile(filepath.Join(registryRoot, "endpoint.txt"), []byte(endpoint+"\n"), 0o644); err != nil {
					t.Fatalf("persist Verdaccio endpoint: %v", err)
				}
				return registry
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	registry.stop(t)
	logData, _ := os.ReadFile(logPath)
	t.Fatalf("Verdaccio did not become ready at %s\n%s", endpoint, logData)
	return nil
}

func (p *verdaccioProcess) stop(t *testing.T) {
	t.Helper()
	if p == nil || p.stopped {
		return
	}
	p.stopped = true
	pidData, err := os.ReadFile(p.pidFile)
	if err != nil {
		t.Errorf("verify owned Verdaccio pid file: %v", err)
		return
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(pidData)))
	if err != nil || p.cmd.Process == nil || pid != p.cmd.Process.Pid || p.cmd.ProcessState != nil {
		t.Errorf("refuse to stop unverified Verdaccio child: recorded=%q process=%v state=%v", pidData, p.cmd.Process, p.cmd.ProcessState)
		return
	}
	if _, err := os.Stat(p.config); err != nil {
		t.Errorf("refuse to stop Verdaccio without owned config %s: %v", p.config, err)
		return
	}
	response, err := loopbackClient().Get(p.endpoint + "/-/ping")
	if err != nil || response.StatusCode != http.StatusOK {
		if response != nil {
			_ = response.Body.Close()
		}
		t.Errorf("refuse to stop Verdaccio without endpoint ownership proof at %s: %v", p.endpoint, err)
		return
	}
	_ = response.Body.Close()
	if runtime.GOOS == "windows" {
		err = p.cmd.Process.Kill()
	} else {
		err = p.cmd.Process.Signal(os.Interrupt)
	}
	if err != nil {
		t.Errorf("stop owned Verdaccio child: %v", err)
		return
	}
	done := make(chan error, 1)
	go func() { done <- p.cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		_ = p.cmd.Process.Kill()
		<-done
	}
	if err := p.logFile.Close(); err != nil {
		t.Errorf("close Verdaccio log: %v", err)
	}
	marker := fmt.Sprintf("pid=%d\nendpoint=%s\nconfig=%s\n", pid, p.endpoint, p.config)
	if err := os.WriteFile(filepath.Join(filepath.Dir(p.pidFile), "stopped.txt"), []byte(marker), 0o644); err != nil {
		t.Errorf("persist Verdaccio stop evidence: %v", err)
	}
}

func createVerdaccioUser(t *testing.T, endpoint, username, password string) string {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"_id": "org.couchdb.user:" + username, "name": username, "password": password,
		"type": "user", "roles": []string{}, "date": time.Unix(0, 0).UTC().Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("encode Verdaccio user: %v", err)
	}
	request, err := http.NewRequest(http.MethodPut, endpoint+"/-/user/org.couchdb.user:"+url.PathEscape(username), bytes.NewReader(body))
	if err != nil {
		t.Fatalf("create Verdaccio user request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := loopbackClient().Do(request)
	if err != nil {
		t.Fatalf("create Verdaccio user: %v", err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read Verdaccio user response: %v", err)
	}
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("create Verdaccio user status = %d, body = %s", response.StatusCode, responseBody)
	}
	var result struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(responseBody, &result); err != nil || result.Token == "" {
		t.Fatalf("decode Verdaccio user token: %v", err)
	}
	return result.Token
}

func writeNPMConfig(t *testing.T, path, endpoint, token string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create npm config directory: %v", err)
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		t.Fatalf("parse registry endpoint: %v", err)
	}
	body := "registry=" + endpoint + "/\n"
	if token != "" {
		body += "//" + parsed.Host + "/:_authToken=" + token + "\n"
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write isolated npm config: %v", err)
	}
}

func packageManagerEnvironment(root, name, userConfig, endpoint string) []string {
	overrides := map[string]string{
		"HOME": filepath.Join(root, "home", name), "USERPROFILE": filepath.Join(root, "home", name),
		"HTTP_PROXY": "", "HTTPS_PROXY": "", "ALL_PROXY": "", "NO_PROXY": "127.0.0.1,localhost,::1",
		"NODE_AUTH_TOKEN": "", "YARN_NPM_AUTH_TOKEN": "",
		"NPM_CONFIG_GLOBALCONFIG": filepath.Join(root, "config", "global.npmrc"),
	}
	if userConfig != "" {
		overrides["NPM_CONFIG_USERCONFIG"] = userConfig
	}
	if endpoint != "" {
		overrides["NPM_CONFIG_REGISTRY"] = endpoint + "/"
	}
	environment := make([]string, 0, len(os.Environ())+len(overrides))
	for _, value := range os.Environ() {
		key, _, _ := strings.Cut(value, "=")
		upper := strings.ToUpper(key)
		if upper == "HOME" || upper == "USERPROFILE" || upper == "NPM_TOKEN" || upper == "NODE_AUTH_TOKEN" || strings.HasPrefix(upper, "YARN_") || strings.HasPrefix(upper, "NPM_CONFIG_") || strings.HasPrefix(upper, "PNPM_") || upper == "HTTP_PROXY" || upper == "HTTPS_PROXY" || upper == "ALL_PROXY" || upper == "NO_PROXY" {
			continue
		}
		environment = append(environment, value)
	}
	keys := make([]string, 0, len(overrides))
	for key := range overrides {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		environment = append(environment, key+"="+overrides[key])
	}
	return environment
}

func pureEnvironment(environment []string) []string {
	purePath := "/usr/bin:/bin"
	if runtime.GOOS == "windows" {
		windows := os.Getenv("SystemRoot")
		purePath = strings.Join([]string{filepath.Join(windows, "System32"), windows, filepath.Join(windows, "System32", "Wbem")}, string(os.PathListSeparator))
	}
	result := make([]string, 0, len(environment)+1)
	for _, value := range environment {
		key, _, _ := strings.Cut(value, "=")
		if !strings.EqualFold(key, "PATH") {
			result = append(result, value)
		}
	}
	return append(result, "PATH="+purePath)
}

func executableCommand(ctx context.Context, executable string, arguments ...string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		extension := strings.ToLower(filepath.Ext(executable))
		if extension == ".cmd" || extension == ".bat" {
			commandShell := os.Getenv("ComSpec")
			if commandShell == "" {
				commandShell = filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe")
			}
			shellArguments := append([]string{"/d", "/s", "/c", "call", executable}, arguments...)
			return exec.CommandContext(ctx, commandShell, shellArguments...)
		}
	}
	return exec.CommandContext(ctx, executable, arguments...)
}

func materializeFixture(t *testing.T, source, destination string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		t.Fatalf("create fixture parent: %v", err)
	}
	if err := os.CopyFS(destination, os.DirFS(source)); err != nil {
		t.Fatalf("materialize fixture %s: %v", source, err)
	}
}

func writeJSONFile(t *testing.T, path string, value any) {
	t.Helper()
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("encode %s: %v", path, err)
	}
	body = append(body, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create parent for %s: %v", path, err)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func decodeJSON(t *testing.T, body []byte, destination any) {
	t.Helper()
	if err := json.Unmarshal(body, destination); err != nil {
		t.Fatalf("decode JSON %q: %v", body, err)
	}
}

func fetchJSON(t *testing.T, endpoint, token string, destination any) []byte {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		t.Fatalf("create GET %s: %v", endpoint, err)
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := loopbackClient().Do(request)
	if err != nil {
		t.Fatalf("GET %s: %v", endpoint, err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read GET %s: %v", endpoint, err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET %s status = %d, body = %s", endpoint, response.StatusCode, body)
	}
	if destination != nil {
		decodeJSON(t, body, destination)
	}
	return body
}

func fetchBytes(t *testing.T, endpoint string) []byte {
	t.Helper()
	response, err := loopbackClient().Get(endpoint)
	if err != nil {
		t.Fatalf("download %s: %v", endpoint, err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read download %s: %v", endpoint, err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("download %s status = %d, body = %s", endpoint, response.StatusCode, body)
	}
	return body
}

func loopbackClient() *http.Client {
	return &http.Client{Transport: &http.Transport{Proxy: nil}, Timeout: 30 * time.Second}
}

func verifyIntegrity(t *testing.T, integrity string, body []byte) {
	t.Helper()
	algorithm, encoded, ok := strings.Cut(integrity, "-")
	if !ok {
		t.Fatalf("invalid SRI %q", integrity)
	}
	digest, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("decode SRI %q: %v", integrity, err)
	}
	var actual []byte
	switch algorithm {
	case "sha512":
		sum := sha512.Sum512(body)
		actual = sum[:]
	default:
		t.Fatalf("unexpected SRI algorithm %q", algorithm)
	}
	if !bytes.Equal(actual, digest) {
		t.Fatalf("tarball does not match %s", integrity)
	}
}

func tarFileNames(t *testing.T, archive []byte) []string {
	t.Helper()
	gzipReader, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		t.Fatalf("open tgz: %v", err)
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
			t.Fatalf("read tgz: %v", err)
		}
		names = append(names, header.Name)
	}
	return names
}

func writeLocusPackage(t *testing.T, root, name, version string, dependencies map[string]string, exports map[string]string, extraScope bool) {
	t.Helper()
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("create package %s: %v", root, err)
	}
	manifest := map[string]any{
		"name": name, "version": version,
		"files":   []string{"package.json", "locus.yaml", "entities.locus.yaml"},
		"locus":   map[string]string{"entry": "locus.yaml"},
		"exports": exports,
	}
	if len(dependencies) != 0 {
		manifest["dependencies"] = dependencies
	}
	if extraScope {
		manifest["files"] = append(manifest["files"].([]string), "locus.json")
	}
	writeJSONFile(t, filepath.Join(root, "package.json"), manifest)
	id := strings.TrimPrefix(strings.ReplaceAll(name, "/", "-"), "@")
	if err := os.WriteFile(filepath.Join(root, "locus.yaml"), []byte("id: "+id+"\nexports:\n  - value\n"), 0o644); err != nil {
		t.Fatalf("write locus manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "entities.locus.yaml"), []byte("entities:\n  - id: value\n"), 0o644); err != nil {
		t.Fatalf("write locus entities: %v", err)
	}
	if extraScope {
		if err := os.WriteFile(filepath.Join(root, "locus.json"), []byte("{\"id\":\"duplicate\"}\n"), 0o644); err != nil {
			t.Fatalf("write duplicate Scope manifest: %v", err)
		}
	}
}

func makeUnsafeTarball(t *testing.T, name, version string) ([]byte, map[string]any) {
	t.Helper()
	manifest := map[string]any{
		"name": name, "version": version, "files": []string{"package.json", "locus.yaml", "entities.locus.yaml"},
		"locus":   map[string]string{"entry": "locus.yaml"},
		"exports": map[string]string{"./package.json": "./package.json"},
	}
	packageJSON, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("encode unsafe package manifest: %v", err)
	}
	var body bytes.Buffer
	gzipWriter := gzip.NewWriter(&body)
	tarWriter := tar.NewWriter(gzipWriter)
	entries := []struct {
		name string
		data []byte
	}{
		{"package/package.json", packageJSON},
		{"package/locus.yaml", []byte("id: unsafe\nexports:\n  - value\n")},
		{"package/entities.locus.yaml", []byte("entities:\n  - id: value\n")},
		{"package/../escaped", []byte("must not escape")},
	}
	for _, entry := range entries {
		header := &tar.Header{Name: entry.name, Mode: 0o644, Size: int64(len(entry.data)), ModTime: time.Unix(0, 0)}
		if err := tarWriter.WriteHeader(header); err != nil {
			t.Fatalf("write unsafe tar header: %v", err)
		}
		if _, err := tarWriter.Write(entry.data); err != nil {
			t.Fatalf("write unsafe tar body: %v", err)
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatalf("close unsafe tar: %v", err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatalf("close unsafe gzip: %v", err)
	}
	return body.Bytes(), manifest
}

func publishRawPackage(t *testing.T, endpoint, token, name, version string, archive []byte, manifest map[string]any) {
	t.Helper()
	filename := strings.TrimPrefix(strings.ReplaceAll(name, "/", "-"), "@") + "-" + version + ".tgz"
	sha1Digest := sha1.Sum(archive)
	sha512Digest := sha512.Sum512(archive)
	integrity := "sha512-" + base64.StdEncoding.EncodeToString(sha512Digest[:])
	tarballURL := endpoint + "/" + name + "/-/" + filename
	versionMetadata := make(map[string]any, len(manifest)+1)
	for key, value := range manifest {
		versionMetadata[key] = value
	}
	versionMetadata["_id"] = name + "@" + version
	versionMetadata["dist"] = map[string]any{"tarball": tarballURL, "shasum": hex.EncodeToString(sha1Digest[:]), "integrity": integrity}
	body, err := json.Marshal(map[string]any{
		"_id": name, "name": name, "dist-tags": map[string]string{"latest": version},
		"versions": map[string]any{version: versionMetadata},
		"_attachments": map[string]any{filename: map[string]any{
			"content_type": "application/octet-stream", "data": base64.StdEncoding.EncodeToString(archive), "length": len(archive),
		}},
	})
	if err != nil {
		t.Fatalf("encode raw npm publish: %v", err)
	}
	request, err := http.NewRequest(http.MethodPut, endpoint+"/"+strings.ReplaceAll(name, "/", "%2f"), bytes.NewReader(body))
	if err != nil {
		t.Fatalf("create raw npm publish request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := loopbackClient().Do(request)
	if err != nil {
		t.Fatalf("publish raw package: %v", err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read raw publish response: %v", err)
	}
	if response.StatusCode != http.StatusCreated && response.StatusCode != http.StatusOK {
		t.Fatalf("raw publish status = %d, body = %s", response.StatusCode, responseBody)
	}
}

func integrityMismatchRegistry(t *testing.T, upstream, packageName, version string) *httptest.Server {
	t.Helper()
	upstreamURL, err := url.Parse(upstream)
	if err != nil {
		t.Fatalf("parse upstream registry: %v", err)
	}
	return httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		forward := request.Clone(request.Context())
		forward.RequestURI = ""
		forward.URL.Scheme = upstreamURL.Scheme
		forward.URL.Host = upstreamURL.Host
		upstreamResponse, err := loopbackClient().Transport.RoundTrip(forward)
		if err != nil {
			http.Error(response, err.Error(), http.StatusBadGateway)
			return
		}
		defer upstreamResponse.Body.Close()
		body, err := io.ReadAll(upstreamResponse.Body)
		if err != nil {
			http.Error(response, err.Error(), http.StatusBadGateway)
			return
		}
		if upstreamResponse.StatusCode == http.StatusOK && strings.Contains(strings.ToLower(request.URL.Path), strings.ToLower(packageName)) {
			var document map[string]any
			if json.Unmarshal(body, &document) == nil {
				var metadata map[string]any
				if versions, ok := document["versions"].(map[string]any); ok {
					metadata, _ = versions[version].(map[string]any)
				} else if documentVersion, _ := document["version"].(string); documentVersion == version {
					metadata = document
				}
				if dist, ok := metadata["dist"].(map[string]any); ok {
					dist["integrity"] = "sha512-" + base64.StdEncoding.EncodeToString(make([]byte, sha512.Size))
					body, _ = json.Marshal(document)
				}
			}
		}
		for key, values := range upstreamResponse.Header {
			for _, value := range values {
				response.Header().Add(key, value)
			}
		}
		response.Header().Del("Content-Length")
		response.WriteHeader(upstreamResponse.StatusCode)
		_, _ = response.Write(body)
	}))
}

func fileBytes(t *testing.T, path string) []byte {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return body
}

func directoryNames(t *testing.T, path string) []string {
	t.Helper()
	entries, err := os.ReadDir(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("read directory %s: %v", path, err)
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	return names
}

func assertNoSecret(t *testing.T, secret string, roots ...string) {
	t.Helper()
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				if entry.Name() == "node_modules" {
					return fs.SkipDir
				}
				return nil
			}
			if entry.Type()&os.ModeSymlink != 0 {
				return nil
			}
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if bytes.Contains(body, []byte(secret)) {
				return fmt.Errorf("credential leaked to %s", path)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func yamlString(value string) string {
	encoded, _ := json.Marshal(filepath.ToSlash(value))
	return string(encoded)
}

func repositoryPath(parts ...string) string {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		panic(err)
	}
	return filepath.Join(append([]string{root}, parts...)...)
}
