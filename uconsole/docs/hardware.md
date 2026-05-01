# uConsole hardware notes

This document summarizes the hardware assumptions behind the uConsole companion setup used by `codex-buddy`.

## Purpose

The uConsole build is meant to be a dedicated sidecar device for Codex CLI status.

Typical responsibilities:

- show aggregate status at a glance
- surface attention and error states clearly
- provide a simple path to trigger `continue`
- optionally feed ambient light data into host-side screen auto-brightness control

## Reference hardware stack

The project has been developed around a CM5-based uConsole-style setup with:

- Linux-based handheld host
- local `codex-buddy` or remote access over an internal network
- optional USB-connected ESP32 for sensors or simple sidecar I/O
- optional external keyboard shortcuts for acknowledge/continue flows

## Design constraints

The hardware integration should stay pragmatic:

- the terminal remains the primary Codex interface
- uConsole is a side display, not a terminal replacement
- auto-brightness should stay smooth and predictable
- the continue action must remain explicit and safe

## Automatic brightness option

For tight internal space, prefer a bare ambient light sensor instead of a breakout module.

Recommended minimal stack:

- `TEPT4400` ambient light sensor
- `1 x` resistor for a simple voltage divider
- `ESP32` reading the divider with ADC
- `USB CDC` from the ESP32 to the Linux host

This layout is intentionally simpler than adding a digital lux module such as `OPT3001` or
`VEML7700`. Those parts work well, but they usually mean an extra PCB or breakout board.
`TEPT4400` is easier to place on a small opening or bezel edge and can be wired directly.

### Why this approach

- very small sensor body, easy to hide in a constrained enclosure
- no separate sensor module required
- one ADC input is enough
- Linux keeps control of the real panel backlight

Avoid direct LCD backlight PWM interception unless there is a strong reason to redesign the
display wiring. The preferred model is:

1. `TEPT4400` measures ambient light.
2. `ESP32` converts the analog level to a smoothed reading.
3. `ESP32` sends periodic readings over USB serial.
4. A small host-side daemon maps those readings to panel brightness.
5. Linux writes the target value to `/sys/class/backlight/backlight@0/brightness`.

On the current uConsole host, the backlight interface is exposed as:

- `/sys/class/backlight/backlight@0/max_brightness`
- `/sys/class/backlight/backlight@0/brightness`

This keeps the design reversible and avoids invasive panel-side modifications.

### Minimal wiring

Use `TEPT4400` as the light-dependent leg of a simple divider and feed the midpoint into an
`ESP32` ADC pin.

```text
3.3V ---- TEPT4400 collector
             |
             |
             +---- emitter ---- [sense node] ----> ESP32 ADC input
                                      |
                                      +---- 10k resistor ----> GND
```

Equivalent connection list:

- `TEPT4400 collector -> 3.3V`
- `TEPT4400 emitter -> ESP32 ADC pin`
- `10k resistor -> between ESP32 ADC pin and GND`
- `ESP32 GND -> sensor ground`

Behavior summary:

- brighter ambient light -> higher sensor current -> higher ADC reading
- darker ambient light -> lower sensor current -> lower ADC reading

The exact resistor value is not critical. Start with `10k`, then tune to `4.7k` or `22k` if
the ADC range is too compressed at your chosen sensor placement.

### Placement notes

- place the sensor where it sees room light, not reflected glare from the display
- avoid aiming it directly at the display surface
- if needed, recess the sensor slightly to reduce false spikes from point light sources
- keep the sensor on short wires if the ESP32 ADC input is noisy

### Host-side behavior

The host-side daemon should not mirror the ADC value directly to brightness. Add:

- a moving average or low-pass filter
- hysteresis so brightness does not flap near thresholds
- update rate limiting, for example `2-4 Hz`
- a floor brightness for dark rooms
- a manual override path

This produces a much better handheld experience than raw instantaneous lux tracking.

## Continue interaction

The uConsole flow is intentionally simple:

- attention creates a visible primary card
- the user can acknowledge it
- the user can trigger a single `continue` action
- long-press confirmation is preferred over accidental single-tap activation

## Deployment notes

- build the GUI variant with `-tags uconsole_gui`
- keep server and GUI builds separable so headless development remains easy

## Practical recommendation

Treat the uConsole hardware as a focused companion surface:

- status display first
- low-friction continue action second
- full debugging still belongs in the main SSH/tmux terminal session
