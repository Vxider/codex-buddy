import Foundation

struct CodexServerEndpoint: Codable, Equatable {
    let id: String
    var name: String
    var url: String
}

final class CodexBuddyAPI {
    static let baseURLKey = "codexBuddyBaseURL"
    static let serversKey = "codexBuddyServers"
    static let activeServerIDKey = "codexBuddyActiveServerID"
    static let defaultBaseURL = "http://127.0.0.1:8787"

    private let session: URLSession
    private let defaults: UserDefaults
    private let decoder = makeCodexDecoder()

    init(session: URLSession = .shared, defaults: UserDefaults = .standard) {
        self.session = session
        self.defaults = defaults
    }

    var baseURLString: String {
        get {
            activeServer.url
        }
        set {
            let url = Self.normalizedBaseURL(newValue) ?? Self.defaultBaseURL
            var currentServers = servers
            if let index = currentServers.firstIndex(where: { $0.id == activeServer.id }) {
                currentServers[index].url = url
                currentServers[index].name = Self.defaultName(for: url)
            } else {
                currentServers = [Self.endpoint(for: url)]
            }
            saveServers(currentServers)
            defaults.set(url, forKey: Self.baseURLKey)
            defaults.set(currentServers.first?.id, forKey: Self.activeServerIDKey)
        }
    }

    var servers: [CodexServerEndpoint] {
        let decoded: [CodexServerEndpoint]
        if let data = defaults.data(forKey: Self.serversKey),
           let value = try? JSONDecoder().decode([CodexServerEndpoint].self, from: data) {
            decoded = value
        } else if let legacy = defaults.string(forKey: Self.baseURLKey),
                  let normalized = Self.normalizedBaseURL(legacy) {
            decoded = [Self.endpoint(for: normalized)]
        } else {
            decoded = [Self.endpoint(for: Self.defaultBaseURL)]
        }

        let normalized = Self.normalizedServers(decoded)
        if normalized != decoded {
            saveServers(normalized)
        }
        return normalized
    }

    var activeServer: CodexServerEndpoint {
        let currentServers = servers
        if let id = defaults.string(forKey: Self.activeServerIDKey),
           let server = currentServers.first(where: { $0.id == id }) {
            return server
        }
        let fallback = currentServers[0]
        defaults.set(fallback.id, forKey: Self.activeServerIDKey)
        defaults.set(fallback.url, forKey: Self.baseURLKey)
        return fallback
    }

    func setActiveServer(id: String) {
        guard let server = servers.first(where: { $0.id == id }) else { return }
        defaults.set(server.id, forKey: Self.activeServerIDKey)
        defaults.set(server.url, forKey: Self.baseURLKey)
    }

    @discardableResult
    func addServer(url rawURL: String) -> CodexServerEndpoint? {
        guard let url = Self.normalizedBaseURL(rawURL) else { return nil }
        var currentServers = servers
        if let existing = currentServers.first(where: { $0.url == url }) {
            setActiveServer(id: existing.id)
            return existing
        }
        let endpoint = Self.endpoint(for: url)
        currentServers.append(endpoint)
        saveServers(currentServers)
        setActiveServer(id: endpoint.id)
        return endpoint
    }

    func removeServer(id: String) {
        var currentServers = servers
        guard currentServers.count > 1 else { return }
        currentServers.removeAll { $0.id == id }
        saveServers(currentServers)
        if defaults.string(forKey: Self.activeServerIDKey) == id {
            setActiveServer(id: currentServers[0].id)
        }
    }

    @discardableResult
    func updateServer(id: String, name rawName: String, url rawURL: String) -> CodexServerEndpoint? {
        guard let url = Self.normalizedBaseURL(rawURL) else { return nil }
        var currentServers = servers
        guard let index = currentServers.firstIndex(where: { $0.id == id }) else { return nil }
        if currentServers.contains(where: { $0.id != id && $0.url == url }) {
            return nil
        }

        let name = rawName.trimmingCharacters(in: .whitespacesAndNewlines)
        currentServers[index] = CodexServerEndpoint(
            id: id,
            name: name.isEmpty ? Self.defaultName(for: url) : name,
            url: url
        )
        saveServers(currentServers)
        if defaults.string(forKey: Self.activeServerIDKey) == id {
            defaults.set(url, forKey: Self.baseURLKey)
        }
        return currentServers[index]
    }

    func loadStatus() async throws -> CodexStatusSnapshot {
        try await loadStatus(for: activeServer)
    }

    func loadStatus(for server: CodexServerEndpoint) async throws -> CodexStatusSnapshot {
        guard let base = URL(string: server.url),
              let url = URL(string: "/v1/status", relativeTo: base) else {
            throw CodexAPIError.invalidBaseURL
        }
        var request = URLRequest(url: url)
        request.httpMethod = "GET"
        request.timeoutInterval = 3
        let (data, response) = try await session.data(for: request)
        try validate(response: response, data: data)
        return try decoder.decode(CodexStatusSnapshot.self, from: data)
    }

    func streamURL() throws -> URL {
        guard let base = URL(string: baseURLString),
              let url = URL(string: "/v1/stream", relativeTo: base) else {
            throw CodexAPIError.invalidBaseURL
        }
        return url
    }

    private func validate(response: URLResponse, data: Data) throws {
        guard let http = response as? HTTPURLResponse else { return }
        guard (200..<300).contains(http.statusCode) else {
            let body = String(data: data, encoding: .utf8)?.trimmingCharacters(in: .whitespacesAndNewlines)
            throw CodexAPIError.server(http.statusCode, body ?? "request failed")
        }
    }

    private func saveServers(_ servers: [CodexServerEndpoint]) {
        if let data = try? JSONEncoder().encode(Self.normalizedServers(servers)) {
            defaults.set(data, forKey: Self.serversKey)
        }
    }

    private static func endpoint(for url: String) -> CodexServerEndpoint {
        CodexServerEndpoint(id: UUID().uuidString, name: defaultName(for: url), url: url)
    }

    private static func normalizedServers(_ value: [CodexServerEndpoint]) -> [CodexServerEndpoint] {
        var seen = Set<String>()
        let normalized = value.compactMap { server -> CodexServerEndpoint? in
            guard let url = normalizedBaseURL(server.url), !seen.contains(url) else { return nil }
            seen.insert(url)
            return CodexServerEndpoint(
                id: server.id.isEmpty ? UUID().uuidString : server.id,
                name: server.name.isEmpty ? defaultName(for: url) : server.name,
                url: url
            )
        }
        return normalized.isEmpty ? [endpoint(for: defaultBaseURL)] : normalized
    }

    private static func defaultName(for url: String) -> String {
        guard let components = URLComponents(string: url) else { return url }
        let host = components.host ?? url
        if let port = components.port {
            return "\(host):\(port)"
        }
        return host
    }

    static func normalizedBaseURL(_ value: String) -> String? {
        let trimmed = value.trimmingCharacters(in: .whitespacesAndNewlines)
        guard let components = URLComponents(string: trimmed),
              let scheme = components.scheme?.lowercased(),
              ["http", "https"].contains(scheme),
              components.host != nil,
              let url = components.url else {
            return nil
        }
        var normalized = url.absoluteString
        while normalized.hasSuffix("/") {
            normalized.removeLast()
        }
        return normalized.isEmpty ? nil : normalized
    }
}

enum CodexAPIError: LocalizedError {
    case invalidBaseURL
    case server(Int, String)

    var errorDescription: String? {
        switch self {
        case .invalidBaseURL:
            return "Set a valid Codex Buddy server URL."
        case let .server(code, message):
            return "Server error \(code): \(message)"
        }
    }
}
