import Foundation

enum CodexState: String, Decodable {
    case offline
    case idle
    case run
    case running
    case runningBash = "running_bash"
    case open
    case error

    init(from decoder: Decoder) throws {
        let container = try decoder.singleValueContainer()
        let value = try container.decode(String.self)
        self = CodexState(rawValue: value) ?? .idle
    }

    var normalized: CodexState {
        switch self {
        case .running:
            return .run
        default:
            return self
        }
    }

    var label: String {
        switch normalized {
        case .offline:
            return "Offline"
        case .idle:
            return "Idle"
        case .run, .running, .runningBash:
            return "Working"
        case .open:
            return "Attention"
        case .error:
            return "Error"
        }
    }
}

struct CodexStatusSnapshot: Decodable {
    let serverTime: Date?
    let overallState: CodexState
    let overallStateDetail: String?
    let sessionsCount: Int
    let sessions: [CodexSessionSummary]

    enum CodingKeys: String, CodingKey {
        case serverTime = "server_time"
        case overallState = "overall_state"
        case overallStateDetail = "overall_state_detail"
        case sessionsCount = "sessions_count"
        case sessions
    }

    static let offline = CodexStatusSnapshot(
        serverTime: nil,
        overallState: .offline,
        overallStateDetail: nil,
        sessionsCount: 0,
        sessions: []
    )
}

struct CodexSessionSummary: Decodable, Identifiable {
    let sessionID: String
    let shortSessionID: String?
    let displayTitle: String?
    let compactTitle: String?
    let microTitle: String?
    let state: CodexState
    let stateDetail: String?
    let updatedAt: Date?
    let summary: String?
    let compactSummary: String?
    let microSummary: String?
    let needsOpen: Bool
    let needsApproval: Bool
    let openReason: String?
    let goalState: String?
    let goalUpdatedAt: Date?
    let openSummary: String?
    let compactOpenSummary: String?
    let microOpenSummary: String?

    enum CodingKeys: String, CodingKey {
        case sessionID = "session_id"
        case shortSessionID = "short_session_id"
        case displayTitle = "display_title"
        case compactTitle = "compact_title"
        case microTitle = "micro_title"
        case state
        case stateDetail = "state_detail"
        case updatedAt = "updated_at"
        case summary
        case compactSummary = "compact_summary"
        case microSummary = "micro_summary"
        case needsOpen = "needs_open"
        case needsApproval = "needs_approval"
        case openReason = "open_reason"
        case goalState = "goal_state"
        case goalUpdatedAt = "goal_updated_at"
        case openSummary = "open_summary"
        case compactOpenSummary = "compact_open_summary"
        case microOpenSummary = "micro_open_summary"
    }

    var id: String { sessionID }
    var title: String { firstNonEmpty(compactTitle, displayTitle, shortSessionID, sessionID) }
    var subtitle: String { shortSessionID ?? String(sessionID.prefix(8)) }
    var message: String {
        firstNonEmpty(openSummary, compactOpenSummary, compactSummary, microSummary, summary)
    }
}

func makeCodexDecoder() -> JSONDecoder {
    let decoder = JSONDecoder()
    decoder.dateDecodingStrategy = .custom { decoder in
        let container = try decoder.singleValueContainer()
        let raw = try container.decode(String.self)
        if let date = DateParsers.fractional.date(from: raw) {
            return date
        }
        if let date = DateParsers.standard.date(from: raw) {
            return date
        }
        throw DecodingError.dataCorruptedError(in: container, debugDescription: "Invalid date: \(raw)")
    }
    return decoder
}

private enum DateParsers {
    static let fractional: ISO8601DateFormatter = {
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        return formatter
    }()

    static let standard: ISO8601DateFormatter = {
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime]
        return formatter
    }()
}

func firstNonEmpty(_ values: String?...) -> String {
    for value in values {
        let trimmed = value?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        if !trimmed.isEmpty {
            return trimmed
        }
    }
    return ""
}
