# uConsole hardware notes

This document summarizes the hardware assumptions behind the uConsole companion setup used by `codex-buddy`.

## Purpose

The uConsole build is meant to be a dedicated sidecar device for Codex CLI status.

Typical responsibilities:

- show aggregate status at a glance
- surface attention and error states clearly
- provide a simple path to trigger `continue`
- optionally drive a small WS2812 LED strip for ambient state feedback

## Reference hardware stack

The project has been developed around a CM5-based uConsole-style setup with:

- Linux-based handheld host
- local `codex-buddy` or remote access over Tailscale/internal network
- optional WS2812 LED strip
- optional external keyboard shortcuts for acknowledge/continue flows

## Design constraints

The hardware integration should stay pragmatic:

- the terminal remains the primary Codex interface
- uConsole is a side display, not a terminal replacement
- LED output should be informative, not distracting
- the continue action must remain explicit and safe

## LED guidance

Recommended default LED assumptions:

- `GPIO18`
- `8 x WS2812`
- modest brightness suitable for handheld indoor use

Suggested visual mapping:

- `offline`: dark or neutral idle
- `idle`: calm green
- `running`: steady blue or soft movement
- `attention`: slow warm scan or pulse
- `error`: strong red emphasis

The key rule is that `attention` must be clearly noticeable without looking like a critical alarm.

## Continue interaction

The uConsole flow is intentionally simple:

- attention creates a visible primary card
- the user can acknowledge it
- the user can trigger a single `continue` action
- long-press confirmation is preferred over accidental single-tap activation

## Deployment notes

- build the GUI variant with `-tags uconsole_gui`
- add `ws281x` only when the hardware stack is present and the host has the required libraries
- keep server and GUI builds separable so headless development remains easy

## Practical recommendation

Treat the uConsole hardware as a focused companion surface:

- status display first
- low-friction continue action second
- full debugging still belongs in the main SSH/tmux terminal session
