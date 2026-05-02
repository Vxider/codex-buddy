package uconsole

import (
	"strings"

	"github.com/vxider/codex-buddy/internal/model"
)

func normalizeCompatState(state model.State) model.State {
	state = model.State(strings.ToLower(strings.TrimSpace(string(state))))
	switch state {
	case model.StateRun, model.StateRunning:
		return model.StateRun
	case model.StateRunningBash:
		return model.StateRunningBash
	case model.StateIdle, model.StateOpen, model.StateError, model.StateOffline:
		return state
	default:
		return model.StateIdle
	}
}

func normalizeSessionState(state model.State) model.State {
	state = normalizeCompatState(state)
	switch state {
	case "":
		return model.StateIdle
	default:
		return state
	}
}

func codexStatusLineState(state model.State, detail string) string {
	state = normalizeCompatState(state)
	detail = strings.ToLower(strings.TrimSpace(detail))
	if state == model.StateRunningBash || detail == string(model.StateRunningBash) {
		return "Working"
	}
	switch state {
	case model.StateRun, model.StateRunning:
		return "Working"
	case model.StateIdle, model.StateOpen:
		return "Ready"
	case model.StateError:
		return "Error"
	case model.StateOffline:
		return "Offline"
	default:
		return "Ready"
	}
}
