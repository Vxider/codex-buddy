import Foundation

@MainActor
final class PhoneAppModel: ObservableObject {
    @Published var snapshot: CodexStatusSnapshot
    @Published var errorMessage: String?
    @Published var isLoading = false
    @Published var baseURLText: String

    private let api: CodexBuddyAPI
    private let bridge: CompanionBridge

    init(api: CodexBuddyAPI = .shared, bridge: CompanionBridge = .shared) {
        self.api = api
        self.bridge = bridge
        self.snapshot = CodexSnapshotStore.load() ?? .offline
        self.baseURLText = api.currentBaseURLString()
        bridge.configure(api: api) { [weak self] in
            self?.snapshot
        }
    }

    func refresh() async {
        isLoading = true
        defer { isLoading = false }
        do {
            let latest = try await api.loadStatus()
            snapshot = latest
            CodexSnapshotStore.save(latest)
            bridge.push(snapshot: latest)
            errorMessage = nil
        } catch {
            errorMessage = error.localizedDescription
        }
    }

    func continueSession(_ session: CodexSessionSummary) async {
        isLoading = true
        defer { isLoading = false }
        do {
            let response = try await api.continueSession(session)
            let latest = response.status ?? try await api.loadStatus()
            snapshot = latest
            CodexSnapshotStore.save(latest)
            bridge.push(snapshot: latest)
            errorMessage = nil
        } catch {
            errorMessage = error.localizedDescription
        }
    }

    func saveServerURL() {
        api.updateBaseURL(baseURLText)
    }
}
