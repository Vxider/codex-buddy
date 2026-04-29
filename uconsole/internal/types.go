package uconsole

import (
	"time"

	"github.com/vxider/codex-buddy/internal/model"
)

type StatusResponse struct {
	ServerTime         time.Time         `json:"server_time"`
	OverallState       model.State       `json:"overall_state"`
	OverallStateDetail string            `json:"overall_state_detail,omitempty"`
	SessionsCount      int               `json:"sessions_count"`
	Sessions           []SessionResponse `json:"sessions"`
}

type SessionResponse struct {
	SessionID           string                `json:"session_id"`
	ShortSessionID      string                `json:"short_session_id,omitempty"`
	DisplayTitle        string                `json:"display_title,omitempty"`
	State               model.State           `json:"state"`
	StateDetail         string                `json:"state_detail,omitempty"`
	UpdatedAt           time.Time             `json:"updated_at,omitempty"`
	Summary             string                `json:"summary,omitempty"`
	SummaryMarkdown     string                `json:"summary_markdown,omitempty"`
	SummaryHTML         string                `json:"summary_html,omitempty"`
	NeedsOpen           bool                  `json:"needs_open,omitempty"`
	NeedsApproval       bool                  `json:"needs_approval,omitempty"`
	OpenReason          string                `json:"open_reason,omitempty"`
	OpenSummary         string                `json:"open_summary,omitempty"`
	OpenSummaryMarkdown string                `json:"open_summary_markdown,omitempty"`
	OpenSummaryHTML     string                `json:"open_summary_html,omitempty"`
	TmuxSession         string                `json:"tmux_session,omitempty"`
	TmuxWindow          string                `json:"tmux_window,omitempty"`
	TmuxPane            string                `json:"tmux_pane,omitempty"`
	CanContinue         bool                  `json:"can_continue,omitempty"`
	ContinueAction      *SessionActionPayload `json:"continue_action,omitempty"`
	CanClose            bool                  `json:"can_close,omitempty"`
	CloseAction         *SessionActionPayload `json:"close_action,omitempty"`
	ServerID            string                `json:"-"`
	ServerName          string                `json:"-"`
	ServerURL           string                `json:"-"`
}

type SessionActionPayload struct {
	Method      string `json:"method"`
	Endpoint    string `json:"endpoint"`
	ActionToken string `json:"action_token,omitempty"`
	Label       string `json:"label,omitempty"`
}

type NotificationsResponse struct {
	Notifications []NotificationResponse `json:"notifications"`
}

type NotificationResponse struct {
	ID          string                     `json:"id"`
	SessionID   string                     `json:"session_id"`
	Kind        model.NotificationKind     `json:"kind"`
	State       model.NotificationState    `json:"state"`
	Title       string                     `json:"title"`
	Summary     string                     `json:"summary"`
	CreatedAt   time.Time                  `json:"created_at"`
	UpdatedAt   time.Time                  `json:"updated_at"`
	ActionToken string                     `json:"action_token,omitempty"`
	Actions     []model.NotificationAction `json:"actions,omitempty"`
	ServerID    string                     `json:"-"`
	ServerName  string                     `json:"-"`
	ServerURL   string                     `json:"-"`
}

func (s StatusResponse) ToSnapshot() model.StatusSnapshot {
	sessions := make([]model.SessionSnapshot, 0, len(s.Sessions))
	for _, session := range s.Sessions {
		sessions = append(sessions, model.SessionSnapshot{
			SessionID:            session.SessionID,
			State:                normalizeSessionState(session.State),
			StateDetail:          session.StateDetail,
			LastAssistantMessage: session.Summary,
			UpdatedAt:            session.UpdatedAt,
		})
	}

	return model.StatusSnapshot{
		ServerTime:         s.ServerTime,
		OverallState:       s.OverallState,
		OverallStateDetail: s.OverallStateDetail,
		SessionsCount:      s.SessionsCount,
		Sessions:           sessions,
	}
}

func (n NotificationResponse) ToSnapshot() model.NotificationSnapshot {
	return model.NotificationSnapshot{
		ID:          n.ID,
		SessionID:   n.SessionID,
		Kind:        n.Kind,
		State:       n.State,
		Title:       n.Title,
		Summary:     n.Summary,
		CreatedAt:   n.CreatedAt,
		UpdatedAt:   n.UpdatedAt,
		ActionToken: n.ActionToken,
		Actions:     append([]model.NotificationAction(nil), n.Actions...),
	}
}
