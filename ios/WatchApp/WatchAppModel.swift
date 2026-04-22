import Foundation
import WatchConnectivity
import WidgetKit

@MainActor
final class WatchAppModel: NSObject, ObservableObject {
    @Published var snapshot: CodexStatusSnapshot = CodexSnapshotStore.load() ?? .offline
    @Published var isRefreshing = false
    @Published var errorMessage: String?

    override init() {
        super.init()
        guard WCSession.isSupported() else { return }
        WCSession.default.delegate = self
        WCSession.default.activate()
    }

    func refresh() async {
        await performRequest(kind: "status", extra: [:])
    }

    func continueSession(_ session: CodexSessionSummary) async {
        guard let action = session.continueAction else {
            errorMessage = CodexAPIError.missingContinueAction.localizedDescription
            return
        }
        await performRequest(kind: "continue", extra: [
            "session_id": session.sessionID,
            "action_token": action.actionToken
        ])
    }

    private func performRequest(kind: String, extra: [String: Any]) async {
        isRefreshing = true
        defer { isRefreshing = false }
        do {
            let snapshot = try await sendMessage(kind: kind, extra: extra)
            apply(snapshot: snapshot)
            errorMessage = nil
        } catch {
            errorMessage = error.localizedDescription
        }
    }

    private func apply(snapshot: CodexStatusSnapshot) {
        self.snapshot = snapshot
        CodexSnapshotStore.save(snapshot)
        WidgetCenter.shared.reloadAllTimelines()
    }

    private func sendMessage(kind: String, extra: [String: Any]) async throws -> CodexStatusSnapshot {
        guard WCSession.isSupported() else {
            throw CodexAPIError.bridgeUnavailable("WatchConnectivity is not available.")
        }
        let session = WCSession.default
        guard session.isReachable else {
            throw CodexAPIError.bridgeUnavailable("Open the iPhone companion app to refresh.")
        }

        var message = extra
        message["kind"] = kind

        let reply: [String: Any] = try await withCheckedThrowingContinuation { continuation in
            session.sendMessage(message) { response in
                continuation.resume(returning: response)
            } errorHandler: { error in
                continuation.resume(throwing: error)
            }
        }

        guard (reply["ok"] as? Bool) == true else {
            let message = (reply["error"] as? String) ?? "Phone bridge failed."
            throw CodexAPIError.bridgeUnavailable(message)
        }
        guard let payload = reply["snapshot"] as? String,
              let data = payload.data(using: .utf8) else {
            throw CodexAPIError.bridgeUnavailable("Missing snapshot payload.")
        }
        return try makeDecoder().decode(CodexStatusSnapshot.self, from: data)
    }
}

extension WatchAppModel: WCSessionDelegate {
    nonisolated func session(_ session: WCSession, activationDidCompleteWith activationState: WCSessionActivationState, error: Error?) {}

    nonisolated func sessionReachabilityDidChange(_ session: WCSession) {}

    nonisolated func session(_ session: WCSession, didReceiveApplicationContext applicationContext: [String : Any]) {
        guard let payload = applicationContext["snapshot"] as? String,
              let data = payload.data(using: .utf8),
              let snapshot = try? makeDecoder().decode(CodexStatusSnapshot.self, from: data) else {
            return
        }
        Task { @MainActor in
            self.apply(snapshot: snapshot)
        }
    }

    nonisolated func session(_ session: WCSession, didReceiveUserInfo userInfo: [String : Any]) {
        applyPayload(userInfo)
    }

    nonisolated func session(_ session: WCSession, didReceiveComplicationUserInfo complicationUserInfo: [String : Any] = [:]) {
        applyPayload(complicationUserInfo)
    }

    private nonisolated func applyPayload(_ payload: [String: Any]) {
        guard let snapshotPayload = payload["snapshot"] as? String,
              let data = snapshotPayload.data(using: .utf8),
              let snapshot = try? makeDecoder().decode(CodexStatusSnapshot.self, from: data) else {
            return
        }
        Task { @MainActor in
            self.apply(snapshot: snapshot)
        }
    }
}
