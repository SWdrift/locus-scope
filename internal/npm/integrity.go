package npm

import (
	"crypto/sha256"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
)

// Integrity is one supported Subresource Integrity digest.
type Integrity struct {
	algorithm string
	digest    []byte
}

// ParseIntegrity accepts SHA-512 and SHA-256 SRI values and chooses the
// strongest supported digest when multiple whitespace-separated values exist.
func ParseIntegrity(value string) (Integrity, error) {
	var selected Integrity
	for _, token := range strings.Fields(value) {
		algorithm, encoded, found := strings.Cut(token, "-")
		if !found || strings.Contains(encoded, "?") {
			return Integrity{}, fmt.Errorf("invalid package integrity")
		}
		if algorithm != "sha512" && algorithm != "sha256" {
			continue
		}
		digest, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return Integrity{}, fmt.Errorf("invalid %s package integrity", algorithm)
		}
		expectedLength := sha256.Size
		if algorithm == "sha512" {
			expectedLength = sha512.Size
		}
		if len(digest) != expectedLength {
			return Integrity{}, fmt.Errorf("invalid %s package integrity", algorithm)
		}
		candidate := Integrity{algorithm: algorithm, digest: digest}
		if selected.algorithm == "" || algorithm == "sha512" {
			selected = candidate
		}
	}
	if selected.algorithm == "" {
		return Integrity{}, fmt.Errorf("package integrity must contain SHA-512 or SHA-256")
	}
	return selected, nil
}

// IntegrityFor computes the canonical SHA-512 SRI value for content.
func IntegrityFor(content []byte) Integrity {
	digest := sha512.Sum512(content)
	return Integrity{algorithm: "sha512", digest: digest[:]}
}

func (i Integrity) String() string {
	return i.algorithm + "-" + base64.StdEncoding.EncodeToString(i.digest)
}

// Algorithm returns the lowercase SRI hash algorithm.
func (i Integrity) Algorithm() string { return i.algorithm }

// Hex returns the lowercase digest used by integrity-addressed stores.
func (i Integrity) Hex() string { return hex.EncodeToString(i.digest) }

// Verify checks content against the parsed digest.
func (i Integrity) Verify(content []byte) error {
	var actual []byte
	switch i.algorithm {
	case "sha512":
		digest := sha512.Sum512(content)
		actual = digest[:]
	case "sha256":
		digest := sha256.Sum256(content)
		actual = digest[:]
	default:
		return fmt.Errorf("unsupported package integrity algorithm")
	}
	if subtle.ConstantTimeCompare(i.digest, actual) != 1 {
		return fmt.Errorf("package integrity mismatch")
	}
	return nil
}
