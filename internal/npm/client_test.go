package npm

import (
	"context"
	"crypto/sha512"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func npmTestRoot(t *testing.T) string {
	t.Helper()
	base := filepath.Join("..", "..", "temp", "unit-npm")
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatalf("create test base: %v", err)
	}
	root, err := os.MkdirTemp(base, "case-")
	if err != nil {
		t.Fatalf("create test root: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	return root
}

func TestConfigPrecedenceScopeAndTokenSelection(t *testing.T) {
	root := npmTestRoot(t)
	userConfig := filepath.Join(root, "user.npmrc")
	if err := os.WriteFile(userConfig, []byte("registry=https://user.example/\n@example:registry=https://scope-user.example/\n//registry.example/:_authToken=user\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	project := "registry=https://project.example/\n@example:registry=https://scope-project.example/\n//registry.example/team/:_authToken=${PROJECT_TOKEN}\n"
	if err := os.WriteFile(filepath.Join(root, ".npmrc"), []byte(project), 0o644); err != nil {
		t.Fatal(err)
	}
	environment := map[string]string{
		"NPM_CONFIG_REGISTRY": "https://environment.example/",
		"PROJECT_TOKEN":       "project-secret",
		"NPM_TOKEN":           "fallback-secret",
	}
	config, err := LoadConfigWithOptions(ConfigOptions{
		Root:       root,
		Registry:   "http://127.0.0.1:4873/",
		UserConfig: userConfig,
		LookupEnvironment: func(name string) (string, bool) {
			value, ok := environment[name]
			return value, ok
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	plain, err := config.RegistryFor("plain")
	if err != nil || plain.String() != "http://127.0.0.1:4873/" {
		t.Fatalf("default registry = %v, %v", plain, err)
	}
	scoped, err := config.RegistryFor("@example/app")
	if err != nil || scoped.String() != "https://scope-project.example/" {
		t.Fatalf("scoped registry = %v, %v", scoped, err)
	}
	team, _ := parseRequestURL("https://registry.example/team/package")
	if got := config.tokenFor(team); got != "project-secret" {
		t.Fatalf("team token = %q", got)
	}
	other, _ := parseRequestURL("http://127.0.0.1:4873/package")
	if got := config.tokenFor(other); got != "fallback-secret" {
		t.Fatalf("fallback token = %q", got)
	}
}

func TestConfigRejectsInsecureRegistryAndPasswordCredentials(t *testing.T) {
	root := npmTestRoot(t)
	if _, err := LoadConfigWithOptions(ConfigOptions{Registry: "http://registry.example/", SkipUserConfig: true}); err == nil {
		t.Fatal("insecure non-loopback Registry accepted")
	}
	if err := os.WriteFile(filepath.Join(root, ".npmrc"), []byte("//registry.example/:_password=c2VjcmV0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfigWithOptions(ConfigOptions{Root: root, SkipUserConfig: true}); err == nil || strings.Contains(err.Error(), "c2VjcmV0") {
		t.Fatalf("password config error = %v", err)
	}
}

func TestRegistryRequestsEscapeNamesBoundResponsesAndRedactTokens(t *testing.T) {
	secret := "never-print-this"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.EscapedPath() != "/%40example%2Fapp" && request.URL.EscapedPath() != "/@example%2Fapp" {
			t.Errorf("request path = %q", request.URL.EscapedPath())
		}
		if got := request.Header.Get("Authorization"); got != "Bearer "+secret {
			t.Errorf("Authorization = %q", got)
		}
		writer.WriteHeader(http.StatusUnauthorized)
		_, _ = writer.Write([]byte(secret))
	}))
	defer server.Close()

	root := npmTestRoot(t)
	tokenKey := strings.TrimPrefix(server.URL, "http:") + "/:_authToken=" + secret + "\n"
	if err := os.WriteFile(filepath.Join(root, ".npmrc"), []byte(tokenKey), 0o644); err != nil {
		t.Fatal(err)
	}
	config, err := LoadConfigWithOptions(ConfigOptions{Root: root, Registry: server.URL, SkipUserConfig: true})
	if err != nil {
		t.Fatal(err)
	}
	_, err = NewClient(config, server.Client()).Packument(context.Background(), "@example/app")
	if err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("Registry error = %v", err)
	}
}

func TestDownloadVerifiesIntegrityAndDoesNotForwardAuthAcrossOrigin(t *testing.T) {
	payload := []byte("tgz bytes")
	digest := sha512.Sum512(payload)
	integrity := "sha512-" + base64.StdEncoding.EncodeToString(digest[:])
	authorization := ""
	target := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		authorization = request.Header.Get("Authorization")
		_, _ = writer.Write(payload)
	}))
	defer target.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, target.URL+"/archive.tgz", http.StatusFound)
	}))
	defer origin.Close()

	root := npmTestRoot(t)
	tokenKey := strings.TrimPrefix(origin.URL, "http:") + "/:_authToken=origin-secret\n"
	if err := os.WriteFile(filepath.Join(root, ".npmrc"), []byte(tokenKey), 0o644); err != nil {
		t.Fatal(err)
	}
	config, err := LoadConfigWithOptions(ConfigOptions{Root: root, Registry: origin.URL, SkipUserConfig: true})
	if err != nil {
		t.Fatal(err)
	}
	client := NewClient(config, origin.Client())
	got, err := client.Download(context.Background(), origin.URL+"/archive.tgz", integrity)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(payload) || authorization != "" {
		t.Fatalf("download = %q, redirected Authorization = %q", got, authorization)
	}
	if _, err := client.Download(context.Background(), origin.URL+"/archive.tgz", fmt.Sprintf("sha512-%s", base64.StdEncoding.EncodeToString(make([]byte, sha512.Size)))); err == nil {
		t.Fatal("integrity mismatch accepted")
	}
}
func TestRedirectRejectsInsecureNonLoopbackTarget(t *testing.T) {
	config, err := LoadConfigWithOptions(ConfigOptions{Registry: "https://registry.example/", SkipUserConfig: true})
	if err != nil {
		t.Fatal(err)
	}
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusFound,
			Header:     http.Header{"Location": []string{"http://packages.example/archive.tgz"}},
			Body:       io.NopCloser(strings.NewReader("")),
			Request:    request,
		}, nil
	})
	client := NewClient(config, &http.Client{Transport: transport})
	if _, err := client.Packument(context.Background(), "example"); err == nil || !strings.Contains(err.Error(), "must use HTTPS") {
		t.Fatalf("insecure redirect error = %v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}
