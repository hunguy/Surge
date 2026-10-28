package scheduler

import (
	"bytes"
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/SurgeDM/Surge/internal/progress"
	"github.com/SurgeDM/Surge/internal/store"
	"github.com/SurgeDM/Surge/internal/testutil"
	"github.com/SurgeDM/Surge/internal/types"
)

// A resume whose .surge file vanished used to fail with ENOENT on every attempt:
// both downloaders open the working file without creating it. RunDownload must
// recreate it and restart from zero instead.
func TestRunDownload_ResumeWithoutWorkingFileRestarts(t *testing.T) {
	for _, mode := range []string{"range", "single"} {
		t.Run(mode, func(t *testing.T) {
			testutil.SetupStateDB(t)
			dir := t.TempDir()
			payload := bytes.Repeat([]byte("restart-without-working-file\n"), 4096)
			size := int64(len(payload))
			server := testutil.NewHTTPServerT(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(payload)
			}))
			defer server.Close()

			path := filepath.Join(dir, "file.bin")
			state := progress.New("missing-working-file-"+mode, size)
			state.Bytes.Downloaded.Store(size / 2)
			state.Bytes.VerifiedProgress.Store(size / 2)
			cfg := types.DownloadRecord{
				ID:                 "missing-working-file-" + mode,
				URL:                server.URL + "/file.bin",
				Filename:           "file.bin",
				OutputPath:         dir,
				DestPath:           path,
				TotalSize:          size,
				SupportsRange:      mode == "range",
				Downloaded:         size / 2,
				Tasks:              []types.Task{{Offset: size / 2, Length: size - size/2}},
				ProgressState:      state,
				Runtime:            &types.RuntimeConfig{Workers: 1, MaxConnectionsPerDownload: 1},
				IsResume:           true,
				SkipChecksumVerify: true,
			}
			if err := store.AddToMasterList(cfg); err != nil {
				t.Fatal(err)
			}
			if err := store.SaveStateWithOptions(cfg.URL, path, &cfg, store.SaveStateOptions{SkipFileHash: true}); err != nil {
				t.Fatal(err)
			}

			if err := RunDownload(context.Background(), &cfg); err != nil {
				t.Fatalf("resume without working file failed: %v", err)
			}
			got, err := os.ReadFile(path + types.IncompleteSuffix)
			if err != nil || !bytes.Equal(got, payload) {
				t.Fatalf("working file not restored: err=%v bytes=%d want %d", err, len(got), size)
			}
			if cfg.IsResume || len(cfg.Tasks) != 0 {
				t.Fatalf("resume state survived restart: isResume=%v tasks=%d", cfg.IsResume, len(cfg.Tasks))
			}
			checkpoint, err := store.LoadState(cfg.URL, path)
			if err != nil || checkpoint == nil || len(checkpoint.Tasks) != 0 || checkpoint.Downloaded != 0 {
				t.Fatalf("stale checkpoint survived restart: %+v err=%v", checkpoint, err)
			}
		})
	}
}
