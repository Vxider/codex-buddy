import Foundation

final class CodexBuddyAPI {
    static let baseURLKey = "codexBuddyBaseURL"
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
            defaults.string(forKey: Self.baseURLKey) ?? Self.defaultBaseURL
        }
        set {
            defaults.set(Self.normalizedBaseURL(newValue) ?? Self.defaultBaseURL, forKey: Self.baseURLKey)
        }
    }

    func loadStatus() async throws -> CodexStatusSnapshot {
        guard let base = URL(string: baseURLString),
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
