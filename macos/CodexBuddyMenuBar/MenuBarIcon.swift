import AppKit

enum MenuBarIcon {
    static func image(state: LEDState) -> NSImage {
        let size = NSSize(width: 18, height: 18)
        let image = NSImage(size: size)
        image.lockFocus()

        let path = NSBezierPath(ovalIn: NSRect(x: 3, y: 3, width: 12, height: 12))
        let shadow = NSShadow()
        shadow.shadowBlurRadius = 2
        shadow.shadowOffset = NSSize(width: 0, height: -0.5)
        shadow.shadowColor = NSColor.black.withAlphaComponent(0.28)
        shadow.set()

        state.color.setFill()
        path.fill()
        NSColor.white.withAlphaComponent(0.82).setStroke()
        path.lineWidth = 1.2
        path.stroke()

        image.unlockFocus()
        image.isTemplate = false
        return image
    }
}
