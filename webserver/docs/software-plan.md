# codex-buddy software plan

> This is the implementation-focused companion to the architecture notes. It documents the current passive-monitoring direction rather than a speculative full control plane.

## Problem statement

The project needs a way to make Codex CLI state visible outside the terminal where it is running.

The system should support:

- uConsole as a native sidecar display and action surface
- browser dashboards for quick inspection
- future iPhone and Apple Watch clients over the network
- passive operation that does not create threads or inject turns unless the user explicitly requests a safe action such as `continue`

## Non-goals for the current version

The current implementation does not try to solve:

- a full approval workflow
- a replacement terminal UI for Codex CLI
- deep remote-control features that require `codex app-server`
- cloud hosting or public internet exposure by default

## Proposed implementation shape

Use one binary with several narrow commands:

- `serve`: runs the daemon/webserver
- `hook`: ingests Codex hook events and wakes the daemon when required
- `status`: prints the current aggregated state
- `uconsole`: runs the GUI sidecar client

## Data flow

1. Codex hook fires.
2. `codex-buddy hook` reads JSON from stdin.
3. The hook command forwards the event to the local daemon.
4. The daemon updates session state.
5. If a transcript path is available, the daemon watches the JSONL file for richer updates.
6. Clients pull the latest snapshot from `/v1/status` or subscribe to `/v1/stream`.
7. Action-capable clients may call `continue` when a session exposes that capability.

## Core internal modules

### Hook ingestion

Requirements:

- accept payloads quickly
- avoid expensive work in the hook process
- keep the contract tolerant of missing fields

### Transcript watcher

Requirements:

- watch one or more JSONL transcript files
- extract user prompts, assistant messages, tool traces, and errors
- enrich session state without replacing the hook event stream

### Session store

Requirements:

- keep one record per `session_id`
- derive aggregate state by priority and recent activity
- maintain a stable client-facing snapshot
- maintain notification/action metadata for attention and error states

### API server

Requirements:

- provide `/health` for operational checks
- provide `/v1/status` for cold-start recovery
- provide `/v1/stream` for low-latency UI updates
- provide session and continue endpoints for richer clients

### Continue executor

Requirements:

- only allow `continue` when the session is currently actionable
- keep the implementation narrow and explicit
- map the action to the existing tmux-backed workflow used by uConsole

## State derivation rules

Recommended aggregate priority:

1. `error`
2. `attention`
3. `running_bash`
4. `running`
5. `idle`
6. `offline`

Recommended state transitions:

- new session -> `idle`
- prompt submitted -> `running`
- Bash tool entered -> `running_bash`
- Bash tool finished -> `running`
- successful stop -> `attention`
- failed stop -> `error`
- stale `running` or `running_bash` -> `idle` after fallback timeout
- `attention` -> `idle` after the attention hold timeout

## Snapshot contract

The snapshot returned by `GET /v1/status` should be enough for every client to fully rebuild its UI.

At minimum it should include:

- server time
- overall state
- overall state detail
- active session id
- active session display title
- session count
- sessions array

Each session item should include:

- `session_id`
- `short_session_id`
- `display_title`
- `state`
- `state_detail`
- `updated_at`
- `summary`
- `needs_attention`
- `attention_summary`
- `can_continue`
- `continue_action`

## Notification model

The notification layer is a secondary view over the session store.

It should:

- surface one primary attention/error item per actionable session
- expose `ack` and `continue` semantics
- expire notifications automatically when the backing session state changes
- avoid diverging from the canonical session snapshot

## Continue flow

The continue flow should be the same across uConsole, iPhone, and Apple Watch.

1. Client reads `v1/status`.
2. User sees an `attention` session with a readable summary.
3. Client checks `can_continue` and `continue_action`.
4. Client sends `POST /v1/sessions/:session_id/continue` with the action token.
5. Server validates that the session is still actionable.
6. Server routes the action through the tmux-backed continue executor.
7. Client refreshes from the updated snapshot.

## Operational defaults

Recommended defaults for the passive setup:

- ingest endpoint restricted to loopback
- no token auth on local-network status endpoints
- Tailscale or another trusted internal network for remote access
- no public internet exposure by default

## Client expectations

### uConsole

- can use both `/v1/status` and `/v1/notifications`
- should show attention/error as the primary card
- should continue to use the same continue semantics as mobile clients

### Browser debug page

- should remain intentionally lightweight
- may consume the richer status payload but does not need to expose every action

### iPhone and Apple Watch

- should read the same `v1/status` snapshot as uConsole
- should display the same attention summaries
- should trigger the same continue semantics
- watch foreground interactions should be proxied through the phone app in the no-developer-account setup

## Testing priorities

1. Session store transitions and aggregate priority
2. Transcript enrichment behavior
3. Notification/action visibility rules
4. `/v1/status` payload stability
5. Continue action validation and execution behavior
6. SSE reconnect behavior for clients

## Near-term milestones

1. Keep the current hook + transcript implementation stable.
2. Lock down the mobile/uConsole-friendly status payload.
3. Validate the continue action across browser, uConsole, and watch/phone clients.
4. Keep room for future `app-server` integration without changing the external passive-monitoring contract.
