import Foundation

enum CodexSnapshotStore {
    static let appGroupID = "group.com.vxider.codexbuddy"
    private static let snapshotKey = "latest_status_snapshot"
    private static let isoDecoder = makeDecoder()
    private static let isoEncoder = makeEncoder()

    static func load() -> CodexStatusSnapshot? {
        guard let data = defaults.data(forKey: snapshotKey) else { return nil }
        return try? isoDecoder.decode(CodexStatusSnapshot.self, from: data)
    }

    static func save(_ snapshot: CodexStatusSnapshot) {
        guard let data = try? isoEncoder.encode(snapshot) else { return }
        defaults.set(data, forKey: snapshotKey)
    }

    static func clear() {
        defaults.removeObject(forKey: snapshotKey)
    }

    private static var defaults: UserDefaults {
        if let shared = UserDefaults(suiteName: appGroupID) {
            return shared
        }
        return .standard
    }
}

func makeDecoder() -> JSONDecoder {
    let decoder = JSONDecoder()
    let formatter = ISO8601DateFormatter()
    formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
    decoder.dateDecodingStrategy = .custom { decoder in
        let container = try decoder.singleValueContainer()
        let raw = try container.decode(String.self)
        if let date = formatter.date(from: raw) {
            return date
        }
        let fallback = ISO8601DateFormatter()
        fallback.formatOptions = [.withInternetDateTime]
        if let date = fallback.date(from: raw) {
            return date
        }
        throw DecodingError.dataCorruptedError(in: container, debugDescription: "Invalid ISO8601 date: \(raw)")
    }
    return decoder
}

func makeEncoder() -> JSONEncoder {
    let encoder = JSONEncoder()
    let formatter = ISO8601DateFormatter()
    formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
    encoder.dateEncodingStrategy = .custom { date, encoder in
        var container = encoder.singleValueContainer()
        try container.encode(formatter.string(from: date))
    }
    return encoder
}
