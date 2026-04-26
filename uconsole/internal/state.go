package uconsole

import (
	"strings"

	"github.com/vxider/codex-buddy/internal/model"
)

func normalizeCompatState(state model.State) model.State {
	switch strings.ToLower(strings.TrimSpace(string(state))) {
	case "attention":
		return model.StateOpen
	default:
		return state
	}
}

func normalizeSessionState(state model.State) model.State {
	state = normalizeCompatState(state)
	switch state {
	case "", model.StateOffline:
		return model.StateIdle
	default:
		return state
	}
}

func normalizeCompatNotificationKind(kind model.NotificationKind) model.NotificationKind {
	switch strings.ToLower(strings.TrimSpace(string(kind))) {
	case "attention":
		return model.NotificationOpen
	default:
		return kind
	}
}
