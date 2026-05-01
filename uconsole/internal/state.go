package uconsole

import (
	"strings"

	"github.com/vxider/codex-buddy/internal/model"
)

func normalizeCompatState(state model.State) model.State {
	state = model.State(strings.ToLower(strings.TrimSpace(string(state))))
	switch state {
	case model.StateRun, model.StateRunning, model.StateRunningBash:
		return model.StateRun
	default:
		return model.StateOpen
	}
}

func normalizeSessionState(state model.State) model.State {
	state = normalizeCompatState(state)
	switch state {
	case "", model.StateOffline:
		return model.StateOpen
	default:
		return state
	}
}
