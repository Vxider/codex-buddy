# tmux Window Status Dots

`codex-buddy tmux-window-dot` renders a colored dot in tmux window tabs to indicate session state.

## Colors and Semantics

| Color   | Hex       | Meaning                          | Trigger Condition                              |
|---------|-----------|----------------------------------|------------------------------------------------|
| Red     | `#ff0000` | Needs approval / Error           | Session state is `error` or `attention`        |
| Yellow  | `#ffff00` | Stopped (unread)                 | Session was running, now idle; user hasn't focused the window |
| Purple  | `#af00ff` | Goal running                     | Session is running with an active goal         |
| Green   | `#00ff00` | Running normal task              | Session is running without an active goal      |

## Priority

When a window has multiple sessions, the dot shows the highest-priority state:

```
red > yellow > purple > green
```

## State Transitions

- **Red**: Triggered by `error` or `attention` state. Does not trigger yellow when cleared.
- **Yellow**: Triggered when a running session transitions to `idle`. Cleared when:
  - User focuses the window
  - Session starts running again
- **Purple**: Session is running and has an active goal (from `/v1/status` or `goals_1.sqlite`).
- **Green**: Session is running but has no active goal.

## Architecture

- `buddy server` is a pure status source via `/v1/status`.
- `codex-buddy tmux-window-dot` reads `/v1/status` and maintains local state in:
  ```
  ~/.cache/codex-buddy/tmux-window-dots.json
  ```
- Yellow dot state is local to tmux helper, not stored in buddy server.
- File locking prevents concurrent write corruption when multiple windows refresh simultaneously.

## tmux Integration

In `.tmux.conf.local`:

```tmux
set -g status-interval 1

# Non-current window
#I #(/path/to/codex-buddy tmux-window-dot "#{window_id}")#W

# Current window (with --active-window for yellow dot clearing)
#I #(/path/to/codex-buddy tmux-window-dot --active-window "#{window_id}" "#{window_id}")#W
```

The dot uses pseudo-blink: renders the dot on even seconds, a single space on odd seconds. Requires `status-interval 1`.
