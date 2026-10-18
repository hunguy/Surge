package concurrent

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"sync"
	"testing"

	"github.com/SurgeDM/Surge/internal/testutil"
	"github.com/SurgeDM/Surge/internal/types"
)

func TestDownloadTask_SendsIfRangeOnlyForPrimaryOrigin(t *testing.T) {
	const fileSize = int64(32 * 1024)
	const validator = `"v1-strong"`

	var (
		mu            sync.Mutex
		primaryHeader string
		mirrorHeader  string
	)

	handler := func(target *string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			start, end, err := parseTestByteRange(r.Header.Get("Range"))
			if err != nil {
				http.Error(w, "bad range", http.StatusRequestedRangeNotSatisfiable)
				return
			}
			mu.Lock()
			*target = r.Header.Get("If-Range")
			mu.Unlock()
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, fileSize))
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write(make([]byte, end-start+1))
		}
	}

	primary := testutil.NewHTTPServerT(t, handler(&primaryHeader))
	defer primary.Close()
	mirror := testutil.NewHTTPServerT(t, handler(&mirrorHeader))
	defer mirror.Close()

	downloader := NewConcurrentDownloader("if-range-task", nil, nil, &types.RuntimeConfig{
		MaxConnectionsPerDownload: 1,
		Workers:                   1,
		MinChunkSize:              types.AlignSize,
	})
	downloader.URL = primary.URL
	downloader.DestPath = "unused"
	downloader.IfRange = validator

	outFile, err := os.CreateTemp(t.TempDir(), "if-range-*.surge")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = outFile.Close() }()

	run := func(targetURL string) {
		t.Helper()
		task := types.Task{Offset: 0, Length: fileSize}
		active := &ActiveTask{Task: task}
		active.CurrentOffset.Store(task.Offset)
		active.StopAt.Store(task.Offset + task.Length)
		client := primary.Client()
		if targetURL == mirror.URL {
			client = mirror.Client()
		}
		if err := downloader.downloadTask(context.Background(), targetURL, outFile, active, make([]byte, 4096), client, fileSize); err != nil {
			t.Fatalf("downloadTask(%s): %v", targetURL, err)
		}
	}

	run(primary.URL)
	run(mirror.URL)

	mu.Lock()
	defer mu.Unlock()
	if primaryHeader != validator {
		t.Fatalf("primary If-Range = %q, want %q", primaryHeader, validator)
	}
	if mirrorHeader != "" {
		t.Fatalf("mirror must not be held to the primary validator, got If-Range %q", mirrorHeader)
	}
}
