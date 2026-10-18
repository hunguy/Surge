package probe

import (
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/SurgeDM/Surge/internal/integrity"
)

// captureSourceValidators records the representation identity and any
// full-file digest the origin volunteered. The values are later persisted with
// the download so a resume can demand the same representation and a completed
// file can be checked against the bytes the server described.
func captureSourceValidators(resp *http.Response, result *ProbeResult) {
	if resp == nil || result == nil {
		return
	}

	result.ETag = strongETag(resp.Header.Get("ETag"))
	result.LastModified = strings.TrimSpace(resp.Header.Get("Last-Modified"))
	result.Checksum = sourceChecksum(resp)
}

// strongETag returns the validator only when it can be used in If-Range. Weak
// validators (W/"...") are not strong enough to guarantee byte identity
// between two representations, so they are dropped.
func strongETag(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if len(value) >= 2 && strings.EqualFold(value[:2], "W/") {
		return ""
	}
	return value
}

// sourceChecksum picks the strongest whole-file digest from the response
// headers. Digests that only cover the enclosed entity (a single byte range on
// a 206 response) are ignored unless they are explicitly representation-level.
func sourceChecksum(resp *http.Response) string {
	if resp == nil {
		return ""
	}
	if encoding := strings.TrimSpace(resp.Header.Get("Content-Encoding")); encoding != "" && !strings.EqualFold(encoding, "identity") {
		// The digest covers the encoded representation. Surge stores whatever
		// bytes arrive, but an intermediary may decode them on the way, so a
		// comparison here cannot be trusted.
		return ""
	}

	// Repr-Digest (RFC 9530) and x-goog-hash describe the selected
	// representation, so they remain valid on a 206 response.
	if checksum, ok := parseStructuredDigest(resp.Header.Values("Repr-Digest")); ok {
		return checksum
	}
	if checksum, ok := parseGoogHash(resp.Header.Values("x-goog-hash")); ok {
		return checksum
	}

	// Everything below describes the message body. On a 206 that is one range,
	// not the file, so only trust it when the server sent the whole body.
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	if checksum, ok := parseBase64DigestHeader(resp.Header.Get("x-amz-checksum-sha256"), integrity.AlgoSHA256); ok {
		return checksum
	}
	if checksum, ok := parseStructuredDigest(resp.Header.Values("Digest")); ok {
		return checksum
	}
	if checksum, ok := parseHexOrBase64Digest(resp.Header.Get("x-checksum-sha256"), integrity.AlgoSHA256); ok {
		return checksum
	}
	if checksum, ok := parseHexOrBase64Digest(resp.Header.Get("x-checksum-md5"), integrity.AlgoMD5); ok {
		return checksum
	}
	if checksum, ok := parseHexOrBase64Digest(resp.Header.Get("Content-MD5"), integrity.AlgoMD5); ok {
		return checksum
	}
	if checksum, ok := parseStructuredDigest(resp.Header.Values("Content-Digest")); ok {
		return checksum
	}
	return ""
}

// parseStructuredDigest handles both RFC 9530 byte-sequence values
// ("sha-256=:BASE64:") and RFC 3230 bare values ("SHA-256=BASE64").
func parseStructuredDigest(values []string) (string, bool) {
	for _, value := range values {
		if checksum, ok := bestDigestItem(value); ok {
			return checksum, true
		}
	}
	return "", false
}

func bestDigestItem(value string) (string, bool) {
	fallback := ""
	for _, item := range strings.Split(value, ",") {
		algorithm, encoded, ok := strings.Cut(item, "=")
		if !ok {
			continue
		}
		algorithm = normalizeAlgorithm(algorithm)
		if algorithm == "" {
			continue
		}
		encoded = strings.TrimSpace(encoded)
		if len(encoded) >= 2 && strings.HasPrefix(encoded, ":") && strings.HasSuffix(encoded, ":") {
			encoded = encoded[1 : len(encoded)-1]
		}
		decoded, ok := decodeBase64(encoded)
		if !ok || !digestLengthMatches(algorithm, len(decoded)) {
			continue
		}
		checksum := integrity.Format(algorithm, hex.EncodeToString(decoded))
		if algorithm == integrity.AlgoSHA256 {
			return checksum, true
		}
		if fallback == "" {
			fallback = checksum
		}
	}
	return fallback, fallback != ""
}

// parseGoogHash reads the multi-value "x-goog-hash: crc32c=...,md5=..." header
// Google Cloud Storage sends. GCS documents the md5 entry as the hash of the
// complete object, including for ranged requests.
func parseGoogHash(values []string) (string, bool) {
	for _, value := range values {
		if checksum, ok := bestDigestItem(value); ok {
			return checksum, true
		}
	}
	return "", false
}

func parseBase64DigestHeader(value, algorithm string) (string, bool) {
	decoded, ok := decodeBase64(strings.TrimSpace(value))
	if !ok || !digestLengthMatches(algorithm, len(decoded)) {
		return "", false
	}
	return integrity.Format(algorithm, hex.EncodeToString(decoded)), true
}

func parseHexOrBase64Digest(value, algorithm string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}
	if len(value) == hex.EncodedLen(hexDigestSize(algorithm)) {
		if decoded, err := hex.DecodeString(value); err == nil && digestLengthMatches(algorithm, len(decoded)) {
			return integrity.Format(algorithm, strings.ToLower(value)), true
		}
	}
	return parseBase64DigestHeader(value, algorithm)
}

func decodeBase64(value string) ([]byte, bool) {
	if value == "" {
		return nil, false
	}
	if decoded, err := base64.StdEncoding.DecodeString(value); err == nil {
		return decoded, true
	}
	if decoded, err := base64.RawStdEncoding.DecodeString(value); err == nil {
		return decoded, true
	}
	return nil, false
}

func normalizeAlgorithm(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "sha-256", "sha256":
		return integrity.AlgoSHA256
	case "md5":
		return integrity.AlgoMD5
	default:
		return ""
	}
}

func hexDigestSize(algorithm string) int {
	if algorithm == integrity.AlgoSHA256 {
		return 32
	}
	return 16
}

func digestLengthMatches(algorithm string, length int) bool {
	return length == hexDigestSize(algorithm)
}
