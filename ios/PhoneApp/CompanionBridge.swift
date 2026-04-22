import Foundation
import WatchConnectivity

@MainActor
final class CompanionBridge: NSObject {
    static let shared = CompanionBridge()

    private var api: CodexBuddyAPI?
    private var snapshotProvider: (() -> CodexStatusSnapshot?)?

    func configure(api: CodexBuddyAPI, snapshotProvider: @escaping () -> CodexStatusSnapshot?) {
        self.api = api
        self.snapshotProvider = snapshotProvider
        guard WCSession.isSupported() else { return }
        let session = WCSession.default
        session.delegate = self
        session.activate()
    }

    func push(snapshot: CodexStatusSnapshot) {
        CodexSnapshotStore.save(snapshot)
        guard WCSession.isSupported() else { return }
        guard let payload = encode(snapshot: snapshot) else { return }
        let session = WCSession.default
        do {
            try session.updateApplicationContext(["snapshot": payload])
        } catch {
            // Ignore transient connectivity errors.
        }
        if session.isPaired, session.isWatchAppInstalled {
            session.transferCurrentComplicationUserInfo(["snapshot": payload])
        }
    }

    private func encode(snapshot: CodexStatusSnapshot) -> String? {
        guard let data = try? makeEncoder().encode(snapshot) else { return nil }
        return String(data: data, encoding: .utf8)
    }

    private func reply(snapshot: CodexStatusSnapshot) -> [String: Any] {
        [
            "ok": true,
            "snapshot": encode(snapshot: snapshot) ?? ""
        ]
    }

    private func reply(error: String) -> [String: Any] {
        [
            "ok": false,
            "error": error
        ]
    }
}

extension CompanionBridge: WCSessionDelegate {
    nonisolated func session(_ session: WCSession, activationDidCompleteWith activationState: WCSessionActivationState, error: Error?) {}

    nonisolated func sessionDidBecomeInactive(_ session: WCSession) {}

    nonisolated func sessionDidDeactivate(_ session: WCSession) {
        session.activate()
    }

    nonisolated func session(_ session: WCSession, didReceiveMessage message: [String: Any], replyHandler: @escaping ([String: Any]) -> Void) {
        Task { @MainActor [weak self] in
            guard let self else {
                replyHandler(["ok": false, "error": "bridge unavailable"])
                return
            }
            guard let kind = message["kind"] as? String else {
                replyHandler(self.reply(error: "missing request kind"))
                return
            }
            guard let api = self.api else {
                replyHandler(self.reply(error: "phone bridge is not configured"))
                return
            }

            do {
                switch kind {
                case "status":
                    let snapshot = try await api.loadStatus()
                    self.push(snapshot: snapshot)
                    replyHandler(self.reply(snapshot: snapshot))
                case "continue":
                    let sessionID = message["session_id"] as? String ?? ""
                    let token = message["action_token"] as? String ?? ""
                    var snapshot = self.snapshotProvider?() ?? CodexSnapshotStore.load() ?? CodexStatusSnapshot.offline
                    if snapshot.sessions.isEmpty {
                        snapshot = try await api.loadStatus()
                    }
                    guard let item = snapshot.sessions.first(where: { $0.sessionID == sessionID && $0.continueAction?.actionToken == token }) else {
                        throw CodexAPIError.missingContinueAction
                    }
                    let response = try await api.continueSession(item)
                    let latest = response.status ?? (try await api.loadStatus())
                    self.push(snapshot: latest)
                    replyHandler(self.reply(snapshot: latest))
                default:
                    replyHandler(self.reply(error: "unsupported request kind"))
                }
            } catch {
                replyHandler(self.reply(error: error.localizedDescription))
            }
        }
    }
}
