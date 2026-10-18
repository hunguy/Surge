package tui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func newRestartTestModel(d *DownloadModel) RootModel {
	m := newCopyURLModel(TabQueued, []*DownloadModel{d})
	m.Service = &mockService{restartNewID: "dl-new"}
	m.list.Select(0)
	return m
}

func TestRestartKeyAsksForConfirmation(t *testing.T) {
	d := NewDownloadModel("dl-1", "https://example.com/file.bin", "file.bin", 100)
	d.Downloaded = 40
	d.paused = true

	svc := &mockService{restartNewID: "dl-new"}
	m := newCopyURLModel(TabQueued, []*DownloadModel{d})
	m.Service = svc
	m.list.Select(0)

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'R', Text: "R"})
	got := unwrapRootModel(t, updated)

	if got.state != RestartDownloadConfirmState {
		t.Fatalf("state = %v, want RestartDownloadConfirmState", got.state)
	}
	if got.restartTargetID != "dl-1" {
		t.Fatalf("restartTargetID = %q, want dl-1", got.restartTargetID)
	}
	if got.quitConfirmFocused != 1 {
		t.Fatalf("focused button = %d, want 1 (Cancel) so a stray enter cannot wipe progress", got.quitConfirmFocused)
	}
	if svc.restartedID != "" {
		t.Fatal("restart must not start before the user confirms")
	}
	if len(got.downloads) != 1 || got.downloads[0].Downloaded != 40 {
		t.Fatal("download changed before confirmation")
	}
}

func TestRestartCancelKeepsDownload(t *testing.T) {
	d := NewDownloadModel("dl-1", "https://example.com/file.bin", "file.bin", 100)
	d.Downloaded = 40
	d.paused = true

	svc := &mockService{}
	m := newRestartTestModel(d)
	m.Service = svc

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'R', Text: "R"})
	updated, _ = unwrapRootModel(t, updated).Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	got := unwrapRootModel(t, updated)

	if got.state != DashboardState {
		t.Fatalf("state = %v, want DashboardState", got.state)
	}
	if svc.restartedID != "" {
		t.Fatal("cancelled restart must not call the service")
	}
	if len(got.downloads) != 1 || got.downloads[0].Downloaded != 40 {
		t.Fatal("cancelled restart lost the download or its progress")
	}
}

func TestRestartConfirmRequeuesFromZero(t *testing.T) {
	d := NewDownloadModel("dl-1", "https://example.com/file.bin", "file.bin", 100)
	d.Downloaded = 40
	d.paused = true

	svc := &mockService{restartNewID: "dl-new"}
	m := newRestartTestModel(d)
	m.Service = svc

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'R', Text: "R"})
	updated, cmd := unwrapRootModel(t, updated).Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	got := unwrapRootModel(t, updated)

	if got.state != DashboardState {
		t.Fatalf("state = %v, want DashboardState", got.state)
	}
	if cmd == nil {
		t.Fatal("confirming restart returned no command")
	}

	msg := cmd()
	result, ok := msg.(restartResultMsg)
	if !ok {
		t.Fatalf("command returned %T, want restartResultMsg", msg)
	}
	if result.err != nil {
		t.Fatalf("restart error: %v", result.err)
	}
	if result.oldID != "dl-1" || result.id != "dl-new" {
		t.Fatalf("result ids = %q/%q, want dl-1/dl-new", result.oldID, result.id)
	}

	updated, _ = got.Update(msg)
	got = unwrapRootModel(t, updated)

	if svc.restartedID != "dl-1" {
		t.Fatalf("service restarted %q, want dl-1", svc.restartedID)
	}
	if len(got.downloads) != 0 {
		t.Fatalf("old download row still present after restart: %d rows", len(got.downloads))
	}
	if len(got.logEntries) == 0 {
		t.Fatal("expected a restart log entry")
	}
	if last := got.logEntries[len(got.logEntries)-1]; !strings.Contains(last, "Restarted from 0: file.bin") {
		t.Fatalf("last log entry = %q, want restart confirmation", last)
	}
}

func TestRestartFailureKeepsDownloadAndLogsError(t *testing.T) {
	d := NewDownloadModel("dl-1", "https://example.com/file.bin", "file.bin", 100)
	d.Downloaded = 40
	d.paused = true

	svc := &mockService{restartErr: errors.New("destination busy")}
	m := newRestartTestModel(d)
	m.Service = svc

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'R', Text: "R"})
	updated, cmd := unwrapRootModel(t, updated).Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	got := unwrapRootModel(t, updated)

	msg := cmd()
	updated, _ = got.Update(msg)
	got = unwrapRootModel(t, updated)

	if len(got.downloads) != 1 {
		t.Fatalf("failed restart removed the download: %d rows", len(got.downloads))
	}
	last := got.logEntries[len(got.logEntries)-1]
	if !strings.Contains(last, "Restart failed") || !strings.Contains(last, "destination busy") {
		t.Fatalf("last log entry = %q, want restart failure detail", last)
	}
}

func TestRestartRejectsCompletedDownload(t *testing.T) {
	d := NewDownloadModel("dl-1", "https://example.com/file.bin", "file.bin", 100)
	d.done = true
	d.Downloaded = 100

	svc := &mockService{}
	m := newCopyURLModel(TabDone, []*DownloadModel{d})
	m.Service = svc
	m.list.Select(0)

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'R', Text: "R"})
	got := unwrapRootModel(t, updated)

	if got.state != DashboardState {
		t.Fatalf("state = %v, want DashboardState", got.state)
	}
	if svc.restartedID != "" {
		t.Fatal("completed download must not be restarted")
	}
	last := got.logEntries[len(got.logEntries)-1]
	if !strings.Contains(last, "only for unfinished downloads") {
		t.Fatalf("last log entry = %q, want completed-download rejection", last)
	}
}

func TestRestartConfirmViewMentionsProgressLoss(t *testing.T) {
	d := NewDownloadModel("dl-1", "https://example.com/file.bin", "file.bin", 100)
	m := newRestartTestModel(d)
	m.restartTargetID = d.ID
	m.state = RestartDownloadConfirmState
	m.width = 120
	m.height = 35

	view := ansiEscapeRE.ReplaceAllString(m.View().Content, "")
	for _, want := range []string{"Restart Download", "file.bin", "from 0", "Restart", "Cancel"} {
		if !strings.Contains(view, want) {
			t.Fatalf("restart modal is missing %q:\n%s", want, view)
		}
	}
}
