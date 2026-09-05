package npm

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// BuildPublishBody creates the npm Registry publish document for one immutable
// version and one base64-encoded tgz attachment.
func BuildPublishBody(pack PackedPackage, registry *url.URL) ([]byte, error) {
	if _, err := ParsePackageName(pack.Name); err != nil {
		return nil, err
	}
	if err := ValidateVersion(pack.Version); err != nil {
		return nil, err
	}
	if len(pack.Tarball) == 0 || len(pack.Tarball) > MaxTarballSize {
		return nil, fmt.Errorf("packed tarball must contain at most %d bytes", MaxTarballSize)
	}
	if registry == nil {
		return nil, fmt.Errorf("npm publish Registry is required")
	}
	if err := validateRequestURL(registry); err != nil {
		return nil, err
	}
	integrity, err := ParseIntegrity(pack.Integrity)
	if err != nil {
		return nil, err
	}
	if integrity.Algorithm() != "sha512" || integrity.Verify(pack.Tarball) != nil {
		return nil, fmt.Errorf("packed tarball SHA-512 integrity mismatch")
	}
	if pack.Filename != TarballFilename(pack.Name, pack.Version) {
		return nil, fmt.Errorf("invalid npm tarball filename %q", pack.Filename)
	}
	if err := rejectDuplicateJSONKeys(pack.PackageJSON); err != nil {
		return nil, fmt.Errorf("decode packed package.json: %w", err)
	}
	var versionDocument map[string]any
	decoder := json.NewDecoder(bytes.NewReader(pack.PackageJSON))
	decoder.UseNumber()
	if err := decoder.Decode(&versionDocument); err != nil {
		return nil, fmt.Errorf("decode packed package.json: %w", err)
	}
	if versionDocument["name"] != pack.Name || versionDocument["version"] != pack.Version {
		return nil, fmt.Errorf("packed package.json identity mismatch")
	}

	tarballURL := *registry
	tarballURL.Path = strings.TrimSuffix(tarballURL.Path, "/") + "/" + pack.Name + "/-/" + pack.Filename
	tarballURL.RawPath = ""
	sha1Digest := sha1.Sum(pack.Tarball)
	versionDocument["_id"] = pack.Name + "@" + pack.Version
	versionDocument["dist"] = map[string]any{
		"integrity": pack.Integrity,
		"shasum":    hex.EncodeToString(sha1Digest[:]),
		"tarball":   tarballURL.String(),
	}
	document := map[string]any{
		"_id":       pack.Name,
		"name":      pack.Name,
		"dist-tags": map[string]string{"latest": pack.Version},
		"versions":  map[string]any{pack.Version: versionDocument},
		"_attachments": map[string]any{
			pack.Filename: map[string]any{
				"content_type": "application/octet-stream",
				"data":         base64.StdEncoding.EncodeToString(pack.Tarball),
				"length":       len(pack.Tarball),
			},
		},
	}
	body, err := json.Marshal(document)
	if err != nil {
		return nil, fmt.Errorf("encode npm publish body: %w", err)
	}
	return body, nil
}

// Publish uploads one immutable package version to its selected Registry.
func (c *Client) Publish(ctx context.Context, pack PackedPackage) error {
	registry, err := c.config.RegistryFor(pack.Name)
	if err != nil {
		return err
	}
	body, err := BuildPublishBody(pack, registry)
	if err != nil {
		return err
	}
	endpoint, err := packageEndpoint(registry, pack.Name)
	if err != nil {
		return err
	}
	request, cancel, err := c.newRequest(ctx, http.MethodPut, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer cancel()
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	response, err := c.http.Do(request)
	if err != nil {
		return fmt.Errorf("npm Registry publish failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusConflict {
		return fmt.Errorf("%w: %s@%s already exists", ErrImmutableVersion, pack.Name, pack.Version)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return responseError(request, response.StatusCode)
	}
	if _, err := io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20)); err != nil {
		return fmt.Errorf("read npm Registry publish response: %w", err)
	}
	return nil
}
