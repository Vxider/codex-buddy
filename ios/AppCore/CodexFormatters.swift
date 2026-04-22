import Foundation

enum CodexFormatters {
    private static let absoluteFormatter: DateFormatter = {
        let formatter = DateFormatter()
        formatter.dateStyle = .none
        formatter.timeStyle = .short
        return formatter
    }()

    static func shortTime(_ date: Date?) -> String {
        guard let date else { return "-" }
        return absoluteFormatter.string(from: date)
    }

    static func relativeTime(_ date: Date?) -> String {
        guard let date else { return "-" }
        let seconds = Int(Date().timeIntervalSince(date))
        if seconds < 60 { return "just now" }
        if seconds < 3600 { return "\(seconds / 60)m" }
        if seconds < 86400 { return "\(seconds / 3600)h" }
        return "\(seconds / 86400)d"
    }

    static func shortSessionLabel(_ sessionID: String?) -> String {
        guard let sessionID, !sessionID.isEmpty else { return "-" }
        if sessionID.count <= 10 { return sessionID }
        return String(sessionID.prefix(8))
    }
}
