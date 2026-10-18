// Package integrity parses source-provided content digests and hashes local
// files so a finished download can be compared against the bytes the server
// said it would deliver.
package integrity

import (
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"os"
	"strings"

	"github.com/SurgeDM/Surge/internal/utils"
)

const (
	AlgoMD5    = "md5"
	AlgoSHA256 = "sha256"
)

// Checksum is a normalized content digest: a supported algorithm name plus a
// lowercase hex value.
type Checksum struct {
	Algorithm string
	Value     string
}

// Parse normalizes a digest in any of the forms Surge accepts: "sha256:<hex>",
// "sha-256:<hex>", "md5:<hex>", or a bare 64/32 character hex string. It
// reports false for empty, unsupported, or malformed values so callers can
// skip verification instead of failing a download over metadata they cannot
// check.
func Parse(raw string) (Checksum, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return Checksum{}, false
	}

	algo := ""
	value := trimmed
	if idx := strings.IndexByte(trimmed, ':'); idx >= 0 {
		algo = strings.ToLower(strings.TrimSpace(trimmed[:idx]))
		value = strings.TrimSpace(trimmed[idx+1:])
	}

	switch algo {
	case "":
		// Infer from the hex length below.
	case AlgoMD5:
	case AlgoSHA256, "sha-256":
		algo = AlgoSHA256
	default:
		return Checksum{}, false
	}

	value = strings.TrimSpace(value)
	if decoded, err := hex.DecodeString(value); err != nil || len(decoded) == 0 {
		return Checksum{}, false
	}

	switch {
	case algo == AlgoMD5:
		if len(value) != md5.Size*2 {
			return Checksum{}, false
		}
	case algo == AlgoSHA256:
		if len(value) != sha256.Size*2 {
			return Checksum{}, false
		}
	case len(value) == sha256.Size*2:
		algo = AlgoSHA256
	case len(value) == md5.Size*2:
		algo = AlgoMD5
	default:
		return Checksum{}, false
	}

	return Checksum{Algorithm: algo, Value: strings.ToLower(value)}, true
}

// Format renders a digest in the persisted "algo:hex" form.
func Format(algorithm, hexValue string) string {
	return algorithm + ":" + strings.ToLower(hexValue)
}

// HashBytes returns the lowercase hex digest of an in-memory buffer.
func HashBytes(data []byte, algorithm string) (string, error) {
	switch algorithm {
	case AlgoMD5:
		sum := md5.Sum(data)
		return hex.EncodeToString(sum[:]), nil
	case AlgoSHA256:
		sum := sha256.Sum256(data)
		return hex.EncodeToString(sum[:]), nil
	default:
		return "", fmt.Errorf("unsupported checksum algorithm %q", algorithm)
	}
}

// HashFile streams a file through the named digest algorithm and returns the
// lowercase hex result.
func HashFile(path, algorithm string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

	var h hash.Hash
	switch algorithm {
	case AlgoMD5:
		h = md5.New()
	case AlgoSHA256:
		h = sha256.New()
	default:
		return "", fmt.Errorf("unsupported checksum algorithm %q", algorithm)
	}

	buf := make([]byte, utils.MiB)
	if _, err := io.CopyBuffer(h, f, buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
