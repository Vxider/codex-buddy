import SwiftUI
import WidgetKit

struct CodexStatusEntry: TimelineEntry {
    let date: Date
    let snapshot: CodexStatusSnapshot
}

struct CodexStatusProvider: TimelineProvider {
    func placeholder(in context: Context) -> CodexStatusEntry {
        CodexStatusEntry(date: Date(), snapshot: .offline)
    }

    func getSnapshot(in context: Context, completion: @escaping (CodexStatusEntry) -> Void) {
        let snapshot = CodexSnapshotStore.load() ?? .offline
        completion(CodexStatusEntry(date: Date(), snapshot: snapshot))
    }

    func getTimeline(in context: Context, completion: @escaping (Timeline<CodexStatusEntry>) -> Void) {
        let snapshot = CodexSnapshotStore.load() ?? .offline
        let entry = CodexStatusEntry(date: Date(), snapshot: snapshot)
        completion(Timeline(entries: [entry], policy: .after(Date().addingTimeInterval(30 * 60))))
    }
}

struct CodexStatusWidgetEntryView: View {
    let entry: CodexStatusEntry

    var body: some View {
        ZStack {
            AccessoryWidgetBackground()
            VStack(spacing: 2) {
                Text(entry.snapshot.overallState.face)
                    .font(.system(size: 24))
                Text(entry.snapshot.overallState.displayName)
                    .font(.system(size: 9, weight: .semibold))
                    .lineLimit(1)
            }
        }
        .widgetURL(URL(string: "codexbuddy://sessions"))
    }
}

@main
struct CodexStatusWidget: Widget {
    var body: some WidgetConfiguration {
        StaticConfiguration(kind: "CodexStatusWidget", provider: CodexStatusProvider()) { entry in
            CodexStatusWidgetEntryView(entry: entry)
        }
        .configurationDisplayName("Codex Status")
        .description("Shows the aggregated Codex state as a face.")
        .supportedFamilies([.accessoryCircular])
    }
}
