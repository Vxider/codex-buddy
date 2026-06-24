package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vxider/codex-buddy/internal/model"
)

func TestUpdateTmuxDotStateMarksDownUnreadAndClearsOnActiveWindow(t *testing.T) {
	now := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	state := map[string]tmuxDotWindowState{}

	updateTmuxDotState(state, map[string]bool{"@7": true}, "", now)
	if !state["@7"].WasRunning {
		t.Fatalf("expected @7 to record running state")
	}
	if state["@7"].DownUnread {
		t.Fatalf("did not expect down unread while still running")
	}

	updateTmuxDotState(state, map[string]bool{}, "@8", now.Add(time.Second))
	if !state["@7"].DownUnread {
		t.Fatalf("expected @7 down unread after running stopped")
	}

	updateTmuxDotState(state, map[string]bool{}, "@7", now.Add(2*time.Second))
	if state["@7"].DownUnread {
		t.Fatalf("expected @7 down unread to clear when active")
	}
}

func TestUpdateTmuxDotStateClearsDownUnreadWhenWindowRunsAgain(t *testing.T) {
	now := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	state := map[string]tmuxDotWindowState{
		"@7": {
			WasRunning: true,
			DownUnread: true,
			UpdatedAt:  now,
		},
	}

	updateTmuxDotState(state, map[string]bool{"@7": true}, "@8", now.Add(time.Second))
	if state["@7"].DownUnread {
		t.Fatalf("expected @7 down unread to clear when running again")
	}
	if !state["@7"].WasRunning {
		t.Fatalf("expected @7 to remain running")
	}
}

func TestSummarizeTmuxWindowsAggregatesRunningGoalState(t *testing.T) {
	status := apiStatus{
		Sessions: []apiSession{
			{SessionID: "sess-goal", State: model.StateRunning, TmuxWindow: "@24", GoalState: model.GoalStateInProgress},
			{SessionID: "sess-idle", State: model.StateIdle, TmuxWindow: "@25"},
			{SessionID: "sess-other", State: model.StateRunning, TmuxWindow: ""},
		},
	}

	running, goal := summarizeTmuxWindows(status)
	if !running["@24"] {
		t.Fatalf("expected @24 running")
	}
	if running["@25"] {
		t.Fatalf("did not expect @25 running")
	}
	if !goal["@24"] {
		t.Fatalf("expected @24 goal")
	}
	if goal["@25"] {
		t.Fatalf("did not expect @25 goal")
	}
}

func TestUpdatePersistedTmuxDotStateStoresAndClearsUnreadState(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	now := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)

	state := updatePersistedTmuxDotState(map[string]bool{"@7": true}, "", now)
	if !state["@7"].WasRunning {
		t.Fatalf("expected @7 to record running state")
	}

	state = updatePersistedTmuxDotState(map[string]bool{}, "@8", now.Add(time.Second))
	if !state["@7"].DownUnread {
		t.Fatalf("expected @7 down unread after running stopped")
	}

	state = updatePersistedTmuxDotState(map[string]bool{}, "@7", now.Add(2*time.Second))
	if state["@7"].DownUnread {
		t.Fatalf("expected @7 down unread to clear when active")
	}

	path, err := tmuxDotStatePath()
	if err != nil {
		t.Fatalf("tmuxDotStatePath: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected state file at %s: %v", path, err)
	}
	if matches, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".tmux-window-dots-*.tmp")); err != nil || len(matches) != 0 {
		t.Fatalf("expected no temp state files, got matches=%v err=%v", matches, err)
	}
}

func TestSafeSQLiteLiteralIDRejectsUnsafeValues(t *testing.T) {
	for _, value := range []string{"019ebb43-5937-7013-8da0-4365e9d75e68", "abc_DEF-123"} {
		if !safeSQLiteLiteralID(value) {
			t.Fatalf("expected safe id %q", value)
		}
	}
	for _, value := range []string{"", "abc'def", "abc;select", "abc/def"} {
		if safeSQLiteLiteralID(value) {
			t.Fatalf("expected unsafe id %q", value)
		}
	}
}
