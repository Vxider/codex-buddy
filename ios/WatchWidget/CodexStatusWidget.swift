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
        let nextRefresh = Calendar.current.nextDate(
            after: entry.date,
            matching: DateComponents(hour: 0, minute: 0, second: 0),
            matchingPolicy: .nextTime
        ) ?? entry.date.addingTimeInterval(60 * 60 * 24)
        completion(Timeline(entries: [entry], policy: .after(nextRefresh)))
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
            .frame(width: size.width * 0.9, height: size.height * 0.9)
            .foregroundStyle(.white)
            .widgetAccentable()
            .frame(width: size.width, height: size.height)
            .containerBackground(for: .widget) {
                Color.clear
            }
    }

    private func widgetGlyph(in size: CGSize) -> some View {
        let isWeekendFace = isSaturdayOrSunday(entry.date)
        let iconSize = min(size.width, size.height)
        let stroke = iconSize * 0.038
        let eyeSize = iconSize * 0.125
        let eyeOffsetX = iconSize * 0.16
        let eyeOffsetY = -iconSize * 0.14
        let mouthStroke = max(iconSize * 0.058, 2.2)
        let mouthWidth = iconSize * 0.36
        let mouthHeight = iconSize * 0.15
        let mouthOffsetY = iconSize * 0.09

        return ZStack {
            Circle()
                .stroke(style: StrokeStyle(lineWidth: stroke, lineCap: .round, lineJoin: .round))

            Circle()
                .frame(width: eyeSize, height: eyeSize)
                .offset(x: -eyeOffsetX, y: eyeOffsetY)

            Circle()
                .frame(width: eyeSize, height: eyeSize)
                .offset(x: eyeOffsetX, y: eyeOffsetY)

            if isWeekendFace {
                WeekendSmileMouth()
                    .stroke(style: StrokeStyle(lineWidth: mouthStroke, lineCap: .round, lineJoin: .round))
                    .frame(width: mouthWidth, height: mouthHeight)
                    .offset(y: mouthOffsetY + iconSize * 0.06)
            } else {
                WeekdayMouth()
                    .stroke(style: StrokeStyle(lineWidth: mouthStroke, lineCap: .round, lineJoin: .round))
                    .frame(width: mouthWidth, height: mouthHeight)
                    .offset(y: mouthOffsetY)
            }
        }
        .padding(stroke * 0.55)
    }

    private func isSaturdayOrSunday(_ date: Date) -> Bool {
        let weekday = Calendar.current.component(.weekday, from: date)
        return weekday == 1 || weekday == 7
    }
}

private struct WeekendSmileMouth: Shape {
    func path(in rect: CGRect) -> Path {
        var path = Path()
        path.move(to: CGPoint(x: rect.minX, y: rect.minY + rect.height * 0.25))
        path.addQuadCurve(
            to: CGPoint(x: rect.maxX, y: rect.minY + rect.height * 0.25),
            control: CGPoint(x: rect.midX, y: rect.maxY)
        )
        return path
    }
}

private struct WeekdayMouth: Shape {
    func path(in rect: CGRect) -> Path {
        var path = Path()
        path.move(to: CGPoint(x: rect.minX, y: rect.minY + rect.height * 0.22))
        path.addLine(
            to: CGPoint(x: rect.midX, y: rect.maxY)
        )
        path.addLine(
            to: CGPoint(x: rect.maxX, y: rect.minY + rect.height * 0.22)
        )
        return path
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
