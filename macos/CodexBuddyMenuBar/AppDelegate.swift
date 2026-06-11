import AppKit

@main
final class AppDelegate: NSObject, NSApplicationDelegate, NSMenuDelegate {
    private let statusItem = NSStatusBar.system.statusItem(withLength: NSStatusItem.squareLength)
    private let api = CodexBuddyAPI()
    private lazy var stream = SSEClient(api: api)
    private var snapshot = CodexStatusSnapshot.offline
    private var ledState = LEDState.off
    private var lastError: String?
    private var menuRefreshTask: Task<Void, Never>?
    private var settingsWindow: SettingsWindowController?

    func applicationDidFinishLaunching(_ notification: Notification) {
        NSApp.setActivationPolicy(.accessory)
        statusItem.button?.imagePosition = .imageOnly
        statusItem.menu = NSMenu()
        statusItem.menu?.delegate = self
        updateStatusIcon()
        startStream()
    }

    func applicationWillTerminate(_ notification: Notification) {
        stream.stop()
        menuRefreshTask?.cancel()
    }

    func menuWillOpen(_ menu: NSMenu) {
        rebuildMenu(menu, loading: true)
        refreshSessionsForMenu(menu)
    }

    private func startStream() {
        stream.onStatus = { [weak self] status in
            DispatchQueue.main.async {
                guard let self else { return }
                self.ledState = LEDState.aggregate(snapshot: status)
                self.lastError = nil
                self.updateStatusIcon()
            }
        }
        stream.onDisconnect = { [weak self] error in
            DispatchQueue.main.async {
                guard let self else { return }
                self.lastError = error?.localizedDescription
                self.ledState = .off
                self.updateStatusIcon()
            }
        }
        stream.start()
    }

    private func restartStream() {
        stream.start()
    }

    private func refreshSessionsForMenu(_ menu: NSMenu) {
        menuRefreshTask?.cancel()
        menuRefreshTask = Task { [weak self, weak menu] in
            guard let self else { return }
            do {
                let status = try await api.loadStatus()
                await MainActor.run {
                    self.snapshot = status
                    self.lastError = nil
                    if let menu {
                        self.rebuildMenu(menu, loading: false)
                    }
                }
            } catch {
                await MainActor.run {
                    self.snapshot = .offline
                    self.lastError = error.localizedDescription
                    if let menu {
                        self.rebuildMenu(menu, loading: false)
                    }
                }
            }
        }
    }

    private func updateStatusIcon() {
        statusItem.button?.image = MenuBarIcon.image(state: ledState)
        statusItem.button?.toolTip = "Codex Buddy: \(ledState.label)"
    }

    private func rebuildMenu(_ menu: NSMenu, loading: Bool) {
        menu.removeAllItems()
        menu.addItem(headerItem(loading: loading))
        if let lastError {
            let item = NSMenuItem(title: truncate("Connection: \(lastError)", limit: 96), action: nil, keyEquivalent: "")
            item.isEnabled = false
            menu.addItem(item)
        }
        menu.addItem(.separator())

        if loading {
            let item = NSMenuItem(title: "Loading sessions...", action: nil, keyEquivalent: "")
            item.isEnabled = false
            menu.addItem(item)
        } else if snapshot.sessions.isEmpty {
            let item = NSMenuItem(title: "No active sessions", action: nil, keyEquivalent: "")
            item.isEnabled = false
            menu.addItem(item)
        } else {
            for session in orderedSessions(snapshot.sessions) {
                menu.addItem(sessionItem(session))
            }
        }

        menu.addItem(.separator())
        menu.addItem(actionItem(title: "Refresh Sessions", action: #selector(refreshMenuAction), keyEquivalent: "r"))
        menu.addItem(settingsItem())
        menu.addItem(.separator())
        menu.addItem(actionItem(title: "Quit Codex Buddy", action: #selector(quit), keyEquivalent: "q"))
    }

    private func headerItem(loading: Bool) -> NSMenuItem {
        let suffix = loading ? "Loading" : "\(snapshot.sessionsCount) session\(snapshot.sessionsCount == 1 ? "" : "s")"
        let item = NSMenuItem(title: "Codex Buddy  \(ledState.label)  \(suffix)", action: nil, keyEquivalent: "")
        item.isEnabled = false
        return item
    }

    private func sessionItem(_ session: CodexSessionSummary) -> NSMenuItem {
        let item = NSMenuItem(title: "\(stateGlyph(session)) \(truncate(session.title, limit: 34))", action: nil, keyEquivalent: "")
        let submenu = NSMenu()
        let subtitle = NSMenuItem(title: "\(session.state.label)  \(session.subtitle)", action: nil, keyEquivalent: "")
        subtitle.isEnabled = false
        submenu.addItem(subtitle)

        let message = truncate(session.message, limit: 140)
        if !message.isEmpty {
            let messageItem = NSMenuItem(title: message, action: nil, keyEquivalent: "")
            messageItem.isEnabled = false
            submenu.addItem(messageItem)
        }

        let updated = NSMenuItem(title: "Updated \(relativeTime(session.updatedAt))", action: nil, keyEquivalent: "")
        updated.isEnabled = false
        submenu.addItem(updated)
        item.submenu = submenu
        return item
    }

    private func settingsItem() -> NSMenuItem {
        let item = NSMenuItem(title: "Settings", action: nil, keyEquivalent: "")
        let submenu = NSMenu()
        submenu.addItem(actionItem(title: "Open Settings...", action: #selector(openSettings), keyEquivalent: ","))
        submenu.addItem(.separator())
        submenu.addItem(toggleItem(title: "BLE LED", key: "codexBuddyBLELEDEnabled"))
        submenu.addItem(toggleItem(title: "USB LED", key: "codexBuddyUSBLEDEnabled"))
        item.submenu = submenu
        return item
    }

    private func toggleItem(title: String, key: String) -> NSMenuItem {
        let item = NSMenuItem(title: title, action: #selector(togglePlaceholder(_:)), keyEquivalent: "")
        item.target = self
        item.state = UserDefaults.standard.bool(forKey: key) ? .on : .off
        item.representedObject = key
        return item
    }

    private func actionItem(title: String, action: Selector, keyEquivalent: String) -> NSMenuItem {
        let item = NSMenuItem(title: title, action: action, keyEquivalent: keyEquivalent)
        item.target = self
        return item
    }

    private func orderedSessions(_ sessions: [CodexSessionSummary]) -> [CodexSessionSummary] {
        sessions.sorted { left, right in
            let leftOpen = left.needsOpen || left.needsApproval
            let rightOpen = right.needsOpen || right.needsApproval
            if leftOpen != rightOpen {
                return leftOpen
            }
            return (left.updatedAt ?? .distantPast) > (right.updatedAt ?? .distantPast)
        }
    }

    private func stateGlyph(_ session: CodexSessionSummary) -> String {
        if session.needsApproval || session.needsOpen {
            return "●"
        }
        switch session.state.normalized {
        case .run, .runningBash, .error:
            return "●"
        default:
            return "○"
        }
    }

    private func truncate(_ value: String, limit: Int) -> String {
        guard value.count > limit else { return value }
        let index = value.index(value.startIndex, offsetBy: max(0, limit - 3))
        return String(value[..<index]) + "..."
    }

    private func relativeTime(_ date: Date?) -> String {
        guard let date else { return "-" }
        let seconds = max(0, Int(Date().timeIntervalSince(date)))
        if seconds < 60 {
            return "\(seconds)s ago"
        }
        let minutes = seconds / 60
        if minutes < 60 {
            return "\(minutes)m ago"
        }
        return "\(minutes / 60)h ago"
    }

    @objc private func refreshMenuAction() {
        if let menu = statusItem.menu {
            rebuildMenu(menu, loading: true)
            refreshSessionsForMenu(menu)
        }
    }

    @objc private func openSettings() {
        if settingsWindow == nil {
            let controller = SettingsWindowController(api: api)
            controller.onSave = { [weak self] in
                self?.restartStream()
            }
            settingsWindow = controller
        }
        settingsWindow?.showWindow(nil)
        NSApp.activate(ignoringOtherApps: true)
    }

    @objc private func togglePlaceholder(_ sender: NSMenuItem) {
        guard let key = sender.representedObject as? String else { return }
        let next = sender.state != .on
        UserDefaults.standard.set(next, forKey: key)
        sender.state = next ? .on : .off
    }

    @objc private func quit() {
        NSApp.terminate(nil)
    }
}
