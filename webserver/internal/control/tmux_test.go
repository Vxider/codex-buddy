package control

import (
	"io"
	"log"
	"testing"

	"github.com/vxider/codex-buddy/internal/model"
)

func TestTmuxSessionOpenCheckerHidesSessionsWithoutTmuxBinding(t *testing.T) {
	checker := NewTmuxSessionOpenChecker(log.New(io.Discard, "", 0))

	if checker.IsOpen(model.SessionSnapshot{SessionID: "sess-no-tmux"}) {
		t.Fatalf("expected session without tmux binding to be hidden")
	}
}

func TestTmuxSessionOpenCheckerRequiresPaneEvenWhenWindowExists(t *testing.T) {
	checker := NewTmuxSessionOpenChecker(log.New(io.Discard, "", 0))

	if checker.IsOpen(model.SessionSnapshot{
		SessionID:   "sess-window-only",
		TmuxWindow:  "@1",
		TmuxSession: "work",
	}) {
		t.Fatalf("expected session without tmux pane to be hidden even when window/session exist")
	}
}
