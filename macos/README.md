# Codex Buddy macOS

Native AppKit menu bar app for `codex-buddy`.

```bash
cd macos
xcodegen generate
open CodexBuddyMac.xcodeproj
```

The menu bar LED uses `/v1/stream`. The session menu refreshes `/v1/status` once when opened.

To build from the command line:

```bash
./scripts/build.sh
```
