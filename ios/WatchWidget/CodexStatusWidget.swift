import SwiftUI
import WidgetKit

struct CodexStatusEntry: TimelineEntry {
    let date: Date
}

struct CodexStatusProvider: TimelineProvider {
    func placeholder(in context: Context) -> CodexStatusEntry {
        CodexStatusEntry(date: Date())
    }

    func getSnapshot(in context: Context, completion: @escaping (CodexStatusEntry) -> Void) {
        completion(CodexStatusEntry(date: Date()))
    }

    func getTimeline(in context: Context, completion: @escaping (Timeline<CodexStatusEntry>) -> Void) {
        let entry = CodexStatusEntry(date: Date())
        completion(Timeline(entries: [entry], policy: .never))
    }
}

struct CodexStatusWidgetEntryView: View {
    let entry: CodexStatusEntry

    var body: some View {
        GeometryReader { proxy in
            complicationImage(in: proxy.size)
        }
    }

    @ViewBuilder
    private func complicationImage(in size: CGSize) -> some View {
        Canvas { context, canvasSize in
            context.clip(to: Path(ellipseIn: CGRect(origin: .zero, size: canvasSize)))
            let image = context.resolve(Image("WidgetIcon"))
            context.draw(image, in: CGRect(origin: .zero, size: canvasSize))
        }
        .frame(width: size.width, height: size.height)
        .containerBackground(for: ContainerBackgroundPlacement.widget) {
            Color.clear
        }
        .widgetAccentable(false)
    }
}

@main
struct CodexStatusWidget: Widget {
    var body: some WidgetConfiguration {
        StaticConfiguration(kind: "CodexStatusWidget", provider: CodexStatusProvider()) { entry in
            CodexStatusWidgetEntryView(entry: entry)
                .widgetURL(URL(string: "codexbuddy://sessions"))
        }
        .configurationDisplayName("Codex Status")
        .description("Shows the Codex Buddy icon.")
        .supportedFamilies([.accessoryCircular])
    }
}
