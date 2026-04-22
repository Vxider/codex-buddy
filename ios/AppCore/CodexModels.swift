import Foundation

enum CodexState: String, Codable, CaseIterable, Hashable {
    case offline
    case idle
    case running
    case runningBash = "running_bash"
    case attention
    case error

    var normalized: CodexState {
        switch self {
        case .runningBash:
            return .running
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
        case .running:
            return "Running"
        case .attention:
            return "Attention"
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
        case .running:
            return "🫡"
        case .attention:
            return "⚠️"
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
        case .running:
            return "blue"
        case .attention:
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
    let needsAttention: Bool
    let attentionSummary: String?
    let compactAttentionSummary: String?
    let microAttentionSummary: String?
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
        case needsAttention = "needs_attention"
        case attentionSummary = "attention_summary"
        case compactAttentionSummary = "compact_attention_summary"
        case microAttentionSummary = "micro_attention_summary"
        case canContinue = "can_continue"
        case continueAction = "continue_action"
    }

    var id: String { sessionID }

    var phoneTitle: String {
        if let compactTitle, !compactTitle.isEmpty {
            return compactTitle
        }
        if let displayTitle, !displayTitle.isEmpty {
            return displayTitle
        }
        if let shortSessionID, !shortSessionID.isEmpty {
            return shortSessionID
        }
        return sessionID
    }

    var watchTitle: String {
        if let microTitle, !microTitle.isEmpty {
            return microTitle
        }
        return phoneTitle
    }

    var phoneSummary: String {
        if let compactAttentionSummary, !compactAttentionSummary.isEmpty {
            return compactAttentionSummary
        }
        if let attentionSummary, !attentionSummary.isEmpty {
            return attentionSummary
        }
        if let compactSummary, !compactSummary.isEmpty {
            return compactSummary
        }
        return summary ?? ""
    }

    var watchSummary: String {
        if let microAttentionSummary, !microAttentionSummary.isEmpty {
            return microAttentionSummary
        }
        if let compactAttentionSummary, !compactAttentionSummary.isEmpty {
            return compactAttentionSummary
        }
        if let attentionSummary, !attentionSummary.isEmpty {
            return attentionSummary
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
    let activeSessionID: String?
    let activeSessionDisplayTitle: String?
    let activeSessionCompactTitle: String?
    let activeSessionMicroTitle: String?
    let sessionsCount: Int
    let sessions: [CodexSessionSummary]

    enum CodingKeys: String, CodingKey {
        case serverTime = "server_time"
        case overallState = "overall_state"
        case overallStateDetail = "overall_state_detail"
        case activeSessionID = "active_session_id"
        case activeSessionDisplayTitle = "active_session_display_title"
        case activeSessionCompactTitle = "active_session_compact_title"
        case activeSessionMicroTitle = "active_session_micro_title"
        case sessionsCount = "sessions_count"
        case sessions
    }

    static let offline = CodexStatusSnapshot(
        serverTime: nil,
        overallState: .offline,
        overallStateDetail: nil,
        activeSessionID: nil,
        activeSessionDisplayTitle: nil,
        activeSessionCompactTitle: nil,
        activeSessionMicroTitle: nil,
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
