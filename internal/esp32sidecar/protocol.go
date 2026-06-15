package esp32sidecar

import (
	"fmt"
	"strings"
	"time"

	"github.com/vxider/codex-buddy/internal/model"
)

const ProtocolVersion = 1

type Frame struct {
	State         model.State
	StateDetail   string
	LED           string
	SessionsCount int
	Summary       string
	Dance         bool
	Seq           uint8
}

func Encode(frame Frame) []byte {
	state := normalizeState(frame.State)
	led := normalizeLED(frame.LED)
	detail := sanitizeField(frame.StateDetail)
	summary := sanitizeField(frame.Summary)
	line := fmt.Sprintf("CB1 state=%s led=%s detail=%s sessions=%d summary=%s\n", state, led, detail, frame.SessionsCount, summary)
	return []byte(line)
}

func FrameFromStatus(status Status) Frame {
	return Frame{
		State:         status.OverallState,
		StateDetail:   status.OverallStateDetail,
		LED:           SignalLED(status),
		SessionsCount: status.SessionsCount,
		Summary:       firstSummary(status.Sessions),
	}
}

func normalizeState(state model.State) model.State {
	switch state {
	case model.StateRunning, model.StateRunningBash:
		return model.StateRun
	case model.StateIdle, model.StateRun, model.StateOpen, model.StateError, model.StateOffline:
		return state
	default:
		return model.StateOffline
	}
}

func SignalLED(status Status) string {
	signal := "off"
	for _, session := range status.Sessions {
		signal = strongerSignal(signal, sessionSignal(session))
	}
	if signal == "off" && normalizeState(status.OverallState) == model.StateIdle {
		return "green"
	}
	return signalColor(signal)
}

func sessionSignal(session Session) string {
	state := strings.ToLower(strings.TrimSpace(string(session.State)))
	detail := strings.ToLower(strings.TrimSpace(session.StateDetail))
	reason := strings.ToLower(strings.TrimSpace(session.OpenReason))
	goalState := strings.ToLower(strings.TrimSpace(string(session.GoalState)))
	if goalState == "achieved" || goalState == "complete" || goalState == "completed" || goalState == "done" || goalState == "success" || goalState == "succeeded" {
		return "goal"
	}
	if session.NeedsApproval || reason == "approval" {
		return "approval"
	}
	if strings.Contains(detail, "permissionrequest") || strings.Contains(detail, "permission request") {
		return "approval"
	}
	if reason == "followup" && session.NeedsOpen && !session.NeedsApproval {
		return "attention"
	}
	if state != "idle" && state != "ready" && state != "run" && state != "running" && state != "running_bash" && (session.NeedsOpen || reason != "") {
		return "attention"
	}
	if state == "run" || state == "running" || state == "running_bash" {
		return "working"
	}
	return "off"
}

func strongerSignal(current, candidate string) string {
	priority := map[string]int{"off": 0, "working": 10, "attention": 20, "approval": 30, "goal": 40}
	if priority[candidate] > priority[current] {
		return candidate
	}
	return current
}

func signalColor(signal string) string {
	switch signal {
	case "approval":
		return "red"
	case "attention":
		return "yellow"
	case "working":
		return "green"
	case "goal":
		return "purple"
	default:
		return "off"
	}
}

func normalizeLED(led string) string {
	switch strings.ToLower(strings.TrimSpace(led)) {
	case "red", "approval", "error":
		return "red"
	case "yellow", "attention", "open":
		return "yellow"
	case "green", "working", "idle":
		return "green"
	case "purple", "violet", "goal", "blocked":
		return "purple"
	default:
		return "off"
	}
}

func EncodeBLE(frame Frame) []byte {
	flags := byte(0)
	if frame.Dance {
		flags |= 1
	}
	return []byte{'C', 'B', byte(ProtocolVersion), ledCode(frame.LED), stateCode(frame.State), flags, frame.Seq}
}

func ledCode(led string) byte {
	switch normalizeLED(led) {
	case "red":
		return 1
	case "yellow":
		return 2
	case "green":
		return 3
	case "purple":
		return 4
	default:
		return 0
	}
}

func stateCode(state model.State) byte {
	switch normalizeState(state) {
	case model.StateIdle:
		return 1
	case model.StateRun:
		return 2
	case model.StateOpen:
		return 3
	case model.StateError:
		return 4
	default:
		return 0
	}
}

func sanitizeField(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "|", " ")
	if len(value) > 96 {
		value = value[:96]
	}
	if value == "" {
		return "-"
	}
	return value
}

func StatusFromSnapshot(snapshot model.StatusSnapshot) Status {
	status := Status{
		ServerTime:         snapshot.ServerTime,
		OverallState:       snapshot.OverallState,
		OverallStateDetail: snapshot.OverallStateDetail,
		SessionsCount:      snapshot.SessionsCount,
		Sessions:           make([]Session, 0, len(snapshot.Sessions)),
	}
	for _, session := range snapshot.Sessions {
		status.Sessions = append(status.Sessions, Session{
			SessionID:     session.SessionID,
			DisplayTitle:  session.CWD,
			State:         session.State,
			StateDetail:   session.StateDetail,
			Summary:       session.LastUserPromptPreview,
			OpenSummary:   session.LastAssistantMessage,
			NeedsOpen:     session.State == model.StateOpen || session.State == model.StateError,
			NeedsApproval: strings.Contains(strings.ToLower(session.StateDetail), "permission"),
			GoalState:     session.GoalState,
			GoalUpdatedAt: session.GoalUpdatedAt,
		})
	}
	return status
}

func firstSummary(sessions []Session) string {
	for _, session := range sessions {
		for _, value := range []string{session.OpenSummary, session.Summary, session.DisplayTitle, session.SessionID} {
			value = strings.TrimSpace(value)
			if value != "" {
				return value
			}
		}
	}
	return ""
}

type Status struct {
	ServerTime         time.Time   `json:"server_time"`
	OverallState       model.State `json:"overall_state"`
	OverallStateDetail string      `json:"overall_state_detail,omitempty"`
	SessionsCount      int         `json:"sessions_count"`
	Sessions           []Session   `json:"sessions"`
}

type Session struct {
	SessionID     string          `json:"session_id"`
	DisplayTitle  string          `json:"display_title,omitempty"`
	State         model.State     `json:"state"`
	StateDetail   string          `json:"state_detail,omitempty"`
	Summary       string          `json:"summary,omitempty"`
	OpenSummary   string          `json:"open_summary,omitempty"`
	OpenReason    string          `json:"open_reason,omitempty"`
	NeedsOpen     bool            `json:"needs_open,omitempty"`
	NeedsApproval bool            `json:"needs_approval,omitempty"`
	GoalState     model.GoalState `json:"goal_state,omitempty"`
	GoalUpdatedAt time.Time       `json:"goal_updated_at,omitempty"`
}
