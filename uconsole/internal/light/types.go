package light

import (
	"context"
	"time"

	"github.com/vxider/codex-buddy/internal/model"
)

// Pixel is one RGB LED output value.
type Pixel struct {
	R uint8 `json:"r"`
	G uint8 `json:"g"`
	B uint8 `json:"b"`
}

// EffectMode describes how a plan should animate over time.
type EffectMode string

const (
	EffectModeOff     EffectMode = "off"
	EffectModeSolid   EffectMode = "solid"
	EffectModePulse   EffectMode = "pulse"
	EffectModeScanner EffectMode = "scanner"
	EffectModeChase   EffectMode = "chase"
	EffectModeFlash   EffectMode = "flash"
)

// Input is the full state handed to the light state machine.
// The light layer only depends on the normalized codex-buddy snapshot.
type Input struct {
	Snapshot            model.StatusSnapshot        `json:"snapshot"`
	PrimaryNotification *model.NotificationSnapshot `json:"primary_notification,omitempty"`
	Now                 time.Time                   `json:"now"`
	Pixels              int                         `json:"pixels"`
}

// Signal is the normalized semantic state the LED mapper should react to.
// It is intentionally narrower than the full snapshot but still preserves
// enough context to generate distinct effects.
type Signal struct {
	Key                   string      `json:"key"`
	State                 model.State `json:"state"`
	Detail                string      `json:"detail,omitempty"`
	ActiveSessionID       string      `json:"active_session_id,omitempty"`
	LastUserPromptPreview string      `json:"last_user_prompt_preview,omitempty"`
	LastAssistantMessage  string      `json:"last_assistant_message,omitempty"`
	LastBashCommand       string      `json:"last_bash_command,omitempty"`
	UpdatedAt             time.Time   `json:"updated_at"`
}

// Plan is the hardware-agnostic effect contract produced by the state machine.
// Drivers can render this onto WS2812, simulated LEDs, or future outputs.
type Plan struct {
	Key           string        `json:"key"`
	State         model.State   `json:"state"`
	Pixels        int           `json:"pixels"`
	Brightness    uint8         `json:"brightness"`
	Mode          EffectMode    `json:"mode"`
	FrameInterval time.Duration `json:"frame_interval"`
	Cycle         time.Duration `json:"cycle,omitempty"`
	Hold          time.Duration `json:"hold,omitempty"`
	Base          []Pixel       `json:"base,omitempty"`
	Accent        []Pixel       `json:"accent,omitempty"`
	Background    Pixel         `json:"background,omitempty"`
	Reverse       bool          `json:"reverse,omitempty"`
	Phase         time.Duration `json:"-"`
}

// Output is the full result of one state machine tick.
type Output struct {
	Signal Signal `json:"signal"`
	Plan   Plan   `json:"plan"`
}

// StateMachine converts a codex-buddy status snapshot into a stable LED plan.
// Implementations should keep restart-sensitive animations stable unless
// Signal.Key changes semantically.
type StateMachine interface {
	Next(input Input) Output
}

// Driver applies a hardware-agnostic plan to a concrete light backend.
type Driver interface {
	Apply(ctx context.Context, plan Plan) error
	Close() error
}
