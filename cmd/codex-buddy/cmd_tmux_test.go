package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vxider/codex-buddy/internal/model"
)

func TestUpdateTmuxDotStateMarksStoppedUnreadAndClearsOnActiveWindow(t *testing.T) {
	now := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	state := map[string]tmuxDotState{}

	updateTmuxDotState(state, map[string]tmuxWindowState{"@7": tmuxStateGreen}, "", now)
	if !state["@7"].WasRunning {
		t.Fatalf("expected @7 to record running state")
	}
	if state["@7"].StoppedUnread {
		t.Fatalf("did not expect stopped unread while still running")
	}

	updateTmuxDotState(state, map[string]tmuxWindowState{}, "@8", now.Add(time.Second))
	if !state["@7"].StoppedUnread {
		t.Fatalf("expected @7 stopped unread after running stopped")
	}

	updateTmuxDotState(state, map[string]tmuxWindowState{}, "@7", now.Add(2*time.Second))
	if state["@7"].StoppedUnread {
		t.Fatalf("expected @7 stopped unread to clear when active")
	}
}

func TestUpdateTmuxDotStateClearsStoppedUnreadWhenWindowRunsAgain(t *testing.T) {
	now := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	state := map[string]tmuxDotState{
		"@7": {
			WasRunning:    true,
			StoppedUnread: true,
			UpdatedAt:     now,
		},
	}

	updateTmuxDotState(state, map[string]tmuxWindowState{"@7": tmuxStateGreen}, "@8", now.Add(time.Second))
	if state["@7"].StoppedUnread {
		t.Fatalf("expected @7 stopped unread to clear when running again")
	}
	if !state["@7"].WasRunning {
		t.Fatalf("expected @7 to remain running")
	}
}

func TestUpdateTmuxDotStateErrorDoesNotTriggerStoppedUnread(t *testing.T) {
	now := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	state := map[string]tmuxDotState{}

	// Window is running
	updateTmuxDotState(state, map[string]tmuxWindowState{"@7": tmuxStateGreen}, "", now)
	if !state["@7"].WasRunning {
		t.Fatalf("expected @7 running")
	}

	// Window transitions to error (red) - should NOT mark stopped_unread
	updateTmuxDotState(state, map[string]tmuxWindowState{"@7": tmuxStateRed}, "@8", now.Add(time.Second))
	if state["@7"].StoppedUnread {
		t.Fatalf("error state should not trigger stopped unread (yellow)")
	}
	// Error is still "active" so WasRunning should be true
	if !state["@7"].WasRunning {
		t.Fatalf("error state should keep WasRunning true")
	}
}

func TestSummarizeTmuxWindowsPriority(t *testing.T) {
	status := apiStatus{
		Sessions: []apiSession{
			// Two sessions in same window: one running (green), one error (red)
			{SessionID: "sess-err", State: model.StateError, TmuxWindow: "@24"},
			{SessionID: "sess-run", State: model.StateRunning, TmuxWindow: "@24"},
		},
	}

	result := summarizeTmuxWindows(status)
	if result["@24"] != tmuxStateRed {
		t.Fatalf("expected red (highest priority) for @24, got %v", result["@24"])
	}

	// Window with goal (purple) and normal running (green) -> purple
	status2 := apiStatus{
		Sessions: []apiSession{
			{SessionID: "sess-goal", State: model.StateRunning, TmuxWindow: "@25", GoalState: model.GoalStateInProgress},
			{SessionID: "sess-norm", State: model.StateRunning, TmuxWindow: "@25"},
		},
	}
	result2 := summarizeTmuxWindows(status2)
	if result2["@25"] != tmuxStatePurple {
		t.Fatalf("expected purple for @25, got %v", result2["@25"])
	}

	// Idle session -> no dot
	status3 := apiStatus{
		Sessions: []apiSession{
			{SessionID: "sess-idle", State: model.StateIdle, TmuxWindow: "@26"},
		},
	}
	result3 := summarizeTmuxWindows(status3)
	if result3["@26"] != tmuxStateNone {
		t.Fatalf("expected none for idle @26, got %v", result3["@26"])
	}
}

func TestSummarizeTmuxWindowsAttentionIsRed(t *testing.T) {
	status := apiStatus{
		Sessions: []apiSession{
			{SessionID: "sess-att", State: model.StateAttention, TmuxWindow: "@30"},
		},
	}
	result := summarizeTmuxWindows(status)
	if result["@30"] != tmuxStateRed {
		t.Fatalf("expected red for attention @30, got %v", result["@30"])
	}
}

func TestSummarizeTmuxWindowsNormalRunningIsGreen(t *testing.T) {
	status := apiStatus{
		Sessions: []apiSession{
			{SessionID: "sess-run", State: model.StateRunning, TmuxWindow: "@31"},
		},
	}
	result := summarizeTmuxWindows(status)
	if result["@31"] != tmuxStateGreen {
		t.Fatalf("expected green for normal running @31, got %v", result["@31"])
	}
}

func TestUpdatePersistedTmuxDotStateStoresAndClearsUnreadState(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	now := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)

	state := updatePersistedTmuxDotState(map[string]tmuxWindowState{"@7": tmuxStateGreen}, "", now)
	if !state["@7"].WasRunning {
		t.Fatalf("expected @7 to record running state")
	}

	state = updatePersistedTmuxDotState(map[string]tmuxWindowState{}, "@8", now.Add(time.Second))
	if !state["@7"].StoppedUnread {
		t.Fatalf("expected @7 stopped unread after running stopped")
	}

	state = updatePersistedTmuxDotState(map[string]tmuxWindowState{}, "@7", now.Add(2*time.Second))
	if state["@7"].StoppedUnread {
		t.Fatalf("expected @7 stopped unread to clear when active")
	}

	path, err := tmuxDotStatePath()
	if err != nil {
		t.Fatalf("tmuxDotStatePath: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected state file to exist: %v", err)
	}

	if matches, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".tmux-window-dots-*.tmp")); err != nil || len(matches) != 0 {
		t.Fatalf("expected no temp files left, found %v", matches)
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
