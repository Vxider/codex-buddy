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
        let mouthWidth = iconSize * 0.34
        let mouthHeight = iconSize * 0.18
        let mouthOffsetY = iconSize * 0.12

        return ZStack {
            Circle()
                .stroke(style: StrokeStyle(lineWidth: stroke, lineCap: .round, lineJoin: .round))

            Circle()
                .frame(width: eyeSize, height: eyeSize)
                .offset(x: -eyeOffsetX, y: eyeOffsetY)

            Circle()
                .frame(width: eyeSize, height: eyeSize)
                .offset(x: eyeOffsetX, y: eyeOffsetY)

            Path { path in
                let start = CGPoint(
                    x: (iconSize - mouthWidth) / 2,
                    y: (iconSize / 2) + mouthOffsetY
                )
                let end = CGPoint(
                    x: start.x + mouthWidth,
                    y: start.y
                )
                let control = CGPoint(
                    x: iconSize / 2,
                    y: start.y + mouthHeight
                )
                path.move(to: start)
                path.addQuadCurve(to: end, control: control)
            }
            .stroke(style: StrokeStyle(lineWidth: stroke * 1.2, lineCap: .round, lineJoin: .round))
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
