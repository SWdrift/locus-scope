package npm

import (
	"context"
	"crypto/sha512"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"deps.dev/util/resolve"
)

func TestResolverClientUsesRegistryPackumentAndNPMOrdering(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests++
		digest := sha512.Sum512(nil)
		integrity := "sha512-" + base64.StdEncoding.EncodeToString(digest[:])
		writer.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(writer, `{"name":"app","versions":{"2.0.0":{"name":"app","version":"2.0.0","dist":{"tarball":%q,"integrity":%q}},"1.5.0":{"name":"app","version":"1.5.0","dependencies":{"base":"^1.0.0"},"dist":{"tarball":%q,"integrity":%q}},"1.0.0":{"name":"app","version":"1.0.0","dist":{"tarball":%q,"integrity":%q}}}}`, serverURL(request)+"/app-2.tgz", integrity, serverURL(request)+"/app-1.5.tgz", integrity, serverURL(request)+"/app-1.tgz", integrity)
	}))
	defer server.Close()
	config, err := LoadConfigWithOptions(ConfigOptions{Registry: server.URL, SkipUserConfig: true})
	if err != nil {
		t.Fatal(err)
	}
	client := NewResolverClient(NewClient(config, server.Client()))
	packageKey := resolve.PackageKey{System: resolve.NPM, Name: "app"}
	versions, err := client.Versions(context.Background(), packageKey)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 3 || versions[0].Version != "1.0.0" || versions[2].Version != "2.0.0" {
		t.Fatalf("versions = %#v", versions)
	}
	matching, err := client.MatchingVersions(context.Background(), resolve.VersionKey{PackageKey: packageKey, VersionType: resolve.Requirement, Version: "^1.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	if len(matching) != 2 || matching[0].Version != "1.0.0" || matching[1].Version != "1.5.0" {
		t.Fatalf("matching versions = %#v", matching)
	}
	requirements, err := client.Requirements(context.Background(), resolve.VersionKey{PackageKey: packageKey, VersionType: resolve.Concrete, Version: "1.5.0"})
	if err != nil {
		t.Fatal(err)
	}
	if len(requirements) != 1 || requirements[0].Name != "base" || requirements[0].Version != "^1.0.0" {
		t.Fatalf("requirements = %#v", requirements)
	}
	if requests != 1 {
		t.Fatalf("packument requests = %d, want 1", requests)
	}
}

func serverURL(request *http.Request) string {
	return "http://" + request.Host
}
