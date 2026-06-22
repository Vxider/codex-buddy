import AppKit

enum MenuBarIcon {
    static func image(state: LEDState, isLit: Bool = true) -> NSImage {
        let color = isLit ? state.color : state.color.withAlphaComponent(0.22)
        return dotImage(color: color, size: NSSize(width: 18, height: 18), dotRect: NSRect(x: 3, y: 3, width: 12, height: 12))
    }

    static func menuDot(state: LEDState) -> NSImage {
        dotImage(color: state.color, size: NSSize(width: 14, height: 14), dotRect: NSRect(x: 3, y: 3, width: 8, height: 8))
    }

    private static func dotImage(color: NSColor, size: NSSize, dotRect: NSRect) -> NSImage {
        let image = NSImage(size: size)
        image.lockFocus()

        let path = NSBezierPath(ovalIn: dotRect)
        let shadow = NSShadow()
        shadow.shadowBlurRadius = 2
        shadow.shadowOffset = NSSize(width: 0, height: -0.5)
        shadow.shadowColor = NSColor.black.withAlphaComponent(0.28)
        shadow.set()

        color.setFill()
        path.fill()

        image.unlockFocus()
        image.isTemplate = false
        return image
    }
}
