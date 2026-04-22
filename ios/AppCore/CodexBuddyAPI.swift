import Foundation

final class CodexBuddyAPI {
    static let shared = CodexBuddyAPI()

    static let baseURLKey = "codexBuddyBaseURL"
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
        if let stored = defaults.string(forKey: Self.baseURLKey), !stored.isEmpty {
            return stored
        }
        if let fromInfo = bundle.object(forInfoDictionaryKey: Self.infoPlistBaseURLKey) as? String {
            return fromInfo
        }
        return ""
    }

    func updateBaseURL(_ value: String) {
        defaults.set(value.trimmingCharacters(in: .whitespacesAndNewlines), forKey: Self.baseURLKey)
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
    case missingContinueAction
    case server(Int, String)
    case bridgeUnavailable(String)

    var errorDescription: String? {
        switch self {
        case .invalidBaseURL:
            return "Set a valid Codex Buddy base URL first."
        case .missingContinueAction:
            return "This session can no longer continue."
        case let .server(code, message):
            return "Server error \(code): \(message)"
        case let .bridgeUnavailable(message):
            return message
        }
    }
}
