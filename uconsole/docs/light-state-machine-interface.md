# uConsole light state machine interface

This document defines the interface between the Codex status snapshot and the LED rendering layer used by the uConsole client.

## Goal

The LED layer should not invent its own understanding of Codex state. It should consume the same aggregate snapshot already exposed by `codex-buddy` and render a stable visual plan from it.

## Input contract

The renderer should work from the same public state vocabulary used elsewhere:

- `offline`
- `idle`
- `running`
- `running_bash`
- `attention`
- `error`

Useful snapshot fields include:

- overall state
- active session id
- session list
- primary notification, when present

## Mapping rules

Recommended visual intent:

- `offline`
  - LEDs mostly dark or neutral
- `idle`
  - calm, low-energy pattern
- `running`
  - active but non-alarming motion
- `running_bash`
  - same family as `running`, optionally with a more focused pulse
- `attention`
  - warm, visible, and persistent enough to be noticed
- `error`
  - high-priority red pattern

## Stability rules

The light renderer should prioritize stability over reactivity.

- repeated snapshots with the same effective state should not restart the animation
- small metadata changes should not cause visible flicker
- only meaningful state changes should trigger a new render plan

## Notification influence

The primary notification can refine the rendering without overriding the base state model.

Examples:

- a new `attention` notification can trigger a short-entry animation before settling into the steady attention pattern
- an `error` notification can elevate intensity or urgency

## Implementation expectations

A good state machine layer should:

- accept the current status snapshot plus optional primary notification
- derive one normalized render state
- generate a compact LED plan for the driver layer
- remain deterministic and easy to test

## Why this matters

This separation keeps three layers aligned:

1. the webserver state model
2. the uConsole GUI state presentation
3. the LED rendering behavior

When these three surfaces share one vocabulary, the system becomes easier to reason about and easier to extend to mobile clients later.
