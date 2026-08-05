import AppKit

enum LEDState: String {
    case off
    case working
    case attention
    case approval
    case error
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
            return .systemYellow
        case .error:
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
        case .error:
            return "Codex abnormal interruption"
        case .goal:
            return "Goal"
        }
    }

    static func aggregate(snapshot: CodexStatusSnapshot) -> LEDState {
        if snapshot.overallState == .error {
            return .error
        }
        guard !snapshot.sessions.isEmpty else {
            return overall(snapshot.overallState, snapshot.overallStateDetail)
        }
        var result = LEDState.off
        for session in snapshot.sessions {
            result = stronger(result, forSession(session))
        }
        if isCodexInterruptionText(snapshot.overallStateDetail) {
            return .error
        }
        if result == .attention {
            return result
        }
        return result
    }

    static func forSession(_ session: CodexSessionSummary) -> LEDState {
        let state = session.state.normalized
        let detail = (session.stateDetail ?? "").lowercased()
        if state == .error {
            return .error
        }
        let reason = (session.openReason ?? "").lowercased()
        if session.needsApproval || reason == "approval" {
            return .attention
        }
        if detail.contains("permissionrequest") || detail.contains("permission request") {
            return .attention
        }
        if isCodexInterruptionText(detail, session.message) {
            return .error
        }
        if session.goalState == "achieved" || session.goalState == "complete" || session.goalState == "completed" {
            return .goal
        }
        if reason == "followup" && session.needsOpen && !session.needsApproval {
            return .attention
        }
        if state != .idle && state != .run && state != .runningBash && (session.needsOpen || !reason.isEmpty) {
            return .attention
        }
        if state == .run || state == .runningBash {
            return .working
        }
        return .off
    }

    private static func overall(_ state: CodexState, _ detail: String?) -> LEDState {
        if state == .error || isCodexInterruptionText(detail) {
            return .error
        }
        switch state.normalized {
        case .run, .runningBash:
            return .working
        case .open:
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
        case .error:
            return 50
        case .goal:
            return 40
        case .approval:
            return 20
        case .attention:
            return 20
        case .working:
            return 10
        case .off:
            return 0
        }
    }

    private static func isCodexInterruptionText(_ values: String?...) -> Bool {
        let markers = [
            "unauthorized", "forbidden", "authentication failed", "invalid api key",
            "invalid token", "payment required", "billing error", "billing required", "quota exhausted", "quota exceeded", "quota limit", "rate limit", "too many requests",
            "usage limit", "credits exhausted", "insufficient credits", "out of credits",
            "limit reached", "token budget exhausted", "usage exhausted", "network error", "network failure", "network unavailable", "network connection", "connection refused", "connection reset", "connection closed", "connection lost", "timed out", "timeout",
            "dns", "socket", "tls handshake", "transport error", "service unavailable", "internal server error", "server error", "service error", "disconnected",
            "codex error", "codex interrupted", "turn failed", "turn aborted", "task failed", "turn interrupted",
            "认证失败", "未授权", "额度", "配额", "用光", "用完", "超额", "限额", "余额不足",
            "网络错误", "网络中断", "网络连接", "网络不可用", "连接失败", "连接中断", "连接断开", "超时", "服务不可用", "请求失败", "异常中断",
        ]
        return values.contains { value in
            let text = (value ?? "").lowercased()
            if ["401", "402", "403", "429", "500", "502", "503", "504"].contains(where: { containsStatusCode(text, $0) }) {
                return true
            }
            return markers.contains { text.contains($0) }
        }
    }

    private static func containsStatusCode(_ text: String, _ code: String) -> Bool {
        var searchStart = text.startIndex
        while searchStart < text.endIndex,
              let range = text.range(of: code, range: searchStart..<text.endIndex) {
            let before = range.lowerBound > text.startIndex ? text[text.index(before: range.lowerBound)] : nil
            let after = range.upperBound < text.endIndex ? text[range.upperBound] : nil
            if !isASCIIDigit(before) && !isASCIIDigit(after) {
                return true
            }
            searchStart = range.upperBound
        }
        return false
    }

    private static func isASCIIDigit(_ value: Character?) -> Bool {
        guard let scalar = value?.unicodeScalars.first?.value else {
            return false
        }
        return scalar >= 48 && scalar <= 57
    }
}
