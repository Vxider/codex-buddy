package uconsole

import (
	"strings"

	"github.com/vxider/codex-buddy/internal/model"
)

func normalizeCompatState(state model.State) model.State {
	return model.State(strings.ToLower(strings.TrimSpace(string(state))))
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
