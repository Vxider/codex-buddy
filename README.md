# codex-buddy

`codex-buddy` is a passive companion for Codex CLI. The server runs on the same machine as `codex`, aggregates session state from hooks and transcript watchers, and exposes that state over local-network HTTP/SSE without taking over your main terminal or tmux workflow.

## Design philosophy

`codex-buddy` follows the same practical philosophy that makes `tmux` useful:

- keep the primary Codex session in its normal SSH/terminal/tmux environment
- add visibility and lightweight sidecar controls without replacing the main UI
- let clients attach, observe, disconnect, and reconnect without disturbing the running session
- prefer passive state bridging over deep orchestration or hidden automation
- keep full debugging and detailed interaction in the main terminal, not in the companion surface

The repository currently contains three tracks:

- Server: passively monitors locally running Codex CLI sessions without creating threads or sending turns
- uConsole client: native GUI for aggregate state, attention/error reminders, and one-shot `continue + Enter` actions during `attention`
- macOS menu bar client: native AppKit status item with SSE-backed LED state and on-demand session menu
- iPhone / Apple Watch companion: SwiftUI sources and an XcodeGen project spec for a mobile client

## Capabilities

- `codex-buddy serve`
  - Starts the local daemon/webserver and exposes `/health`, `/status`, `/v1/status`, `/v1/notifications`, `/v1/sessions`, and `/v1/stream`
- `codex-buddy hook`
  - Reads Codex hook events from `stdin` and forwards them to the local daemon
- `codex-buddy status`
  - Prints the current aggregate status, optionally as JSON
- `codex-buddy start`
  - Starts the local daemon through `systemd --user` when installed, otherwise starts `serve` in the background
- `codex-buddy restart`
  - Restarts the local daemon through `systemd --user` when installed, otherwise performs a local stop/start cycle
- `codex-buddy stop`
  - Asks the local daemon to shut down cleanly, with `systemd --user` fallback when available
- `codex-buddy-uconsole`
  - Starts the native uConsole companion GUI
  - Connects to a local or remote `codex-buddy` server over HTTP/SSE and supports attention/error cards, acknowledge, continue, and voice follow-up input
- `codex-buddy uconsole`
  - Compatibility entrypoint for the same GUI app
- `codex-buddy esp32`
  - Bridges `/v1/status` and `/v1/stream` to an ESP32-C3 sidecar over USB CDC / UART
  - The firmware drives a motor on GPIO 10 and 21 WS2812 LEDs on GPIO 3

macOS menu bar app:

```bash
cd macos
xcodegen generate
open CodexBuddyMac.xcodeproj
```

The menu bar LED is backed by `/v1/stream`; the session list is loaded from `/v1/status` when the menu opens.

## Build

Recommended build scripts by target:

Webserver:

```bash
./webserver/scripts/build-install.sh
```

uConsole GUI:

```bash
./uconsole/scripts/build-install.sh
```

Notes:

- `webserver/scripts/build-install.sh`
  - Builds the server-oriented binary without GUI tags
  - Installs to `~/.local/bin/codex-buddy` by default
  - Restarts `codex-buddy.service` automatically when the service is already installed
- `uconsole/scripts/build-install.sh`
  - Builds the GUI-enabled uConsole variant with the `uconsole_gui` tag
  - Installs to `~/.local/bin/codex-buddy-uconsole` by default

Manual builds are also supported.

Server build without desktop GUI dependencies:

```bash
mkdir -p ~/.local/bin
go build -o ~/.local/bin/codex-buddy ./cmd/codex-buddy
```

Native uConsole GUI build:

```bash
go build -tags uconsole_gui -o ~/.local/bin/codex-buddy-uconsole ./cmd/codex-buddy-uconsole
```

Notes:

- `uconsole_gui` isolates Fyne desktop dependencies so server-only builds and tests stay lightweight

## Setup

Write the initial config, Codex `config.toml` hook definitions, and `systemd --user` service:

```bash
codex-buddy setup
```

`setup` writes a managed hook block to `~/.codex/config.toml`. Recent Codex versions expose hooks as a stable feature, so no separate `codex_hooks` feature flag or `~/.codex/hooks.json` file is required.

Start or restart the service:

```bash
codex-buddy start
codex-buddy restart
systemctl --user restart codex-buddy.service
```

Verify the installation:

```bash
codex-buddy doctor
codex-buddy status
```

Stop the local daemon cleanly:

```bash
codex-buddy stop
```

## Running uConsole

Connect to the local default server at `http://127.0.0.1:8787`:

```bash
codex-buddy-uconsole
```

Connect to a remote `codex-buddy` server:

```bash
codex-buddy-uconsole --server-url http://<codex-host>:8787
```

`codex-buddy uconsole` remains available as a compatibility command. If the current binary does not include the GUI, the command will tell you to rebuild with `-tags uconsole_gui`.

## Running ESP32 Sidecar

Build and flash the firmware in [firmware/codex-buddy-esp32c3](firmware/codex-buddy-esp32c3):

```bash
cd firmware/codex-buddy-esp32c3
pio run -t upload
```

Bridge the local Codex Buddy status stream to the ESP32-C3 serial device:

```bash
codex-buddy esp32 --uart /dev/ttyACM0
```

Set the motor independently:

```bash
codex-buddy esp32 --uart /dev/ttyACM0 --motor 160
```

The host sends compact newline-delimited frames such as `CB1 state=run led=green detail=running sessions=1 summary=...`. The current transport is UART; the `internal/esp32sidecar` package keeps the transport boundary explicit so BLE advertising can be added without changing the firmware state protocol.

## Configuration

See the example configuration at [webserver/examples/config.example.json](webserver/examples/config.example.json).

The current default mode is passive local-network monitoring:

- `/status`, `/v1/status`, and `/v1/stream` do not require token authentication
- the hook ingest endpoint is still loopback-only by default

The `uconsole` config block currently includes:

- `server_url`
- `http_timeout_ms`
- `reconnect_delay_ms`
- `poll_fallback_ms`
- `continue_hold_ms`
- `window.width`
- `window.height`
- `window.fullscreen`

## Documentation

- [ios/README.md](ios/README.md)
- [webserver/docs/architecture.md](webserver/docs/architecture.md)
- [webserver/docs/software-plan.md](webserver/docs/software-plan.md)
- [uconsole/docs/hardware.md](uconsole/docs/hardware.md)
