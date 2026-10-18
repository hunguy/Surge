package scheduler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/SurgeDM/Surge/internal/integrity"
	"github.com/SurgeDM/Surge/internal/progress"
	"github.com/SurgeDM/Surge/internal/testutil"
	"github.com/SurgeDM/Surge/internal/types"
)

func writeWorkingFile(t *testing.T, destPath string, content []byte) {
	t.Helper()
	if err := os.WriteFile(destPath+types.IncompleteSuffix, content, 0o600); err != nil {
		t.Fatalf("write working file: %v", err)
	}
}

func testPayload() []byte {
	payload := make([]byte, 200*1024)
	for i := range payload {
		payload[i] = byte(i*7 + i/251)
	}
	return payload
}

func TestVerifyCompletedDownload_StructuralChecks(t *testing.T) {
	cfg := &types.DownloadRecord{}
	payload := testPayload()
	dir := t.TempDir()
	destPath := filepath.Join(dir, "file.bin")

	t.Run("size mismatch", func(t *testing.T) {
		writeWorkingFile(t, destPath, payload[:len(payload)-1])
		err := verifyCompletedDownload(cfg, nil, destPath, int64(len(payload)))
		if !errors.Is(err, types.ErrChecksumMismatch) {
			t.Fatalf("err = %v, want ErrChecksumMismatch", err)
		}
		if !strings.Contains(err.Error(), "size mismatch") {
			t.Fatalf("error should explain the size mismatch, got %q", err)
		}
	})

	t.Run("chunk coverage incomplete", func(t *testing.T) {
		writeWorkingFile(t, destPath, payload)
		state := progress.New("coverage", int64(len(payload)))
		const chunkSize = 64 * 1024
		state.InitBitmap(int64(len(payload)), chunkSize)
		state.UpdateChunkStatus(0, chunkSize, types.ChunkCompleted)
		err := verifyCompletedDownload(cfg, state, destPath, int64(len(payload)))
		if !errors.Is(err, types.ErrChecksumMismatch) {
			t.Fatalf("err = %v, want ErrChecksumMismatch", err)
		}
		if !strings.Contains(err.Error(), "chunk coverage") {
			t.Fatalf("error should explain missing coverage, got %q", err)
		}
	})

	t.Run("complete file passes without checksum", func(t *testing.T) {
		writeWorkingFile(t, destPath, payload)
		if err := verifyCompletedDownload(cfg, nil, destPath, int64(len(payload))); err != nil {
			t.Fatalf("verify completed file: %v", err)
		}
	})

	t.Run("missing working file fails", func(t *testing.T) {
		err := verifyCompletedDownload(cfg, nil, filepath.Join(dir, "absent.bin"), 10)
		if !errors.Is(err, types.ErrChecksumMismatch) {
			t.Fatalf("err = %v, want ErrChecksumMismatch", err)
		}
	})
}

func TestVerifyCompletedDownload_Digest(t *testing.T) {
	payload := testPayload()
	dir := t.TempDir()
	destPath := filepath.Join(dir, "file.bin")
	writeWorkingFile(t, destPath, payload)

	sum, err := integrity.HashFile(destPath+types.IncompleteSuffix, integrity.AlgoSHA256)
	if err != nil {
		t.Fatal(err)
	}
	good := integrity.Format(integrity.AlgoSHA256, sum)

	t.Run("matching checksum passes", func(t *testing.T) {
		cfg := &types.DownloadRecord{Checksum: good}
		if err := verifyCompletedDownload(cfg, nil, destPath, int64(len(payload))); err != nil {
			t.Fatalf("verify: %v", err)
		}
	})

	t.Run("mismatched checksum fails with both digests", func(t *testing.T) {
		cfg := &types.DownloadRecord{Checksum: integrity.Format(integrity.AlgoSHA256, strings.Repeat("0", 64))}
		err := verifyCompletedDownload(cfg, nil, destPath, int64(len(payload)))
		if !errors.Is(err, types.ErrChecksumMismatch) {
			t.Fatalf("err = %v, want ErrChecksumMismatch", err)
		}
		msg := err.Error()
		if !strings.Contains(msg, "checksum mismatch") || !strings.Contains(msg, sum) {
			t.Fatalf("error should show expected and actual digests, got %q", msg)
		}
	})

	t.Run("verification disabled skips the digest but still checks size", func(t *testing.T) {
		cfg := &types.DownloadRecord{
			Checksum:           integrity.Format(integrity.AlgoSHA256, strings.Repeat("0", 64)),
			SkipChecksumVerify: true,
		}
		if err := verifyCompletedDownload(cfg, nil, destPath, int64(len(payload))); err != nil {
			t.Fatalf("disabled verification should skip the digest: %v", err)
		}
		if err := verifyCompletedDownload(cfg, nil, destPath, int64(len(payload))+1); !errors.Is(err, types.ErrChecksumMismatch) {
			t.Fatalf("size check must still run when disabled, got %v", err)
		}
	})

	t.Run("unsupported stored checksum is ignored", func(t *testing.T) {
		cfg := &types.DownloadRecord{Checksum: "crc32:deadbeef"}
		if err := verifyCompletedDownload(cfg, nil, destPath, int64(len(payload))); err != nil {
			t.Fatalf("unsupported checksum should not fail the download: %v", err)
		}
	})
}

// rangeServer serves payload with range support and records whether every
// primary range request carried the expected If-Range validator.
func rangeServer(t *testing.T, payload []byte, wantIfRange string) (*httptest.Server, *int) {
	t.Helper()
	var (
		mu           sync.Mutex
		ifRangeCount int
	)
	server := testutil.NewHTTPServerT(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rangeHeader := r.Header.Get("Range")
		if r.Header.Get("If-Range") == wantIfRange && wantIfRange != "" {
			mu.Lock()
			ifRangeCount++
			mu.Unlock()
		}
		if rangeHeader == "" {
			w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
			_, _ = w.Write(payload)
			return
		}
		start, end, err := parseRangeHeader(rangeHeader, int64(len(payload)))
		if err != nil {
			http.Error(w, "bad range", http.StatusRequestedRangeNotSatisfiable)
			return
		}
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(payload)))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(payload[start : end+1])
	}))
	return server, &ifRangeCount
}

func parseRangeHeader(header string, size int64) (int64, int64, error) {
	spec, ok := strings.CutPrefix(header, "bytes=")
	if !ok {
		return 0, 0, fmt.Errorf("unsupported range %q", header)
	}
	startStr, endStr, ok := strings.Cut(spec, "-")
	if !ok {
		return 0, 0, fmt.Errorf("unsupported range %q", header)
	}
	start, err := strconv.ParseInt(startStr, 10, 64)
	if err != nil || start < 0 || start >= size {
		return 0, 0, fmt.Errorf("bad range start %q", startStr)
	}
	end := size - 1
	if endStr != "" {
		end, err = strconv.ParseInt(endStr, 10, 64)
		if err != nil || end < start {
			return 0, 0, fmt.Errorf("bad range end %q", endStr)
		}
	}
	if end >= size {
		end = size - 1
	}
	return start, end, nil
}

func TestRunDownload_ChecksumMismatchBecomesError(t *testing.T) {
	testutil.SetupStateDB(t)
	payload := testPayload()
	dir := t.TempDir()
	destPath := filepath.Join(dir, "digest.bin")
	server, _ := rangeServer(t, payload, "")
	defer server.Close()

	base := func() types.DownloadRecord {
		return types.DownloadRecord{
			ID:            "digest-mismatch",
			URL:           server.URL + "/digest.bin",
			OutputPath:    dir,
			DestPath:      destPath,
			Filename:      "digest.bin",
			TotalSize:     int64(len(payload)),
			SupportsRange: true,
			ProgressState: progress.New("digest-mismatch", 0),
			Runtime:       &types.RuntimeConfig{Workers: 2, MaxConnectionsPerDownload: 2, MinChunkSize: 64 * 1024},
		}
	}

	t.Run("mismatch keeps the working file and fails", func(t *testing.T) {
		cfg := base()
		cfg.Checksum = integrity.Format(integrity.AlgoSHA256, strings.Repeat("0", 64))
		writeWorkingFile(t, destPath, nil)

		err := RunDownload(context.Background(), &cfg)
		if !errors.Is(err, types.ErrChecksumMismatch) {
			t.Fatalf("err = %v, want ErrChecksumMismatch", err)
		}
		if _, statErr := os.Stat(destPath); !os.IsNotExist(statErr) {
			t.Fatalf("mismatched file must not be promoted: stat err = %v", statErr)
		}
		if _, statErr := os.Stat(destPath + types.IncompleteSuffix); statErr != nil {
			t.Fatalf("working file should be kept for inspection: %v", statErr)
		}
	})

	t.Run("matching checksum completes", func(t *testing.T) {
		cfg := base()
		sum, hashErr := integrity.HashBytes(payload, integrity.AlgoSHA256)
		if hashErr != nil {
			t.Fatal(hashErr)
		}
		cfg.Checksum = integrity.Format(integrity.AlgoSHA256, sum)
		writeWorkingFile(t, destPath, nil)

		if err := RunDownload(context.Background(), &cfg); err != nil {
			t.Fatalf("RunDownload: %v", err)
		}
		got, readErr := os.ReadFile(destPath + types.IncompleteSuffix)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if len(got) != len(payload) {
			t.Fatalf("downloaded %d bytes, want %d", len(got), len(payload))
		}
	})
}

func TestRunDownload_SendsIfRangeValidator(t *testing.T) {
	testutil.SetupStateDB(t)
	payload := testPayload()
	dir := t.TempDir()
	destPath := filepath.Join(dir, "ifrange.bin")
	const validator = `"v1-strong"`
	server, ifRangeCount := rangeServer(t, payload, validator)
	defer server.Close()

	cfg := types.DownloadRecord{
		ID:            "ifrange",
		URL:           server.URL + "/ifrange.bin",
		OutputPath:    dir,
		DestPath:      destPath,
		Filename:      "ifrange.bin",
		TotalSize:     int64(len(payload)),
		SupportsRange: true,
		ETag:          validator,
		ProgressState: progress.New("ifrange", int64(len(payload))),
		Runtime:       &types.RuntimeConfig{Workers: 2, MaxConnectionsPerDownload: 2, MinChunkSize: 64 * 1024},
	}
	writeWorkingFile(t, destPath, nil)

	if err := RunDownload(context.Background(), &cfg); err != nil {
		t.Fatalf("RunDownload: %v", err)
	}
	if *ifRangeCount == 0 {
		t.Fatal("no range request carried the probed If-Range validator")
	}
}
