import Foundation

enum CodexDeepLink: Equatable {
    case sessions
    case serverSettings

    init?(url: URL) {
        guard url.scheme?.lowercased() == "codexbuddy" else {
            return nil
        }

        switch url.host?.lowercased() {
        case "sessions":
            self = .sessions
        case "server", "settings":
            self = .serverSettings
        default:
            return nil
        }
    }
}
