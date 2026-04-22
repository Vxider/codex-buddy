import Foundation

@MainActor
final class PhoneAppModel: ObservableObject {
    @Published var snapshot: CodexStatusSnapshot
    @Published var errorMessage: String?
    @Published var isLoading = false
    @Published private(set) var servers: [CodexBuddyServer]
    @Published private(set) var selectedServerID: UUID?
    private var isSilentRefreshInFlight = false

    private let api: CodexBuddyAPI
    private let bridge: CompanionBridge

    init(api: CodexBuddyAPI = .shared, bridge: CompanionBridge? = nil) {
        let bridge = bridge ?? .shared
        self.api = api
        self.bridge = bridge
        self.snapshot = CodexSnapshotStore.load() ?? .offline
        self.servers = api.loadServers()
        self.selectedServerID = api.currentServer()?.id
        bridge.configure(api: api) { [weak self] in
            self?.snapshot
        }
    }

    func refresh() async {
        await refresh(showLoading: true, reportErrors: true)
    }

    func refreshSilently() async {
        guard !isLoading, !isSilentRefreshInFlight else { return }
        isSilentRefreshInFlight = true
        defer { isSilentRefreshInFlight = false }
        await refresh(showLoading: false, reportErrors: false)
    }

    func continueSession(_ session: CodexSessionSummary) async {
        isLoading = true
        defer { isLoading = false }
        do {
            let response = try await api.continueSession(session)
            let latest: CodexStatusSnapshot
            if let status = response.status {
                latest = status
            } else {
                latest = try await api.loadStatus()
            }
            snapshot = latest
            CodexSnapshotStore.save(latest)
            bridge.push(snapshot: latest)
            errorMessage = nil
        } catch {
            errorMessage = error.localizedDescription
        }
    }

    var activeServer: CodexBuddyServer? {
        guard let selectedServerID else { return nil }
        return servers.first { $0.id == selectedServerID }
    }

    func selectServer(_ server: CodexBuddyServer) async {
        api.selectServer(id: server.id)
        reloadServers()
        await refreshSilently()
    }

    func saveServer(name: String, baseURL: String, editing server: CodexBuddyServer?) throws {
        _ = try api.saveServer(name: name, baseURL: baseURL, id: server?.id)
        reloadServers()
    }

    func deleteServer(_ server: CodexBuddyServer) {
        api.deleteServer(id: server.id)
        reloadServers()
    }

    private func refresh(showLoading: Bool, reportErrors: Bool) async {
        let shouldShowLoading = showLoading
        if shouldShowLoading {
            isLoading = true
        }
        defer {
            if shouldShowLoading {
                isLoading = false
            }
        }

        do {
            let latest = try await api.loadStatus()
            snapshot = latest
            CodexSnapshotStore.save(latest)
            bridge.push(snapshot: latest)
            errorMessage = nil
        } catch {
            if reportErrors {
                errorMessage = error.localizedDescription
            }
        }
    }

    private func reloadServers() {
        servers = api.loadServers()
        selectedServerID = api.currentServer()?.id
    }
}
