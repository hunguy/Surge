package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/SurgeDM/Surge/internal/types"
)

func TestUpdate_ChecksumMismatchMarksDownloadErrored(t *testing.T) {
	d := NewDownloadModel("dl-1", "https://example.com/file.bin", "file.bin", 100)
	d.Downloaded = 40
	d.started = true

	m := newCopyURLModel(TabActive, []*DownloadModel{d})

	failure := fmt.Errorf("%w: expected sha256:%s, got sha256:%s",
		types.ErrChecksumMismatch, strings.Repeat("a", 64), strings.Repeat("b", 64))

	updated, _ := m.Update(types.DownloadEvent{
		Type:       types.EventError,
		DownloadID: "dl-1",
		Filename:   "file.bin",
		Err:        failure,
	})
	got := unwrapRootModel(t, updated)

	row := got.downloads[0]
	if row.err == nil || !strings.Contains(row.err.Error(), "checksum mismatch") {
		t.Fatalf("row error = %v, want checksum mismatch", row.err)
	}
	if row.done || row.paused {
		t.Fatal("failed verification must leave the download errored, not done or paused")
	}

	if len(got.logEntries) == 0 {
		t.Fatal("expected an error log entry")
	}
	last := got.logEntries[len(got.logEntries)-1]
	if !strings.Contains(last, "file.bin") || !strings.Contains(last, "checksum mismatch") {
		t.Fatalf("log entry = %q, want filename and cause", last)
	}
}
