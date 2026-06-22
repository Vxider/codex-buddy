package model

import "time"

type State string

const (
	StateOffline     State = "offline"
	StateIdle        State = "idle"
	StateRun         State = "run"
	StateRunning     State = "running"
	StateRunningBash State = "running_bash"
	StateOpen        State = "open"
	StateAttention   State = StateOpen
	StateError       State = "error"
)

type GoalState string

const (
	GoalStateUnknown    GoalState = "unknown"
	GoalStateInProgress GoalState = "in_progress"
	GoalStateAchieved   GoalState = "achieved"
	GoalStateBlocked    GoalState = "blocked"
	GoalStateNeedsUser  GoalState = "needs_user"
)

type HookPayload struct {
	HookEventName        string `json:"hook_event_name"`
	SessionID            string `json:"session_id"`
	TurnID               string `json:"turn_id"`
	CWD                  string `json:"cwd"`
	Model                string `json:"model"`
	TmuxPane             string `json:"tmux_pane,omitempty"`
	TmuxSession          string `json:"tmux_session,omitempty"`
	TmuxWindow           string `json:"tmux_window,omitempty"`
	TranscriptPath       string `json:"transcript_path"`
	Prompt               string `json:"prompt"`
	LastAssistantMessage string `json:"last_assistant_message"`
	ToolName             string `json:"tool_name"`
	ToolInput            any    `json:"tool_input"`
	Error                string `json:"error"`
	CodexPID             int    `json:"codex_pid,omitempty"`
	ApprovalsReviewer    string `json:"approvals_reviewer,omitempty"`
}

type IngestRequest struct {
	Source        string      `json:"source"`
	EventName     string      `json:"event_name"`
	HookEventName string      `json:"hook_event_name"`
	ReceivedAt    time.Time   `json:"received_at"`
	Payload       HookPayload `json:"payload"`
}

type SessionSnapshot struct {
	SessionID                string    `json:"session_id"`
	TurnID                   string    `json:"turn_id,omitempty"`
	CWD                      string    `json:"cwd,omitempty"`
	Model                    string    `json:"model,omitempty"`
	State                    State     `json:"state"`
	StateDetail              string    `json:"state_detail,omitempty"`
	LastUserPromptPreview    string    `json:"last_user_prompt_preview,omitempty"`
	LastAssistantMessage     string    `json:"last_assistant_message,omitempty"`
	LastAssistantMessageFull string    `json:"-"`
	LastBashCommand          string    `json:"last_bash_command,omitempty"`
	TranscriptPath           string    `json:"transcript_path,omitempty"`
	LastError                string    `json:"-"`
	UpdatedAt                time.Time `json:"updated_at"`
	TmuxPane                 string    `json:"-"`
	TmuxSession              string    `json:"-"`
	TmuxWindow               string    `json:"-"`
	CurrentAttentionDeadline time.Time `json:"-"`
	PendingApprovalAt        time.Time `json:"-"`
	GoalState                GoalState `json:"goal_state,omitempty"`
	GoalSummary              string    `json:"goal_summary,omitempty"`
	GoalUpdatedAt            time.Time `json:"goal_updated_at,omitempty"`
}

type RecentEvent struct {
	Time      time.Time `json:"time"`
	Source    string    `json:"source"`
	SessionID string    `json:"session_id,omitempty"`
	EventName string    `json:"event_name,omitempty"`
	Summary   string    `json:"summary,omitempty"`
}

type StatusSnapshot struct {
	ServerTime         time.Time         `json:"server_time"`
	OverallState       State             `json:"overall_state"`
	OverallStateDetail string            `json:"overall_state_detail,omitempty"`
	ActiveSessionID    string            `json:"active_session_id,omitempty"`
	SessionsCount      int               `json:"sessions_count"`
	Sessions           []SessionSnapshot `json:"sessions"`
	RecentEvents       []RecentEvent     `json:"recent_events,omitempty"`
	TranscriptWatchers map[string]string `json:"transcript_watchers,omitempty"`
	GoalState          GoalState         `json:"goal_state,omitempty"`
	GoalSummary        string            `json:"goal_summary,omitempty"`
	GoalUpdatedAt      time.Time         `json:"goal_updated_at,omitempty"`
}

type TranscriptUpdate struct {
	SessionID             string
	TurnID                string
	CWD                   string
	Model                 string
	LastUserPromptPreview string
	LastAssistantMessage  string
	LastBashCommand       string
	Error                 string
	UpdatedAt             time.Time
	GoalUpdated           bool
	GoalState             GoalState
	GoalSummary           string
	GoalUpdatedAt         time.Time
}

type NotificationKind string

const (
	NotificationOpen      NotificationKind = "open"
	NotificationAttention NotificationKind = NotificationOpen
	NotificationError     NotificationKind = "error"
)

type NotificationState string

const (
	NotificationPending NotificationState = "pending"
	NotificationAcked   NotificationState = "acked"
	NotificationActed   NotificationState = "acted"
	NotificationExpired NotificationState = "expired"
)

type NotificationAction string

const (
	NotificationActionAck      NotificationAction = "ack"
	NotificationActionContinue NotificationAction = "continue"
)

// ContinueCommandText is the exact prompt text sent to Codex when a user
// confirms a continue action. It is centralized here so the transport and
// tests share one source of truth.
const ContinueCommandText = "继续"

type NotificationSnapshot struct {
	ID          string               `json:"id"`
	SessionID   string               `json:"session_id"`
	Kind        NotificationKind     `json:"kind"`
	State       NotificationState    `json:"state"`
	Title       string               `json:"title"`
	Summary     string               `json:"summary"`
	CreatedAt   time.Time            `json:"created_at"`
	UpdatedAt   time.Time            `json:"updated_at"`
	ActionToken string               `json:"action_token,omitempty"`
	Actions     []NotificationAction `json:"actions,omitempty"`
}
