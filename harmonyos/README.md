# Agent Buddy for HarmonyOS NEXT

This directory contains the ArkTS/ArkUI client for `agent-buddy`, modelled on
the existing iOS client and using the same public HTTP contract.

## Features

- Reads `GET /v1/status` from one configured agent-buddy server.
- Polls every three seconds while the app is visible.
- Shows Codex sessions and continues actionable sessions through
  `POST /v1/sessions/{session_id}/continue`.
- Publishes the aggregate Codex state to HarmonyOS NEXT Live View Kit.
- Uses the shared light semantics:
  - red: Codex abnormal interruption, authentication/quota/network/service
    failure, or a lost connection;
  - yellow: approval, permission, attention, or follow-up;
  - purple: an active or completed goal when the server exposes goal metadata;
  - green: normal running or idle-ready state.

The Live View uses the supported `TIMER` progress template. The light color is
rendered in the primary content, capsule background, and progress indicator so
the compact capsule and expanded card carry the same signal.

## Open in DevEco Studio

1. Open this `harmonyos/` directory as a project in DevEco Studio.
2. Install a HarmonyOS NEXT SDK at or above API 11. Live View Kit starts at
   API 11; the project profile targets API 12 by default.
3. Configure signing for the `com.vxider.agentbuddy.next` bundle.
4. In AppGallery Connect, apply for Live View service entitlement and
   regenerate the application Profile after it is approved. Without that
   entitlement, the normal app still works and Live View calls report the
   platform error in the log.
5. Build and run on a supported HarmonyOS phone or tablet. Live View Kit has
   no effect on unsupported device types.
6. Enter a reachable server URL, for example `http://192.168.1.20:8787`, then
   tap `启用实况窗`.

The client accepts both `http://` and `https://` URLs. The sample network
profile permits clear-text HTTP because agent-buddy is commonly used on a
trusted local network; use HTTPS when the server is reachable outside that
network.

## Live View behavior

The page starts a Live View after the first successful status refresh when the
user enables it. Subsequent status changes update the same Live View with a
monotonic sequence number. A click on the Live View reopens `EntryAbility`.
The app attempts to recover an existing Live View after a process restart with
`getActiveLiveView` before creating a new one.

The local polling loop is intentionally conservative because Live View updates
are rate limited by the platform. For updates while the app process is fully
stopped, integrate the server's status events with Push Kit and send the same
Live View payload remotely; the state and color mapping in `common/CodexModels.ets`
is already independent of the transport.

## Server contract

The client follows `api/contract/README.md` and ignores fields it does not
need. Unknown Codex states are treated as `offline` and transport failures are
rendered as red abnormal interruptions, matching the other agent-buddy clients.
