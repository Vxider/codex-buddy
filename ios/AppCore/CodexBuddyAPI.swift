import Foundation

struct CodexBuddyServer: Codable, Identifiable, Equatable {
    let id: UUID
    var name: String
    var baseURL: String

    init(id: UUID = UUID(), name: String, baseURL: String) {
        self.id = id
        self.name = name
        self.baseURL = baseURL
    }

    var displayName: String {
        if !name.isEmpty {
            return name
        }
        if let host = URL(string: baseURL)?.host(), !host.isEmpty {
            return host
        }
        return baseURL
    }
}

final class CodexBuddyAPI {
    static let shared = CodexBuddyAPI()

    static let baseURLKey = "codexBuddyBaseURL"
    static let serversKey = "codexBuddyServers"
    static let selectedServerIDKey = "codexBuddySelectedServerID"
    static let infoPlistBaseURLKey = "CodexBuddyBaseURL"

    private let decoder = makeDecoder()
    private let encoder = makeEncoder()
    private let session: URLSession
    private let defaults: UserDefaults

    init(session: URLSession = .shared, defaults: UserDefaults = .standard) {
        self.session = session
        self.defaults = defaults
    }

    func currentBaseURLString(bundle: Bundle = .main) -> String {
        if let server = currentServer(bundle: bundle) {
            return server.baseURL
        }
        if let fromInfo = bundle.object(forInfoDictionaryKey: Self.infoPlistBaseURLKey) as? String {
            return fromInfo
        }
        return ""
    }

    func loadServers(bundle: Bundle = .main) -> [CodexBuddyServer] {
        migrateLegacyServerIfNeeded(bundle: bundle)
        guard let data = defaults.data(forKey: Self.serversKey) else {
            return []
        }
        return (try? decoder.decode([CodexBuddyServer].self, from: data)) ?? []
    }

    func currentServer(bundle: Bundle = .main) -> CodexBuddyServer? {
        let servers = loadServers(bundle: bundle)
        if let rawID = defaults.string(forKey: Self.selectedServerIDKey),
           let id = UUID(uuidString: rawID),
           let server = servers.first(where: { $0.id == id }) {
            return server
        }
        return servers.first
    }

    func saveServer(name: String, baseURL: String, id: UUID? = nil, bundle: Bundle = .main) throws -> CodexBuddyServer {
        var servers = loadServers(bundle: bundle)
        let selectedServerID = currentServer(bundle: bundle)?.id
        let normalizedURL = try normalizedBaseURL(baseURL)
        let normalizedName = normalizedServerName(name, baseURL: normalizedURL)
        let server = CodexBuddyServer(id: id ?? UUID(), name: normalizedName, baseURL: normalizedURL)

        if let index = servers.firstIndex(where: { $0.id == server.id }) {
            servers[index] = server
        } else {
            servers.append(server)
        }

        persistServers(servers)
        if id == nil || selectedServerID == nil || selectedServerID == server.id {
            selectServer(id: server.id, bundle: bundle)
        }
        return server
    }

    func deleteServer(id: UUID, bundle: Bundle = .main) {
        var servers = loadServers(bundle: bundle)
        servers.removeAll { $0.id == id }
        persistServers(servers)

        if defaults.string(forKey: Self.selectedServerIDKey) == id.uuidString {
            if let first = servers.first {
                defaults.set(first.id.uuidString, forKey: Self.selectedServerIDKey)
            } else {
                defaults.removeObject(forKey: Self.selectedServerIDKey)
            }
        }
    }

    func selectServer(id: UUID, bundle: Bundle = .main) {
        let servers = loadServers(bundle: bundle)
        guard servers.contains(where: { $0.id == id }) else { return }
        defaults.set(id.uuidString, forKey: Self.selectedServerIDKey)
    }

    func loadStatus(bundle: Bundle = .main) async throws -> CodexStatusSnapshot {
        let url = try statusURL(bundle: bundle)
        var request = URLRequest(url: url)
        request.httpMethod = "GET"
        let (data, response) = try await self.session.data(for: request)
        try validate(response: response, data: data)
        return try decoder.decode(CodexStatusSnapshot.self, from: data)
    }

    func continueSession(_ item: CodexSessionSummary, bundle: Bundle = .main) async throws -> CodexContinueResponse {
        guard let action = item.continueAction else {
            throw CodexAPIError.missingContinueAction
        }
        let base = try baseURL(bundle: bundle)
        guard let endpoint = URL(string: action.endpoint, relativeTo: base) else {
            throw CodexAPIError.invalidBaseURL
        }
        var request = URLRequest(url: endpoint)
        request.httpMethod = action.method
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.httpBody = try encoder.encode(["action_token": action.actionToken])
        let (data, response) = try await self.session.data(for: request)
        try validate(response: response, data: data)
        return try decoder.decode(CodexContinueResponse.self, from: data)
    }

    private func statusURL(bundle: Bundle) throws -> URL {
        let base = try baseURL(bundle: bundle)
        guard let url = URL(string: "/v1/status", relativeTo: base) else {
            throw CodexAPIError.invalidBaseURL
        }
        return url
    }

    private func baseURL(bundle: Bundle) throws -> URL {
        let raw = currentBaseURLString(bundle: bundle).trimmingCharacters(in: .whitespacesAndNewlines)
        guard !raw.isEmpty, let url = URL(string: raw) else {
            throw CodexAPIError.invalidBaseURL
        }
        return url
    }

    private func persistServers(_ servers: [CodexBuddyServer]) {
        guard let data = try? encoder.encode(servers) else { return }
        defaults.set(data, forKey: Self.serversKey)
    }

    private func migrateLegacyServerIfNeeded(bundle: Bundle) {
        if defaults.data(forKey: Self.serversKey) != nil {
            return
        }

        let legacy = defaults.string(forKey: Self.baseURLKey)?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        guard !legacy.isEmpty, let normalized = try? normalizedBaseURL(legacy) else {
            return
        }

        let server = CodexBuddyServer(name: normalizedServerName("", baseURL: normalized), baseURL: normalized)
        persistServers([server])
        defaults.set(server.id.uuidString, forKey: Self.selectedServerIDKey)
        defaults.removeObject(forKey: Self.baseURLKey)
    }

    private func normalizedBaseURL(_ value: String) throws -> String {
        let trimmed = value.trimmingCharacters(in: .whitespacesAndNewlines)
        guard let components = URLComponents(string: trimmed),
              let scheme = components.scheme?.lowercased(),
              ["http", "https"].contains(scheme),
              components.host != nil,
              let url = components.url else {
            throw CodexAPIError.invalidServerURL
        }

        var normalized = url.absoluteString
        if normalized.hasSuffix("/") {
            normalized.removeLast()
        }
        return normalized
    }

    private func normalizedServerName(_ value: String, baseURL: String) -> String {
        let trimmed = value.trimmingCharacters(in: .whitespacesAndNewlines)
        if !trimmed.isEmpty {
            return trimmed
        }
        if let host = URL(string: baseURL)?.host(), !host.isEmpty {
            return host
        }
        return baseURL
    }

    private func validate(response: URLResponse, data: Data) throws {
        guard let http = response as? HTTPURLResponse else { return }
        guard (200..<300).contains(http.statusCode) else {
            let message = String(data: data, encoding: .utf8)?.trimmingCharacters(in: .whitespacesAndNewlines)
            throw CodexAPIError.server(http.statusCode, message ?? "request failed")
        }
    }
}

enum CodexAPIError: LocalizedError {
    case invalidBaseURL
    case invalidServerURL
    case missingContinueAction
    case server(Int, String)
    case bridgeUnavailable(String)

    var errorDescription: String? {
        switch self {
        case .invalidBaseURL:
            return "Set a valid Codex Buddy base URL first."
        case .invalidServerURL:
            return "Enter a valid http:// or https:// server URL."
        case .missingContinueAction:
            return "This session can no longer continue."
        case let .server(code, message):
            return "Server error \(code): \(message)"
        case let .bridgeUnavailable(message):
            return message
        }
    }
}
