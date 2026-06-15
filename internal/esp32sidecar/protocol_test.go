package esp32sidecar

import (
	"strings"
	"testing"

	"github.com/vxider/codex-buddy/internal/model"
)

func TestEncodeNormalizesState(t *testing.T) {
	got := string(Encode(Frame{State: model.StateRunningBash, StateDetail: "running_bash", SessionsCount: 2, Summary: "build\nok"}))
	if !strings.HasPrefix(got, "CB1 state=run led=off detail=running_bash sessions=2 summary=build ok\n") {
		t.Fatalf("unexpected frame: %q", got)
	}
}

func TestFrameFromStatusUsesOpenSummary(t *testing.T) {
	frame := FrameFromStatus(Status{
		OverallState:  model.StateOpen,
		SessionsCount: 1,
		Sessions: []Session{{
			SessionID:   "sess-1",
			Summary:     "regular",
			OpenSummary: "needs input",
		}},
	})
	if frame.Summary != "needs input" {
		t.Fatalf("expected open summary, got %q", frame.Summary)
	}
}

func TestSignalLEDMatchesUConsoleHelperPriority(t *testing.T) {
	status := Status{
		OverallState:  model.StateRun,
		SessionsCount: 2,
		Sessions: []Session{
			{SessionID: "run", State: model.StateRun},
			{SessionID: "approval", State: model.StateOpen, NeedsApproval: true},
		},
	}
	if got := SignalLED(status); got != "red" {
		t.Fatalf("expected red approval signal, got %q", got)
	}

	status.Sessions = []Session{{SessionID: "goal", State: model.StateIdle, GoalState: model.GoalStateAchieved}}
	if got := SignalLED(status); got != "purple" {
		t.Fatalf("expected purple goal signal, got %q", got)
	}
}
