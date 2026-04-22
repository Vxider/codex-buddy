# codex-buddy webserver architecture

> This document captures the architecture direction for the passive-monitoring `codex-buddy` server. The implementation has since converged into a single CLI with `serve` and `hook` subcommands, but the core design goals still apply.

## Goals

`codex-buddy` is not meant to replace Codex CLI. Its job is to provide a stable state bridge on the same machine that runs Codex CLI so local and remote clients can observe current session state and drive UI or hardware.

The current design goals are:

1. Use Codex hooks to wake or start a local webserver and forward status events.
2. Allow uConsole, local browser clients, and remote clients to connect over the network without Bluetooth.
3. Reserve space for future approve/reject actions without forcing V1 to depend on them.
4. Keep the protocol and state model compatible with future iPhone and Apple Watch clients.
5. Borrow the event-normalization mindset from `codex-on-desk` without copying its desktop-shell assumptions.

## Recommended runtime split

Keep a single `codex-buddy` binary, but split responsibility between two roles:

1. `codex-buddy hook`
   - Lightweight helper invoked by Codex hooks.
   - Ensures the server is running and forwards events quickly.
2. `codex-buddy serve`
   - Long-running daemon/webserver on the Codex host.
   - Owns status aggregation, transcript watching, HTTP/SSE APIs, and client broadcast.

Recommended V1 data sources:

- Primary: Codex hooks
- Secondary: transcript watching through the `transcript_path` provided by hooks

Likely V2 extensions:

- `codex app-server` as a richer upstream source
- more complete approval request/response flows
- multi-client control surfaces

Why this path works well:

- V1 is fast to implement and does not require modifying Codex itself
- it is naturally compatible with LED output, uConsole, browser dashboards, and watch/mobile clients
- higher-level client protocols can stay mostly stable even if the upstream source changes later

## Why not make `app-server` mandatory in V1

`codex app-server` is a stronger long-term integration point and already supports `stdio://` and `ws://IP:PORT` transports.

It is still a poor fit as the only V1 dependency because:

1. The immediate requirement is visibility and alerting, which hooks already cover.
2. V1 does not require a full approval/denial loop.
3. Hooks plus transcript watching fit better with the current SSH + tmux + Codex CLI workflow.

Recommended roadmap:

- V1: hooks first
- V2: add an `app-server` adapter

## Components

### Codex CLI side

- Codex CLI runs normally on the remote machine, typically inside SSH + tmux.
- Codex hooks fire at important lifecycle events.
- All hook commands point to `codex-buddy hook`.

### `codex-buddy hook`

Responsibilities:

- receive hook JSON from standard input
- check whether `codex-buddy serve` is alive locally
- start `serve` in the background when needed
- forward events to the daemon over loopback HTTP or a local socket
- exit quickly so Codex itself is not delayed

Design rules:

- `hook` does not own state
- `hook` does not do deep parsing
- `hook` only wakes and delivers

### `codex-buddy serve`

Responsibilities:

- track session state
- merge multiple input sources
- expose query and streaming APIs
- provide throttling and short-term history
- leave room for future control endpoints

Suggested internal modules:

- `HookAdapter`
- `TranscriptAdapter`
- `StateAggregator`
- `SessionStore`
- `APIServer`
- `Notifier`

### Clients

Clients only consume network APIs and do not need to know how hooks work.

Current client classes:

- uConsole GUI
- local browser status page
- remote browser client
- iPhone app
- Apple Watch app and complication/widget

## Event sources and normalization

### Hook events

The current Codex hooks surface the following useful events:

- `SessionStart`
- `UserPromptSubmit`
- `PreToolUse`
- `PostToolUse`
- `Stop`

Useful fields include:

- `session_id`
- `turn_id`
- `cwd`
- `model`
- `prompt`
- `transcript_path`
- `last_assistant_message`

Caveats:

- `PreToolUse` and `PostToolUse` are especially valuable for `Bash`
- hooks are not a complete real-time event bus

### Transcript watcher

Hooks alone are too coarse, so the daemon should also watch the JSONL transcript referenced by `transcript_path`.

This fills in:

- latest user input
- latest assistant output
- turn completion or failure
- error messages
- tool-output traces that appear after hook delivery

### Internal normalized events

A useful internal event vocabulary is:

- `session.started`
- `turn.submitted`
- `turn.stopped`
- `bash.started`
- `bash.finished`
- `transcript.updated`
- `state.derived`
- `server.error`

Future additions:

- `approval.requested`
- `approval.resolved`

## External state machine

V1 should keep the state model intentionally small so multiple input sources stay consistent.

Recommended public states:

- `offline`
  - daemon unreachable or no fresh events for a long time
- `idle`
  - session exists but no turn is currently running
- `running`
  - Codex is actively processing a turn
- `running_bash`
  - the active phase is clearly a Bash execution
- `attention`
  - a turn just ended, failed, or otherwise needs user review
- `error`
  - daemon/hook/transcript failure, or an execution failure surfaced to the user

Recommended derivation rules:

- `SessionStart` -> `idle`
- `UserPromptSubmit` -> `running`
- `PreToolUse(Bash)` -> `running_bash`
- `PostToolUse(Bash)` -> `running`
- `Stop(success)` -> `attention`
- `Stop(error)` -> `error`
- `attention` decays back to `idle` after a configurable hold window

Why keep `attention`:

- it maps well to uConsole LEDs and notification cards
- it also maps cleanly to future watch/iPhone summary UI
- it avoids conflating “finished and needs a glance” with “idle and nothing changed”

## API shape

The passive monitor mode should expose a small, stable API surface:

- `GET /health`
- `GET /status`
- `GET /v1/status`
- `GET /v1/notifications`
- `GET /v1/sessions`
- `GET /v1/sessions/:session_id`
- `POST /v1/sessions/:session_id/continue`
- `GET /v1/stream`

`GET /v1/status` should be the cold-start recovery API for all clients.

The current mobile/uConsole-oriented payload includes:

- overall aggregate state
- active session id and display title
- session list with display title, short id, summary, attention summary, and continue metadata

## Protocol constraints for uConsole, iPhone, and Apple Watch

The API should follow a few rules so every client can implement the same interaction model:

- a session must have a stable `session_id`
- UI-facing clients should receive a short or user-friendly title without having to derive it themselves
- attention sessions should include a readable summary
- continue actions must be expressed as explicit metadata, not inferred from state alone
- reconnecting clients must be able to rebuild UI from `GET /v1/status`

## Operational notes

- keep the ingest path loopback-only by default
- keep external status endpoints unauthenticated only for trusted local-network or Tailscale deployments
- prefer passive observation over control unless a feature explicitly requires action routing
- treat the browser status page as a debug view and `v1/status` as the formal client contract

## Roadmap

Near term:

1. keep refining hook + transcript aggregation
2. stabilize the state payload for uConsole and mobile clients
3. validate iPhone/watch clients against the same continue semantics already used by uConsole

Later:

1. evaluate an `app-server` adapter
2. add richer approval flows if needed
3. extend client-side surfaces without breaking the passive-monitoring contract
