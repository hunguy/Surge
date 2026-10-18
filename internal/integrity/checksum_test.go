package integrity

import (
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	sha := strings.Repeat("a", sha256.Size*2)
	md5hex := strings.Repeat("b", md5.Size*2)

	cases := []struct {
		name  string
		raw   string
		want  Checksum
		valid bool
	}{
		{"sha256 prefix", "sha256:" + sha, Checksum{AlgoSHA256, sha}, true},
		{"sha-256 prefix", "SHA-256:" + strings.ToUpper(sha), Checksum{AlgoSHA256, sha}, true},
		{"md5 prefix", "md5:" + md5hex, Checksum{AlgoMD5, md5hex}, true},
		{"bare sha256", sha, Checksum{AlgoSHA256, sha}, true},
		{"bare md5", md5hex, Checksum{AlgoMD5, md5hex}, true},
		{"whitespace", "  sha256:" + sha + "\n", Checksum{AlgoSHA256, sha}, true},
		{"empty", "", Checksum{}, false},
		{"unsupported", "sha512:" + strings.Repeat("c", 128), Checksum{}, false},
		{"non hex", "sha256:" + strings.Repeat("z", sha256.Size*2), Checksum{}, false},
		{"wrong length", "sha256:" + md5hex, Checksum{}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := Parse(tc.raw)
			if ok != tc.valid {
				t.Fatalf("Parse(%q) ok = %v, want %v", tc.raw, ok, tc.valid)
			}
			if ok && got != tc.want {
				t.Fatalf("Parse(%q) = %+v, want %+v", tc.raw, got, tc.want)
			}
		})
	}
}

func TestHashFile(t *testing.T) {
	content := []byte("surge integrity check\n")
	path := filepath.Join(t.TempDir(), "payload.bin")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}

	shaSum := sha256.Sum256(content)
	got, err := HashFile(path, AlgoSHA256)
	if err != nil {
		t.Fatalf("HashFile(sha256): %v", err)
	}
	if got != hex.EncodeToString(shaSum[:]) {
		t.Fatalf("sha256 = %q, want %q", got, hex.EncodeToString(shaSum[:]))
	}

	md5Sum := md5.Sum(content)
	got, err = HashFile(path, AlgoMD5)
	if err != nil {
		t.Fatalf("HashFile(md5): %v", err)
	}
	if got != hex.EncodeToString(md5Sum[:]) {
		t.Fatalf("md5 = %q, want %q", got, hex.EncodeToString(md5Sum[:]))
	}

	if _, err := HashFile(path, "crc32"); err == nil {
		t.Fatal("expected unsupported algorithm error")
	}
	if _, err := HashFile(filepath.Join(t.TempDir(), "missing"), AlgoSHA256); err == nil {
		t.Fatal("expected missing file error")
	}
}
