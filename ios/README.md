# Agent Buddy iPhone + Apple Watch

This directory contains the first public-source version of the iPhone and Apple Watch companion for `agent-buddy`.

## Layout

- `AppCore/`: shared models, JSON decoding, snapshot storage, deep links, and the HTTP client for `/v1/status` and `/v1/sessions/{id}/continue`
- `PhoneApp/`: the iPhone app that talks directly to the `agent-buddy` webserver
- `WatchApp/`: the watchOS foreground app that refreshes through `WatchConnectivity` and proxies `continue` through the iPhone app
- `WatchWidget/`: the accessory circular watch widget that renders the cached aggregate state
- `project.yml`: XcodeGen specification for the iPhone app, watch app, and widget extension

## What works today

- The iPhone app loads the current Codex status snapshot from `v1/status`
- Attention sessions display a summary and expose a `continue` button
- The watch app opens a compact session list and proxies actions through the iPhone companion
- The watch widget shows the aggregated state as a face emoji and deep-links into the app

## Generate the Xcode project

1. Install XcodeGen on macOS: `brew install xcodegen`
2. Copy `project.local.env.example` to `project.local.env` and set your Apple Developer team there
3. Run `./generate-project.sh`
4. Open `AgentBuddy.xcodeproj` in Xcode
5. Confirm that the shared App Group is available for your signing setup; if not, temporarily fall back to `.standard` storage in `CodexSnapshotStore`

`project.local.env` is intentionally untracked and is loaded before `xcodegen` runs, so regenerating the project does not wipe your local signing configuration.

## Base URL examples

- `http://<codex-host>:8787`
- `https://<codex-host>`

## Runtime model

- The iPhone app is the only client that talks directly to the `agent-buddy` webserver
- The watch app talks to the iPhone app over `WatchConnectivity`
- The widget only shows the last cached aggregate state; interactive work happens in the watch foreground app
- The shared deep-link scheme is `agentbuddy://sessions`

## Current limitations

- This code was authored outside macOS, so the generated Xcode project has not been opened or signed in this environment
- App icons and asset catalogs are still placeholders
- The watch widget is intentionally low-frequency and should be treated as summary UI, not real-time monitoring
