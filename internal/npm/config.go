package npm

import (
	"bufio"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const DefaultRegistry = "https://registry.npmjs.org/"

type authToken struct {
	host  string
	path  string
	value string
}

// Config is immutable Registry routing and bearer-token configuration.
type Config struct {
	defaultRegistry *url.URL
	scopedRegistry  map[string]*url.URL
	tokens          []authToken
	fallbackToken   string
}

// ConfigOptions exposes configuration inputs so callers and tests can isolate
// environment and user configuration.
type ConfigOptions struct {
	Root              string
	Registry          string
	UserConfig        string
	SkipUserConfig    bool
	LookupEnvironment func(string) (string, bool)
}

// LoadConfig reads npm configuration using command, environment, project, then
// user precedence. Scoped registries still override the selected default.
func LoadConfig(root, registryOverride string) (*Config, error) {
	return LoadConfigWithOptions(ConfigOptions{Root: root, Registry: registryOverride})
}

// LoadConfigWithOptions is LoadConfig with explicit configuration sources.
func LoadConfigWithOptions(options ConfigOptions) (*Config, error) {
	lookup := options.LookupEnvironment
	if lookup == nil {
		lookup = os.LookupEnv
	}
	values := make(map[string]string)
	if !options.SkipUserConfig {
		userConfig := options.UserConfig
		if userConfig == "" {
			if configured, ok := lookup("NPM_CONFIG_USERCONFIG"); ok && configured != "" {
				userConfig = configured
			} else if home, err := os.UserHomeDir(); err == nil {
				userConfig = filepath.Join(home, ".npmrc")
			}
		}
		if userConfig != "" {
			if err := mergeNPMRC(values, userConfig, lookup); err != nil {
				return nil, err
			}
		}
	}
	if options.Root != "" {
		if err := mergeNPMRC(values, filepath.Join(options.Root, ".npmrc"), lookup); err != nil {
			return nil, err
		}
	}

	defaultValue := values["registry"]
	if defaultValue == "" {
		defaultValue = DefaultRegistry
	}
	if environmentRegistry, ok := lookup("NPM_CONFIG_REGISTRY"); ok && environmentRegistry != "" {
		defaultValue = environmentRegistry
	}
	if options.Registry != "" {
		defaultValue = options.Registry
	}
	defaultRegistry, err := parseRemoteURL(defaultValue, "registry")
	if err != nil {
		return nil, err
	}

	config := &Config{
		defaultRegistry: defaultRegistry,
		scopedRegistry:  make(map[string]*url.URL),
	}
	for key, value := range values {
		lowerKey := strings.ToLower(key)
		switch {
		case strings.HasPrefix(key, "@") && strings.HasSuffix(lowerKey, ":registry"):
			scope := key[:len(key)-len(":registry")]
			if _, err := ParsePackageName(scope + "/package"); err != nil {
				return nil, fmt.Errorf("invalid scoped registry key %q", key)
			}
			registry, err := parseRemoteURL(value, key)
			if err != nil {
				return nil, err
			}
			config.scopedRegistry[scope] = registry
		case strings.HasSuffix(lowerKey, ":_authtoken"):
			token, err := parseAuthToken(key, value)
			if err != nil {
				return nil, err
			}
			config.tokens = append(config.tokens, token)
		case unsupportedCredentialKey(lowerKey):
			return nil, fmt.Errorf("unsupported npm credential field %q; bearer auth tokens are required", key)
		}
	}
	if token, ok := lookup("NPM_TOKEN"); ok {
		config.fallbackToken = token
	}
	return config, nil
}

func mergeNPMRC(values map[string]string, filename string, lookup func(string) (string, bool)) error {
	file, err := os.Open(filename)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read npm configuration %q: %w", filename, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found || strings.TrimSpace(key) == "" {
			return fmt.Errorf("invalid npm configuration line in %q", filename)
		}
		expanded, err := interpolateEnvironment(strings.TrimSpace(value), lookup)
		if err != nil {
			return fmt.Errorf("invalid npm configuration value for %q in %q: %w", strings.TrimSpace(key), filename, err)
		}
		values[strings.TrimSpace(key)] = expanded
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read npm configuration %q: %w", filename, err)
	}
	return nil
}

func interpolateEnvironment(value string, lookup func(string) (string, bool)) (string, error) {
	var result strings.Builder
	for {
		start := strings.Index(value, "${")
		if start < 0 {
			result.WriteString(value)
			return result.String(), nil
		}
		result.WriteString(value[:start])
		value = value[start+2:]
		end := strings.IndexByte(value, '}')
		if end < 0 || end == 0 {
			return "", fmt.Errorf("malformed environment interpolation")
		}
		name := value[:end]
		replacement, ok := lookup(name)
		if !ok {
			return "", fmt.Errorf("environment variable %s is not set", name)
		}
		result.WriteString(replacement)
		value = value[end+1:]
	}
}

func unsupportedCredentialKey(key string) bool {
	if key == "username" || key == "password" || key == "_password" || key == "login" || key == "_auth" {
		return true
	}
	for _, suffix := range []string{":username", ":password", ":_password", ":login", ":_auth"} {
		if strings.HasSuffix(key, suffix) {
			return true
		}
	}
	return false
}

func parseAuthToken(key, value string) (authToken, error) {
	prefix := key[:len(key)-len(":_authToken")]
	if !strings.HasPrefix(prefix, "//") || value == "" {
		return authToken{}, fmt.Errorf("invalid npm bearer token configuration key %q", key)
	}
	parsed, err := url.Parse("https:" + prefix)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return authToken{}, fmt.Errorf("invalid npm bearer token configuration key %q", key)
	}
	path := parsed.EscapedPath()
	if path == "" {
		path = "/"
	}
	if !strings.HasSuffix(path, "/") {
		path += "/"
	}
	return authToken{host: strings.ToLower(parsed.Host), path: path, value: value}, nil
}

func parseRemoteURL(value, field string) (*url.URL, error) {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("invalid npm %s URL %q", field, value)
	}
	if parsed.Scheme == "http" && !isLoopbackHost(parsed.Hostname()) {
		return nil, fmt.Errorf("npm %s URL must use HTTPS unless its host is loopback", field)
	}
	if !strings.HasSuffix(parsed.Path, "/") {
		parsed.Path += "/"
	}
	parsed.RawPath = ""
	return parsed, nil
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// RegistryFor returns the default or matching scoped Registry URL.
func (c *Config) RegistryFor(name string) (*url.URL, error) {
	if _, err := ParsePackageName(name); err != nil {
		return nil, err
	}
	if strings.HasPrefix(name, "@") {
		if slash := strings.IndexByte(name, '/'); slash > 0 {
			if registry := c.scopedRegistry[name[:slash]]; registry != nil {
				copy := *registry
				return &copy, nil
			}
		}
	}
	copy := *c.defaultRegistry
	return &copy, nil
}

func (c *Config) tokenFor(target *url.URL) string {
	host := strings.ToLower(target.Host)
	path := target.EscapedPath()
	if path == "" {
		path = "/"
	}
	best, bestLength := "", -1
	for _, candidate := range c.tokens {
		if candidate.host == host && strings.HasPrefix(path, candidate.path) && len(candidate.path) > bestLength {
			best, bestLength = candidate.value, len(candidate.path)
		}
	}
	if best != "" {
		return best
	}
	if sameURLOrigin(target, c.defaultRegistry) {
		return c.fallbackToken
	}
	for _, registry := range c.scopedRegistry {
		if sameURLOrigin(target, registry) {
			return c.fallbackToken
		}
	}
	return ""
}

func sameURLOrigin(left, right *url.URL) bool {
	return strings.EqualFold(left.Scheme, right.Scheme) && strings.EqualFold(left.Host, right.Host)
}
