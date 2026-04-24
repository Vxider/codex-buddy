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

    private func complicationImage(in size: CGSize) -> some View {
        widgetGlyph(in: size)
            .frame(width: size.width * 0.82, height: size.height * 0.82)
            .foregroundStyle(.white)
            .widgetAccentable()
            .frame(width: size.width, height: size.height)
            .containerBackground(for: .widget) {
                Color.clear
            }
    }

    private func widgetGlyph(in size: CGSize) -> some View {
        let iconSize = min(size.width, size.height)
        let stroke = iconSize * 0.045
        let eyeSize = iconSize * 0.145
        let eyeOffsetX = iconSize * 0.16
        let eyeOffsetY = -iconSize * 0.14
        let mouthWidth = iconSize * 0.28
        let mouthHeight = stroke
        let mouthOffsetY = iconSize * 0.14

        return ZStack {
            Circle()
                .stroke(style: StrokeStyle(lineWidth: stroke, lineCap: .round, lineJoin: .round))

            Circle()
                .frame(width: eyeSize, height: eyeSize)
                .offset(x: -eyeOffsetX, y: eyeOffsetY)

            Circle()
                .frame(width: eyeSize, height: eyeSize)
                .offset(x: eyeOffsetX, y: eyeOffsetY)

            Capsule(style: .circular)
                .frame(width: mouthWidth, height: mouthHeight)
                .rotationEffect(.degrees(45))
                .offset(y: mouthOffsetY)

            Capsule(style: .circular)
                .frame(width: mouthWidth, height: mouthHeight)
                .rotationEffect(.degrees(-45))
                .offset(y: mouthOffsetY)
        }
        .padding(stroke * 0.55)
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
