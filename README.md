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
- uConsole client: native GUI with optional WS2812 LED output for aggregate state, attention/error reminders, and one-shot `continue + Enter` actions during `attention`
- iPhone / Apple Watch companion: SwiftUI sources and an XcodeGen project spec for a Tailscale-only mobile client

## Capabilities

- `codex-buddy serve`
  - Starts the local daemon/webserver and exposes `/health`, `/status`, `/v1/status`, `/v1/notifications`, `/v1/sessions`, and `/v1/stream`
- `codex-buddy hook`
  - Reads Codex hook events from `stdin` and forwards them to the local daemon
- `codex-buddy status`
  - Prints the current aggregate status, optionally as JSON
- `codex-buddy-uconsole`
  - Starts the native uConsole companion GUI
  - Connects to a local or remote `codex-buddy` server over HTTP/SSE and supports attention/error cards, acknowledge, continue, and LED state rendering
- `codex-buddy uconsole`
  - Compatibility entrypoint for the same GUI app

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

uConsole GUI with WS2812 hardware support:

```bash
./uconsole/scripts/build-install.sh --ws281x
```

Notes:

- `webserver/scripts/build-install.sh`
  - Builds the server-oriented binary without GUI tags
  - Installs to `~/.local/bin/codex-buddy` by default
  - Restarts `codex-buddy.service` automatically when the service is already installed
- `uconsole/scripts/build-install.sh`
  - Builds the GUI-enabled uConsole variant with the `uconsole_gui` tag
  - Accepts `--ws281x` for real hardware LED output
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

Native uConsole GUI build with Raspberry Pi WS2812 output:

```bash
go build -tags 'uconsole_gui ws281x' -o ~/.local/bin/codex-buddy-uconsole ./cmd/codex-buddy-uconsole
```

Notes:

- `uconsole_gui` isolates Fyne desktop dependencies so server-only builds and tests stay lightweight
- `ws281x` enables the `rpi-ws281x-go` hardware driver; without it the code falls back to a no-op LED driver
- On Raspberry Pi / uConsole hardware, `libws2811` and related system dependencies must be installed before enabling `ws281x`

## Setup

Write the initial config, hook definitions, and `systemd --user` service:

```bash
codex-buddy setup
```

Start or restart the service:

```bash
systemctl --user restart codex-buddy.service
```

Verify the installation:

```bash
codex-buddy doctor
codex-buddy status
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

Disable LEDs for local debugging:

```bash
codex-buddy-uconsole --no-led
```

`codex-buddy uconsole` remains available as a compatibility command. If the current binary does not include the GUI, the command will tell you to rebuild with `-tags uconsole_gui`.

## Configuration

See the example configuration at [webserver/examples/config.example.json](/home/vxider/WorkSpace/codex-buddy/webserver/examples/config.example.json).

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
- `led.enabled`
- `led.pixels`
- `led.brightness`
- `led.gpio_pin`
- `led.dma_num`
- `led.frequency`

The recommended default LED setup for a CM5-based uConsole 4G blank board is `GPIO45 / pin 52 + 8 x WS2812`.

## Documentation

- [ios/README.md](/home/vxider/WorkSpace/codex-buddy/ios/README.md)
- [webserver/docs/architecture.md](/home/vxider/WorkSpace/codex-buddy/webserver/docs/architecture.md)
- [webserver/docs/software-plan.md](/home/vxider/WorkSpace/codex-buddy/webserver/docs/software-plan.md)
- [uconsole/docs/hardware.md](/home/vxider/WorkSpace/codex-buddy/uconsole/docs/hardware.md)
- [uconsole/docs/light-state-machine-interface.md](/home/vxider/WorkSpace/codex-buddy/uconsole/docs/light-state-machine-interface.md)
- [uconsole/docs/archive/zh-CN/README.md](/home/vxider/WorkSpace/codex-buddy/uconsole/docs/archive/zh-CN/README.md)
