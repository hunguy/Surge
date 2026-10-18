package probe

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/SurgeDM/Surge/internal/integrity"
)

var (
	testSHA256 = make([]byte, 32)
	testMD5    = make([]byte, 16)
)

func init() {
	for i := range testSHA256 {
		testSHA256[i] = byte(i + 1)
	}
	for i := range testMD5 {
		testMD5[i] = byte(i + 101)
	}
}

func TestProbeCapturesStrongValidatorsAndReprDigest(t *testing.T) {
	var sawWantDigest string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawWantDigest = r.Header.Get("Want-Digest")
		w.Header().Set("ETag", `"abc123"`)
		w.Header().Set("Last-Modified", "Wed, 21 Oct 2015 07:28:00 GMT")
		w.Header().Set("Repr-Digest", "sha-256=:"+base64.StdEncoding.EncodeToString(testSHA256)+":")
		w.Header().Set("Content-Range", "bytes 0-0/10")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte("x"))
	}))
	defer server.Close()

	result, err := ProbeServer(context.Background(), server.URL+"/file.bin", "", nil)
	if err != nil {
		t.Fatalf("ProbeServer: %v", err)
	}

	if result.ETag != `"abc123"` {
		t.Fatalf("ETag = %q, want strong etag", result.ETag)
	}
	if result.LastModified != "Wed, 21 Oct 2015 07:28:00 GMT" {
		t.Fatalf("LastModified = %q", result.LastModified)
	}
	want := integrity.Format(integrity.AlgoSHA256, hexString(testSHA256))
	if result.Checksum != want {
		t.Fatalf("Checksum = %q, want %q", result.Checksum, want)
	}
	if sawWantDigest != "sha-256" {
		t.Fatalf("Want-Digest = %q, want sha-256", sawWantDigest)
	}
}

func TestProbeRejectsWeakETagAndPartialDigest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `W/"weak"`)
		w.Header().Set("Content-MD5", base64.StdEncoding.EncodeToString(testMD5))
		w.Header().Set("Content-Range", "bytes 0-0/10")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte("x"))
	}))
	defer server.Close()

	result, err := ProbeServer(context.Background(), server.URL+"/file.bin", "", nil)
	if err != nil {
		t.Fatalf("ProbeServer: %v", err)
	}
	if result.ETag != "" {
		t.Fatalf("weak ETag must not be stored, got %q", result.ETag)
	}
	if result.Checksum != "" {
		t.Fatalf("Content-MD5 on a 206 covers one range, got %q", result.Checksum)
	}
}

func TestProbeUsesContentMD5OnFullBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-MD5", base64.StdEncoding.EncodeToString(testMD5))
		w.Header().Set("Content-Length", "4")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data"))
	}))
	defer server.Close()

	result, err := ProbeServer(context.Background(), server.URL+"/file.bin", "", nil)
	if err != nil {
		t.Fatalf("ProbeServer: %v", err)
	}
	want := integrity.Format(integrity.AlgoMD5, hexString(testMD5))
	if result.Checksum != want {
		t.Fatalf("Checksum = %q, want %q", result.Checksum, want)
	}
}

func TestProbeUsesGoogHashOnRangedResponse(t *testing.T) {
	md5Base64 := base64.StdEncoding.EncodeToString(testMD5)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"goog-etag"`)
		w.Header().Set("x-goog-hash", "crc32c=AAAAAA==,md5="+md5Base64)
		w.Header().Set("Content-Range", "bytes 0-0/10")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte("x"))
	}))
	defer server.Close()

	result, err := ProbeServer(context.Background(), server.URL+"/file.bin", "", nil)
	if err != nil {
		t.Fatalf("ProbeServer: %v", err)
	}
	want := integrity.Format(integrity.AlgoMD5, hexString(testMD5))
	if result.Checksum != want {
		t.Fatalf("Checksum = %q, want %q", result.Checksum, want)
	}
}

func TestProbeSkipsDigestForEncodedBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Repr-Digest", "sha-256=:"+base64.StdEncoding.EncodeToString(testSHA256)+":")
		w.Header().Set("Content-Range", "bytes 0-0/10")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte("x"))
	}))
	defer server.Close()

	result, err := ProbeServer(context.Background(), server.URL+"/file.bin", "", nil)
	if err != nil {
		t.Fatalf("ProbeServer: %v", err)
	}
	if result.Checksum != "" {
		t.Fatalf("digest of an encoded body must not be used, got %q", result.Checksum)
	}
}

func TestProbeIgnoresUnsupportedDigestAlgorithm(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Repr-Digest", "sha-512=:"+base64.StdEncoding.EncodeToString(make([]byte, 64))+":")
		w.Header().Set("Content-Range", "bytes 0-0/10")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte("x"))
	}))
	defer server.Close()

	result, err := ProbeServer(context.Background(), server.URL+"/file.bin", "", nil)
	if err != nil {
		t.Fatalf("ProbeServer: %v", err)
	}
	if result.Checksum != "" {
		t.Fatalf("unsupported algorithm must be ignored, got %q", result.Checksum)
	}
}

func TestStrongETag(t *testing.T) {
	cases := map[string]string{
		`"abc"`:    `"abc"`,
		`W/"abc"`:  "",
		`w/"abc"`:  "",
		`  "abc" `: `"abc"`,
		``:         "",
	}
	for input, want := range cases {
		if got := strongETag(input); got != want {
			t.Errorf("strongETag(%q) = %q, want %q", input, got, want)
		}
	}
}

func hexString(raw []byte) string {
	const alphabet = "0123456789abcdef"
	out := make([]byte, 0, len(raw)*2)
	for _, b := range raw {
		out = append(out, alphabet[b>>4], alphabet[b&0x0f])
	}
	return string(out)
}
