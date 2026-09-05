package npm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const RequestTimeout = 5 * time.Minute

var ErrImmutableVersion = errors.New("npm package version is immutable")

// Distribution is the Registry location and integrity of one package version.
type Distribution struct {
	Tarball   string `json:"tarball"`
	Integrity string `json:"integrity"`
	Shasum    string `json:"shasum,omitempty"`
}

// VersionMetadata is the Registry metadata needed for package resolution.
type VersionMetadata struct {
	Name                 string            `json:"name"`
	Version              string            `json:"version"`
	Dependencies         map[string]string `json:"dependencies,omitempty"`
	DevDependencies      map[string]string `json:"devDependencies,omitempty"`
	PeerDependencies     map[string]string `json:"peerDependencies,omitempty"`
	OptionalDependencies map[string]string `json:"optionalDependencies,omitempty"`
	Dist                 Distribution      `json:"dist"`
}

// Packument is the focused npm Registry package metadata document.
type Packument struct {
	Name     string                     `json:"name"`
	DistTags map[string]string          `json:"dist-tags"`
	Versions map[string]VersionMetadata `json:"versions"`
}

// HTTPError reports a failed Registry operation without response bodies,
// credentials, or query strings.
type HTTPError struct {
	Method     string
	URL        string
	StatusCode int
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("npm Registry %s %s returned HTTP %d", e.Method, e.URL, e.StatusCode)
}

// Client is a bounded npm Registry HTTP client.
type Client struct {
	config *Config
	http   *http.Client
}

// NewClient creates a Registry client. A supplied HTTP client is copied before
// redirect policy is installed, so later caller mutation cannot weaken it.
func NewClient(config *Config, provided *http.Client) *Client {
	if config == nil {
		panic("npm.NewClient: nil Config")
	}
	base := http.DefaultClient
	if provided != nil {
		base = provided
	}
	copy := *base
	previousRedirectPolicy := copy.CheckRedirect
	copy.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if err := validateRequestURL(request.URL); err != nil {
			return err
		}
		if previousRedirectPolicy != nil {
			if err := previousRedirectPolicy(request, via); err != nil {
				return err
			}
		} else if len(via) >= 10 {
			return fmt.Errorf("stopped after 10 redirects")
		}
		request.Header.Del("Authorization")
		return nil
	}
	return &Client{config: config, http: &copy}
}

// RegistryFor returns the Registry selected for name.
func (c *Client) RegistryFor(name string) (*url.URL, error) {
	return c.config.RegistryFor(name)
}

// Packument fetches a bounded package metadata document.
func (c *Client) Packument(ctx context.Context, name string) (Packument, error) {
	registry, err := c.config.RegistryFor(name)
	if err != nil {
		return Packument{}, err
	}
	endpoint, err := packageEndpoint(registry, name)
	if err != nil {
		return Packument{}, err
	}
	body, err := c.get(ctx, endpoint, MaxPackumentSize, "application/vnd.npm.install-v1+json, application/json")
	if err != nil {
		return Packument{}, err
	}
	var result Packument
	if err := decodeOneJSON(body, &result); err != nil {
		return Packument{}, fmt.Errorf("decode npm packument for %q: %w", name, err)
	}
	if result.Name != name || len(result.Versions) == 0 {
		return Packument{}, fmt.Errorf("invalid npm packument for %q", name)
	}
	for version, metadata := range result.Versions {
		if metadata.Name != name || metadata.Version != version || ValidateVersion(version) != nil {
			return Packument{}, fmt.Errorf("invalid npm packument version %q for %q", version, name)
		}
	}
	return result, nil
}

// VersionMetadata fetches one concrete package version document.
func (c *Client) VersionMetadata(ctx context.Context, name, version string) (VersionMetadata, error) {
	registry, err := c.config.RegistryFor(name)
	if err != nil {
		return VersionMetadata{}, err
	}
	if err := ValidateVersion(version); err != nil {
		return VersionMetadata{}, err
	}
	endpoint, err := packageEndpoint(registry, name)
	if err != nil {
		return VersionMetadata{}, err
	}
	endpoint, err = url.Parse(strings.TrimSuffix(endpoint.String(), "/") + "/" + url.PathEscape(version))
	if err != nil {
		return VersionMetadata{}, fmt.Errorf("build npm version endpoint: %w", err)
	}
	body, err := c.get(ctx, endpoint, MaxPackumentSize, "application/json")
	if err != nil {
		return VersionMetadata{}, err
	}
	var result VersionMetadata
	if err := decodeOneJSON(body, &result); err != nil {
		return VersionMetadata{}, fmt.Errorf("decode npm metadata for %s@%s: %w", name, version, err)
	}
	if err := validateVersionMetadata(name, version, result, true); err != nil {
		return VersionMetadata{}, err
	}
	return result, nil
}

// Download fetches and verifies original compressed tgz bytes.
func (c *Client) Download(ctx context.Context, tarballURL, integrityValue string) ([]byte, error) {
	integrity, err := ParseIntegrity(integrityValue)
	if err != nil {
		return nil, err
	}
	endpoint, err := parseRequestURL(tarballURL)
	if err != nil {
		return nil, err
	}
	body, err := c.get(ctx, endpoint, MaxTarballSize, "application/octet-stream")
	if err != nil {
		return nil, err
	}
	if err := integrity.Verify(body); err != nil {
		return nil, err
	}
	return body, nil
}

func (c *Client) get(ctx context.Context, endpoint *url.URL, limit int, accept string) ([]byte, error) {
	request, cancel, err := c.newRequest(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer cancel()
	request.Header.Set("Accept", accept)
	response, err := c.http.Do(request)
	if err != nil {
		return nil, fmt.Errorf("npm Registry request failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, responseError(request, response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, int64(limit)+1))
	if err != nil {
		return nil, fmt.Errorf("read npm Registry response: %w", err)
	}
	if len(body) > limit {
		return nil, fmt.Errorf("npm Registry response exceeds %d bytes", limit)
	}
	return body, nil
}

func (c *Client) newRequest(ctx context.Context, method string, endpoint *url.URL, body io.Reader) (*http.Request, context.CancelFunc, error) {
	if err := validateRequestURL(endpoint); err != nil {
		return nil, func() {}, err
	}
	bounded, cancel := context.WithTimeout(ctx, RequestTimeout)
	request, err := http.NewRequestWithContext(bounded, method, endpoint.String(), body)
	if err != nil {
		cancel()
		return nil, func() {}, fmt.Errorf("create npm Registry request: %w", err)
	}
	request.Header.Set("User-Agent", "locus-pkg")
	if token := c.config.tokenFor(endpoint); token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	return request, cancel, nil
}

func packageEndpoint(registry *url.URL, name string) (*url.URL, error) {
	if _, err := ParsePackageName(name); err != nil {
		return nil, err
	}
	value := strings.TrimSuffix(registry.String(), "/") + "/" + url.PathEscape(name)
	return url.Parse(value)
}

func parseRequestURL(value string) (*url.URL, error) {
	parsed, err := url.Parse(value)
	if err != nil {
		return nil, fmt.Errorf("invalid npm request URL")
	}
	if err := validateRequestURL(parsed); err != nil {
		return nil, err
	}
	return parsed, nil
}

func validateRequestURL(parsed *url.URL) error {
	if parsed == nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.User != nil || parsed.Fragment != "" {
		return fmt.Errorf("invalid npm request URL")
	}
	if parsed.Scheme == "http" && !isLoopbackHost(parsed.Hostname()) {
		return fmt.Errorf("npm request URL must use HTTPS unless its host is loopback")
	}
	return nil
}

func responseError(request *http.Request, status int) error {
	clean := request.URL.Scheme + "://" + request.URL.Host + request.URL.EscapedPath()
	return &HTTPError{Method: request.Method, URL: clean, StatusCode: status}
}

func decodeOneJSON(content []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(content))
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("unexpected trailing JSON value")
		}
		return err
	}
	return nil
}

func validateVersionMetadata(name, version string, metadata VersionMetadata, requireDistribution bool) error {
	if metadata.Name != name || metadata.Version != version {
		return fmt.Errorf("npm metadata identity mismatch for %s@%s", name, version)
	}
	for dependencyName, constraint := range metadata.Dependencies {
		if _, err := ParsePackageName(dependencyName); err != nil {
			return fmt.Errorf("invalid dependency in %s@%s: %w", name, version, err)
		}
		if err := ValidateConstraint(constraint); err != nil {
			return fmt.Errorf("invalid dependency %q in %s@%s: %w", dependencyName, name, version, err)
		}
	}
	if len(metadata.PeerDependencies) != 0 || len(metadata.OptionalDependencies) != 0 {
		return fmt.Errorf("unsupported peerDependencies or optionalDependencies in %s@%s", name, version)
	}
	if requireDistribution {
		if metadata.Dist.Tarball == "" || metadata.Dist.Integrity == "" {
			return fmt.Errorf("npm metadata for %s@%s lacks tarball integrity", name, version)
		}
		if _, err := parseRequestURL(metadata.Dist.Tarball); err != nil {
			return fmt.Errorf("invalid tarball URL for %s@%s: %w", name, version, err)
		}
		if _, err := ParseIntegrity(metadata.Dist.Integrity); err != nil {
			return fmt.Errorf("invalid integrity for %s@%s: %w", name, version, err)
		}
	}
	return nil
}
