# Agent Buddy ESP32-C3 Firmware

Control firmware for an ESP32-C3 sidecar:

- motor on GPIO 10
- WS2812 data on GPIO 3
- 21 LEDs total, grouped as 3 groups of 7
- status input over USB CDC / UART

The firmware is intentionally small and self-contained. Before the first status
frame arrives it runs a local demo cycle so the motor and LEDs can be tested.

## Build

```sh
cd firmware/agent-buddy-esp32c3
pio run
```

Flash and monitor:

```sh
pio run -t upload
pio device monitor -b 115200
```

## Host Bridge

Run `agent-buddy` on the host and bridge its status stream to the ESP32-C3 serial
device:

```sh
agent-buddy esp32 --uart /dev/ttyACM0
```

Publish one frame and exit:

```sh
agent-buddy esp32 --uart /dev/ttyACM0 --once
```

Set the motor separately from Codex status:

```sh
agent-buddy esp32 --uart /dev/ttyACM0 --motor 160
agent-buddy esp32 --uart /dev/ttyACM0 --motor 0
```

The default baud rate is `115200`.

## Wire Protocol

The host sends one newline-terminated ASCII frame:

```text
CB1 state=run led=green detail=running sessions=1 summary=Refactor
```

The firmware primarily follows the `led` field:

- `red`: first group, approval/error
- `yellow`: second group, attention/open
- `green`: third group, idle/working
- `purple`: all groups with a pipeline effect, goal/purple signal
- `off`: fallback to state-derived display

All flashing effects include a light breathing modulation. If no frame is
received for 15 seconds after the first frame, the firmware shows the offline
state.

## Serial Test Commands

Open the serial monitor and send one command per line:

- `idle`
- `run`
- `open`
- `error`
- `offline`
- `led red`
- `led yellow`
- `led green`
- `led purple`
- `CB1 state=run led=green detail=running sessions=1 summary=test`
- `motor 0` through `motor 255`
- `auto` to stop the motor
- `help`

## State Mapping

- group 1: red
- group 2: yellow
- group 3: green
- `approval` / permission or error: red group breathes
- `attention` / open follow-up: yellow group breathes
- `working` and idle-ready fallback: green group breathes
- `goal` / purple signal: all 21 LEDs use a purple breathing pipeline

Motor control is independent of Codex state and only changes when a `motor`
command is sent.

## Hardware Notes

GPIO 10 drives only a control signal. Use a MOSFET or motor driver, a flyback
diode for brushed motors, and a shared ground. Do not power the motor directly
from the ESP32-C3 GPIO.

WS2812 LEDs should use a suitable external 5 V supply when needed, with shared
ground. A 330 ohm data resistor and a bulk capacitor across LED power are
recommended.
