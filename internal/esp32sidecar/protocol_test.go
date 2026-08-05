package esp32sidecar

import (
	"strings"
	"testing"

	"github.com/vxider/agent-buddy/internal/model"
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

func TestSignalLEDMatchesErrorPriority(t *testing.T) {
	status := Status{
		OverallState:  model.StateRun,
		SessionsCount: 2,
		Sessions: []Session{
			{SessionID: "run", State: model.StateRun},
			{SessionID: "approval", State: model.StateOpen, NeedsApproval: true},
		},
	}
	if got := SignalLED(status); got != "yellow" {
		t.Fatalf("expected yellow approval signal, got %q", got)
	}
	status.Sessions[1].Summary = "Approval required before editing billing copy"
	if got := SignalLED(status); got != "yellow" {
		t.Fatalf("expected approval summary to stay yellow, got %q", got)
	}

	status.Sessions = []Session{{SessionID: "goal", State: model.StateIdle, GoalState: model.GoalStateAchieved}}
	if got := SignalLED(status); got != "purple" {
		t.Fatalf("expected purple goal signal, got %q", got)
	}
}

func TestSignalLEDUsesRedForCodexError(t *testing.T) {
	tests := []Status{
		{OverallState: model.StateError},
		{OverallState: model.StateRun, Sessions: []Session{{State: model.StateError}}},
		{OverallState: model.StateRun, Sessions: []Session{{State: model.StateRun, Summary: "HTTP 401 Unauthorized"}}},
		{OverallState: model.StateRun, Sessions: []Session{{State: model.StateRun, Summary: "network connection timed out"}}},
	}
	for _, status := range tests {
		if got := SignalLED(status); got != "red" {
			t.Errorf("expected red Codex error signal, got %q for %#v", got, status)
		}
	}
}

func TestNormalizeLEDMovesApprovalToYellow(t *testing.T) {
	if got := normalizeLED("approval"); got != "yellow" {
		t.Fatalf("expected approval to normalize to yellow, got %q", got)
	}
	if got := normalizeLED("error"); got != "red" {
		t.Fatalf("expected error to normalize to red, got %q", got)
	}
}
