package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/SurgeDM/Surge/internal/config"
)

func limitTestModel(maxActive int, downloads []*DownloadModel) RootModel {
	settings := config.DefaultSettings()
	settings.Network.MaxConcurrentDownloads.Value = maxActive
	return RootModel{
		state:     DashboardState,
		activeTab: TabQueued,
		downloads: downloads,
		Settings:  settings,
		keys:      config.DefaultKeyMap(),
		list:      NewDownloadList(80, 20),
	}
}

func logContains(m RootModel, want string) bool {
	for _, entry := range m.logEntries {
		if strings.Contains(entry, want) {
			return true
		}
	}
	return false
}

func TestResumeBlockedAtActiveLimit(t *testing.T) {
	activeA := NewDownloadModel("active-a", "https://example.com/a", "a.bin", 100)
	activeA.started = true
	activeA.Speed = 1024
	activeB := NewDownloadModel("active-b", "https://example.com/b", "b.bin", 100)
	activeB.started = true
	activeB.Speed = 1024
	paused := NewDownloadModel("paused", "https://example.com/c", "c.bin", 100)
	paused.paused = true

	svc := &errorResumeService{}
	m := limitTestModel(2, []*DownloadModel{activeA, activeB, paused})
	m.Service = svc
	m.UpdateListItems()
	m.list.Select(0)

	updated, _ := m.updateDashboard(tea.KeyPressMsg{Code: 'p', Text: "p"})
	got := updated.(RootModel)

	if svc.resumedID != "" {
		t.Fatalf("Resume called at the limit for %q", svc.resumedID)
	}
	if !paused.paused || paused.resuming {
		t.Fatalf("blocked resume changed state: paused=%v resuming=%v", paused.paused, paused.resuming)
	}
	if !logContains(got, "Max active downloads reached (2)") {
		t.Fatalf("missing active-limit warning in log: %v", got.logEntries)
	}
}

func TestResumeAllowedBelowActiveLimit(t *testing.T) {
	active := NewDownloadModel("active", "https://example.com/a", "a.bin", 100)
	active.started = true
	active.Speed = 1024
	paused := NewDownloadModel("paused", "https://example.com/b", "b.bin", 100)
	paused.paused = true

	svc := &errorResumeService{}
	m := limitTestModel(2, []*DownloadModel{active, paused})
	m.Service = svc
	m.UpdateListItems()
	m.list.Select(0)

	updated, _ := m.updateDashboard(tea.KeyPressMsg{Code: 'p', Text: "p"})
	got := updated.(RootModel)

	if svc.resumedID != paused.ID {
		t.Fatalf("Resume called with %q, want %q", svc.resumedID, paused.ID)
	}
	if paused.paused || !paused.resuming || paused.started {
		t.Fatalf("successful resume state = paused:%v resuming:%v started:%v", paused.paused, paused.resuming, paused.started)
	}
	if downloadTab(paused) != TabQueued {
		t.Fatal("resume waiting for a slot must stay in the Queue tab")
	}
	if logContains(got, "Max active downloads reached") {
		t.Fatal("unexpected active-limit warning below the limit")
	}
}

func TestRestartQueuedDownloadBlockedAtActiveLimit(t *testing.T) {
	activeA := NewDownloadModel("active-a", "https://example.com/a", "a.bin", 100)
	activeA.started = true
	activeA.Speed = 1024
	activeB := NewDownloadModel("active-b", "https://example.com/b", "b.bin", 100)
	activeB.started = true
	activeB.Speed = 1024
	queued := NewDownloadModel("queued", "https://example.com/c", "c.bin", 100)

	svc := &mockService{}
	m := limitTestModel(2, []*DownloadModel{activeA, activeB, queued})
	m.Service = svc
	m.UpdateListItems()
	m.list.Select(0)

	updated, _ := m.updateDashboard(tea.KeyPressMsg{Code: 'R', Text: "R"})
	got := updated.(RootModel)

	if got.state == RestartDownloadConfirmState || got.restartTargetID != "" {
		t.Fatal("restart confirmation opened at the active-download limit")
	}
	if !logContains(got, "Max active downloads reached (2)") {
		t.Fatalf("missing active-limit warning in log: %v", got.logEntries)
	}
}

func TestRestartActiveDownloadAllowedAtActiveLimit(t *testing.T) {
	activeA := NewDownloadModel("active-a", "https://example.com/a", "a.bin", 100)
	activeA.started = true
	activeA.Speed = 1024
	activeB := NewDownloadModel("active-b", "https://example.com/b", "b.bin", 100)
	activeB.started = true
	activeB.Speed = 1024

	m := limitTestModel(2, []*DownloadModel{activeA, activeB})
	m.Service = &mockService{}
	m.activeTab = TabActive
	m.UpdateListItems()
	m.list.Select(0)

	updated, _ := m.updateDashboard(tea.KeyPressMsg{Code: 'R', Text: "R"})
	got := updated.(RootModel)

	if got.state != RestartDownloadConfirmState || got.restartTargetID == "" {
		t.Fatalf("restarting an active download should not be blocked: state=%v target=%q", got.state, got.restartTargetID)
	}
}
