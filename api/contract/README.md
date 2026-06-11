# Codex Buddy API Contract

This directory contains stable JSON fixtures for client implementations.

The fixtures are intentionally small and hand-readable. Swift, macOS, uConsole,
and other clients should use them as decoding fixtures for the public HTTP API.
Server tests also decode these files so field names and payload shapes do not
drift silently.

## Endpoints

### `GET /v1/status`

Returns a complete snapshot that clients can use to rebuild their UI after
cold start or reconnect. See:

- `fixtures/status.idle.json`
- `fixtures/status.running.json`
- `fixtures/status.open.json`

### `POST /v1/sessions/{session_id}/continue`

Sends a continue action or custom text to an actionable session. See:

- `fixtures/continue.ok.json`

## Compatibility Notes

- Clients should ignore unknown fields.
- Optional fields may be absent when they do not apply.
- Timestamps are RFC 3339 strings.
- `running` and `running_bash` may be normalized to `run` by the public API.
