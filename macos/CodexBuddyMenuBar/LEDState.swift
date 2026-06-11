import AppKit

enum LEDState: String {
    case off
    case working
    case attention
    case approval
    case goal

    var color: NSColor {
        switch self {
        case .off:
            return .systemGray
        case .working:
            return .systemGreen
        case .attention:
            return .systemYellow
        case .approval:
            return .systemRed
        case .goal:
            return .systemPurple
        }
    }

    var label: String {
        switch self {
        case .off:
            return "Off"
        case .working:
            return "Working"
        case .attention:
            return "Attention"
        case .approval:
            return "Approval"
        case .goal:
            return "Goal"
        }
    }

    static func aggregate(snapshot: CodexStatusSnapshot) -> LEDState {
        guard !snapshot.sessions.isEmpty else {
            return overall(snapshot.overallState)
        }
        var result = LEDState.off
        for session in snapshot.sessions {
            result = stronger(result, sessionState(session))
        }
        return result
    }

    private static func sessionState(_ session: CodexSessionSummary) -> LEDState {
        let detail = (session.stateDetail ?? "").lowercased()
        let reason = (session.openReason ?? "").lowercased()
        if session.goalState == "achieved" || session.goalState == "complete" || session.goalState == "completed" {
            return .goal
        }
        if session.needsApproval || reason == "approval" {
            return .approval
        }
        if detail.contains("permissionrequest") || detail.contains("permission request") {
            return .approval
        }
        if reason == "followup" && session.needsOpen && !session.needsApproval {
            return .attention
        }
        let state = session.state.normalized
        if state != .idle && state != .run && state != .runningBash && (session.needsOpen || !reason.isEmpty) {
            return .attention
        }
        if state == .run || state == .runningBash {
            return .working
        }
        return .off
    }

    private static func overall(_ state: CodexState) -> LEDState {
        switch state.normalized {
        case .run, .runningBash:
            return .working
        case .open, .error:
            return .attention
        default:
            return .off
        }
    }

    private static func stronger(_ current: LEDState, _ candidate: LEDState) -> LEDState {
        rank(candidate) > rank(current) ? candidate : current
    }

    private static func rank(_ state: LEDState) -> Int {
        switch state {
        case .goal:
            return 40
        case .approval:
            return 30
        case .attention:
            return 20
        case .working:
            return 10
        case .off:
            return 0
        }
    }
}
