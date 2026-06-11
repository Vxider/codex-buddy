//go:build uconsole_gui

package uconsole

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/vxider/codex-buddy/internal/model"
)

const ledStatusStaleAfter = 60 * time.Second

type codexLEDState string

const (
	codexLEDOff       codexLEDState = "off"
	codexLEDWorking   codexLEDState = "working"
	codexLEDAttention codexLEDState = "attention"
	codexLEDApproval  codexLEDState = "approval"
	codexLEDGoal      codexLEDState = "goal"
)

type codexLEDStatus struct {
	State     codexLEDState `json:"state"`
	UpdatedAt float64       `json:"updated_at"`
	Connected bool          `json:"connected"`
	Source    string        `json:"source"`
}

func codexLEDStatusPath() string {
	runtimeDir := strings.TrimSpace(os.Getenv("XDG_RUNTIME_DIR"))
	if runtimeDir == "" {
		runtimeDir = os.TempDir()
	}
	return filepath.Join(runtimeDir, "uconsole-helper", "codex-led.json")
}

func writeCodexLEDStatus(state codexLEDState, connected bool) error {
	path := codexLEDStatusPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	payload := codexLEDStatus{
		State:     state,
		UpdatedAt: float64(time.Now().UnixNano()) / float64(time.Second),
		Connected: connected,
		Source:    "codex-buddy",
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func codexLEDStateFromStatus(status StatusResponse, connected bool) codexLEDState {
	if !connected {
		return codexLEDOff
	}
	state := codexLEDOff
	if len(status.Sessions) > 0 {
		for _, session := range status.Sessions {
			state = strongerCodexLEDState(state, codexLEDStateFromSession(session))
		}
		return state
	}
	return codexLEDStateFromOverall(status.OverallState)
}

func codexLEDStateFromSession(session SessionResponse) codexLEDState {
	state := normalizeSessionState(session.State)
	detail := strings.ToLower(strings.TrimSpace(session.StateDetail))
	reason := strings.ToLower(strings.TrimSpace(session.OpenReason))
	if sessionHasRecentGoal(session) {
		return codexLEDGoal
	}
	if session.NeedsApproval || reason == "approval" {
		return codexLEDApproval
	}
	if strings.Contains(detail, "permissionrequest") || strings.Contains(detail, "permission request") {
		return codexLEDApproval
	}
	if reason == "followup" && session.NeedsOpen && !session.NeedsApproval {
		return codexLEDAttention
	}
	if state != model.StateIdle && state != model.StateRun && state != model.StateRunningBash && (session.NeedsOpen || reason != "") {
		return codexLEDAttention
	}
	if state == model.StateRun || state == model.StateRunningBash {
		return codexLEDWorking
	}
	return codexLEDOff
}

func sessionHasRecentGoal(session SessionResponse) bool {
	if session.GoalState != model.GoalStateAchieved {
		return false
	}
	if session.GoalUpdatedAt.IsZero() || session.UpdatedAt.IsZero() {
		return !session.GoalUpdatedAt.IsZero()
	}
	return session.UpdatedAt.Sub(session.GoalUpdatedAt) <= ledStatusStaleAfter*5
}

func codexLEDStateFromOverall(state model.State) codexLEDState {
	switch normalizeCompatState(state) {
	case model.StateRun, model.StateRunningBash:
		return codexLEDWorking
	case model.StateAttention:
		return codexLEDAttention
	default:
		return codexLEDOff
	}
}

func strongerCodexLEDState(current, candidate codexLEDState) codexLEDState {
	if codexLEDStateRank(candidate) > codexLEDStateRank(current) {
		return candidate
	}
	return current
}

func codexLEDStateRank(state codexLEDState) int {
	switch state {
	case codexLEDGoal:
		return 40
	case codexLEDApproval:
		return 30
	case codexLEDAttention:
		return 20
	case codexLEDWorking:
		return 10
	default:
		return 0
	}
}

func validateCodexLEDState(state codexLEDState) error {
	switch state {
	case codexLEDOff, codexLEDWorking, codexLEDAttention, codexLEDApproval, codexLEDGoal:
		return nil
	default:
		return fmt.Errorf("invalid codex LED state %q", state)
	}
}
