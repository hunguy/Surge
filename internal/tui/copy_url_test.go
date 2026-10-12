package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/SurgeDM/Surge/internal/config"
)

func newCopyURLModel(tab int, downloads []*DownloadModel) RootModel {
	m := RootModel{
		state:       DashboardState,
		activeTab:   tab,
		downloads:   downloads,
		list:        NewDownloadList(80, 20),
		logViewport: viewport.New(viewport.WithWidth(40), viewport.WithHeight(5)),
		keys:        config.DefaultKeyMap(),
		Settings:    config.DefaultSettings(),
		// Skip the on-disk keymap reload so the test stays deterministic.
		lastConfigCheckTime: time.Now(),
	}
	m.UpdateListItems()
	return m
}

func TestUpdate_CopyURLFromAnyTab(t *testing.T) {
	original := writeDownloadURLClipboard
	defer func() { writeDownloadURLClipboard = original }()

	cases := []struct {
		name string
		tab  int
		dl   *DownloadModel
	}{
		{"queued", TabQueued, &DownloadModel{ID: "q1", URL: "https://example.com/queued.zip", Filename: "queued.zip", paused: true}},
		{"active", TabActive, &DownloadModel{ID: "a1", URL: "https://example.com/active.zip", Filename: "active.zip", Speed: 1024}},
		{"done", TabDone, &DownloadModel{ID: "d1", URL: "https://example.com/done.zip", Filename: "done.zip", done: true}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var copied []string
			writeDownloadURLClipboard = func(text string) error {
				copied = append(copied, text)
				return nil
			}

			m := newCopyURLModel(tc.tab, []*DownloadModel{tc.dl})
			if len(m.list.Items()) != 1 {
				t.Fatalf("expected 1 selectable item on %s tab, got %d", tc.name, len(m.list.Items()))
			}
			m.list.Select(0)

			updated, _ := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
			_ = unwrapRootModel(t, updated)

			if len(copied) != 1 || copied[0] != tc.dl.URL {
				t.Fatalf("copied = %v, want [%q]", copied, tc.dl.URL)
			}
		})
	}
}

func TestUpdate_CopyURLClipboardFailureLogsError(t *testing.T) {
	original := writeDownloadURLClipboard
	defer func() { writeDownloadURLClipboard = original }()

	writeDownloadURLClipboard = func(string) error {
		return errors.New("clipboard unavailable")
	}

	m := newCopyURLModel(TabQueued, []*DownloadModel{
		{ID: "q1", URL: "https://example.com/file.zip", Filename: "file.zip", paused: true},
	})
	m.list.Select(0)

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	m2 := unwrapRootModel(t, updated)

	if len(m2.logEntries) == 0 {
		t.Fatal("expected a log entry when clipboard write fails")
	}
	last := m2.logEntries[len(m2.logEntries)-1]
	if !strings.Contains(last, "Copy failed") {
		t.Fatalf("last log entry = %q, want clipboard failure message", last)
	}
}

func TestUpdate_CopyURLWithNoSelectionIsNoOp(t *testing.T) {
	original := writeDownloadURLClipboard
	defer func() { writeDownloadURLClipboard = original }()

	copied := false
	writeDownloadURLClipboard = func(string) error {
		copied = true
		return nil
	}

	m := newCopyURLModel(TabQueued, nil)

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	_ = unwrapRootModel(t, updated)

	if copied {
		t.Fatal("clipboard should not be written when nothing is selected")
	}
}
