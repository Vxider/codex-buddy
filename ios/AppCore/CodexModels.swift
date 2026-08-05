import Foundation

enum CodexState: String, Codable, CaseIterable, Hashable {
    case offline
    case idle
    case run
    case running
    case runningBash = "running_bash"
    case open
    case error

    init(from decoder: Decoder) throws {
        let container = try decoder.singleValueContainer()
        let rawValue = try container.decode(String.self)
        switch rawValue {
        case "offline":
            self = .offline
        case "idle":
            self = .idle
        case "run":
            self = .run
        case "running":
            self = .running
        case "running_bash":
            self = .runningBash
        case "open":
            self = .open
        case "error":
            self = .error
        default:
            throw DecodingError.dataCorruptedError(in: container, debugDescription: "Unknown CodexState: \(rawValue)")
        }
    }

    var normalized: CodexState {
        switch self {
        case .run, .running, .runningBash:
            return .run
        default:
            return self
        }
    }

    var displayName: String {
        switch self.normalized {
        case .offline:
            return "Offline"
        case .idle:
            return "Idle"
        case .run, .running:
            return "RUN"
        case .open:
            return "OPEN"
        case .error:
            return "Error"
        case .runningBash:
            return "Running"
        }
    }

    var face: String {
        switch self.normalized {
        case .offline:
            return "❔"
        case .idle:
            return "😊"
        case .run, .running:
            return "🫡"
        case .open:
            return "🟠"
        case .error:
            return "😵"
        case .runningBash:
            return "🫡"
        }
    }

    var accentName: String {
        switch self.normalized {
        case .offline:
            return "secondary"
        case .idle:
            return "green"
        case .run, .running:
            return "blue"
        case .open:
            return "orange"
        case .error:
            return "red"
        case .runningBash:
            return "blue"
        }
    }
}

struct CodexContinueAction: Codable, Hashable {
    let method: String
    let endpoint: String
    let actionToken: String
    let label: String?

    enum CodingKeys: String, CodingKey {
        case method
        case endpoint
        case actionToken = "action_token"
        case label
    }
}

struct CodexSessionSummary: Codable, Identifiable, Hashable {
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
    let openSummary: String?
    let compactOpenSummary: String?
    let microOpenSummary: String?
    let canContinue: Bool
    let continueAction: CodexContinueAction?

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
        case openSummary = "open_summary"
        case compactOpenSummary = "compact_open_summary"
        case microOpenSummary = "micro_open_summary"
        case canContinue = "can_continue"
        case continueAction = "continue_action"
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        sessionID = try container.decode(String.self, forKey: .sessionID)
        shortSessionID = try container.decodeIfPresent(String.self, forKey: .shortSessionID)
        displayTitle = try container.decodeIfPresent(String.self, forKey: .displayTitle)
        compactTitle = try container.decodeIfPresent(String.self, forKey: .compactTitle)
        microTitle = try container.decodeIfPresent(String.self, forKey: .microTitle)
        state = try container.decode(CodexState.self, forKey: .state)
        stateDetail = try container.decodeIfPresent(String.self, forKey: .stateDetail)
        updatedAt = try container.decodeIfPresent(Date.self, forKey: .updatedAt)
        summary = try container.decodeIfPresent(String.self, forKey: .summary)
        compactSummary = try container.decodeIfPresent(String.self, forKey: .compactSummary)
        microSummary = try container.decodeIfPresent(String.self, forKey: .microSummary)
        needsOpen = try container.decodeIfPresent(Bool.self, forKey: .needsOpen) ?? false
        openSummary = try container.decodeIfPresent(String.self, forKey: .openSummary)
        compactOpenSummary = try container.decodeIfPresent(String.self, forKey: .compactOpenSummary)
        microOpenSummary = try container.decodeIfPresent(String.self, forKey: .microOpenSummary)
        canContinue = try container.decodeIfPresent(Bool.self, forKey: .canContinue) ?? false
        continueAction = try container.decodeIfPresent(CodexContinueAction.self, forKey: .continueAction)
    }

    var id: String { sessionID }

    var phoneListTitle: String {
        if let compactTitle, !compactTitle.isEmpty {
            return compactTitle
        }
        return fallbackTitle
    }

    var watchListTitle: String {
        if let microTitle, !microTitle.isEmpty {
            return microTitle
        }
        return fallbackTitle
    }

    private var fallbackTitle: String {
        if let displayTitle, !displayTitle.isEmpty {
            return displayTitle
        }
        if let shortSessionID, !shortSessionID.isEmpty {
            return shortSessionID
        }
        return sessionID
    }

    var phoneAssistantSummary: String {
        if let compactSummary, !compactSummary.isEmpty {
            return compactSummary
        }
        return fallbackSummary
    }

    var watchAssistantSummary: String {
        if let microSummary, !microSummary.isEmpty {
            return microSummary
        }
        return fallbackSummary
    }

    private var fallbackSummary: String {
        if let openSummary, !openSummary.isEmpty {
            return openSummary
        }
        return summary ?? ""
    }

    var phoneTitle: String { phoneListTitle }

    var watchTitle: String { watchListTitle }

    var phoneSummary: String {
        if let openSummary, !openSummary.isEmpty {
            return openSummary
        }
        if let compactOpenSummary, !compactOpenSummary.isEmpty {
            return compactOpenSummary
        }
        if let compactSummary, !compactSummary.isEmpty {
            return compactSummary
        }
        return summary ?? ""
    }

    var watchSummary: String {
        if let openSummary, !openSummary.isEmpty {
            return openSummary
        }
        if let microOpenSummary, !microOpenSummary.isEmpty {
            return microOpenSummary
        }
        if let compactOpenSummary, !compactOpenSummary.isEmpty {
            return compactOpenSummary
        }
        if let microSummary, !microSummary.isEmpty {
            return microSummary
        }
        return phoneSummary
    }
}

struct CodexStatusSnapshot: Codable, Hashable {
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

struct CodexContinueResponse: Codable, Hashable {
    let ok: Bool?
    let message: String?
    let session: CodexSessionSummary?
    let status: CodexStatusSnapshot?
}

struct CodexBridgeEnvelope: Codable {
    let snapshot: CodexStatusSnapshot
}
