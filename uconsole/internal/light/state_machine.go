package light

import (
	"fmt"
	"time"

	"github.com/vxider/codex-buddy/internal/model"
)

type DefaultStateMachine struct{}

func NewDefaultStateMachine() *DefaultStateMachine {
	return &DefaultStateMachine{}
}

func (m *DefaultStateMachine) Next(input Input) Output {
	pixels := input.Pixels
	if pixels <= 0 {
		pixels = 8
	}

	state, key := effectiveState(input)
	plan := planForState(state, key, pixels)

	return Output{
		Signal: Signal{
			Key:             key,
			State:           state,
			Detail:          string(state),
			ActiveSessionID: input.Snapshot.ActiveSessionID,
			UpdatedAt:       input.Now,
		},
		Plan: plan,
	}
}

func effectiveState(input Input) (model.State, string) {
	if input.PrimaryNotification != nil {
		switch input.PrimaryNotification.Kind {
		case model.NotificationError:
			return model.StateError, fmt.Sprintf("notification:%s:error", input.PrimaryNotification.ID)
		case model.NotificationAttention:
			return model.StateAttention, fmt.Sprintf("notification:%s:attention", input.PrimaryNotification.ID)
		}
	}

	state := input.Snapshot.OverallState
	if state == "" {
		state = model.StateOffline
	}
	return state, fmt.Sprintf("state:%s:%s", state, input.Snapshot.ActiveSessionID)
}

func planForState(state model.State, key string, pixels int) Plan {
	switch state {
	case model.StateIdle:
		return Plan{
			Key:           key,
			State:         state,
			Pixels:        pixels,
			Brightness:    32,
			Mode:          EffectModeSolid,
			FrameInterval: 150 * time.Millisecond,
			Base:          []Pixel{{R: 18, G: 110, B: 72}},
			Background:    Pixel{R: 4, G: 14, B: 10},
		}
	case model.StateRunningBash:
		return Plan{
			Key:           key,
			State:         model.StateRunning,
			Pixels:        pixels,
			Brightness:    46,
			Mode:          EffectModeChase,
			FrameInterval: 85 * time.Millisecond,
			Cycle:         750 * time.Millisecond,
			Accent:        []Pixel{{R: 120, G: 220, B: 255}},
			Background:    Pixel{R: 4, G: 16, B: 34},
		}
	case model.StateRunning:
		return Plan{
			Key:           key,
			State:         state,
			Pixels:        pixels,
			Brightness:    40,
			Mode:          EffectModeChase,
			FrameInterval: 100 * time.Millisecond,
			Cycle:         900 * time.Millisecond,
			Accent:        []Pixel{{R: 54, G: 138, B: 255}},
			Background:    Pixel{R: 4, G: 12, B: 26},
		}
	case model.StateAttention:
		return Plan{
			Key:           key,
			State:         state,
			Pixels:        pixels,
			Brightness:    52,
			Mode:          EffectModeScanner,
			FrameInterval: 90 * time.Millisecond,
			Cycle:         800 * time.Millisecond,
			Accent:        []Pixel{{R: 255, G: 196, B: 54}},
			Background:    Pixel{R: 32, G: 18, B: 2},
		}
	case model.StateError:
		return Plan{
			Key:           key,
			State:         state,
			Pixels:        pixels,
			Brightness:    56,
			Mode:          EffectModePulse,
			FrameInterval: 90 * time.Millisecond,
			Cycle:         1100 * time.Millisecond,
			Accent:        []Pixel{{R: 255, G: 88, B: 72}},
			Background:    Pixel{R: 24, G: 2, B: 2},
		}
	default:
		return Plan{
			Key:           key,
			State:         model.StateOffline,
			Pixels:        pixels,
			Brightness:    0,
			Mode:          EffectModeOff,
			FrameInterval: 200 * time.Millisecond,
			Background:    Pixel{},
		}
	}
}
