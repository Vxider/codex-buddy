import AppKit

final class AppDelegate: NSObject, NSApplicationDelegate, NSMenuDelegate {
    private let statusItem = NSStatusBar.system.statusItem(withLength: NSStatusItem.variableLength)
    private let api = AgentBuddyAPI()
    private lazy var stream = SSEClient(api: api)
    private let deviceOutputs = DeviceOutputController()
    private var snapshot = CodexStatusSnapshot.offline
    private var serverSnapshots: [String: CodexStatusSnapshot] = [:]
    private var serverErrors: [String: String] = [:]
    private var ledState = LEDState.off
    private var blinkTimer: Timer?
    private var blinkLit = true
    private var lastError: String?
    private var menuRefreshTask: Task<Void, Never>?
    private var settingsWindow: SettingsWindowController?

    func applicationDidFinishLaunching(_ notification: Notification) {
        NSApp.setActivationPolicy(.accessory)
        statusItem.button?.imagePosition = .imageOnly
        statusItem.button?.title = ""
        statusItem.menu = NSMenu()
        statusItem.menu?.delegate = self
        updateStatusIcon()
        deviceOutputs.reloadSettings()
        startStream()
        refreshInitialStatusForDeviceOutputs()
    }

    func applicationWillTerminate(_ notification: Notification) {
        stream.stop()
        deviceOutputs.stop()
        blinkTimer?.invalidate()
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
                self.snapshot = status
                self.lastError = nil
                self.updateStatusIcon()
                self.deviceOutputs.publish(snapshot: status, ledState: self.ledState)
            }
        }
        stream.onDisconnect = { [weak self] error in
            DispatchQueue.main.async {
                guard let self else { return }
                self.lastError = error?.localizedDescription
                self.ledState = .off
                self.updateStatusIcon()
                self.deviceOutputs.publish(snapshot: .offline, ledState: .off)
            }
        }
        stream.start()
    }

    private func restartStream() {
        stream.start()
    }

    private func refreshInitialStatusForDeviceOutputs() {
        Task { [weak self, api] in
            guard let self else { return }
            do {
                let status = try await api.loadStatus()
                await MainActor.run {
                    self.ledState = LEDState.aggregate(snapshot: status)
                    self.snapshot = status
                    self.lastError = nil
                    self.updateStatusIcon()
                    self.deviceOutputs.publish(snapshot: status, ledState: self.ledState)
                }
            } catch {
                await MainActor.run {
                    self.lastError = error.localizedDescription
                    self.deviceOutputs.publish(snapshot: .offline, ledState: .off)
                }
            }
        }
    }

    private func refreshSessionsForMenu(_ menu: NSMenu) {
        menuRefreshTask?.cancel()
        menuRefreshTask = Task { [weak self, weak menu] in
            guard let self else { return }
            let servers = api.servers
            var nextSnapshots: [String: CodexStatusSnapshot] = [:]
            var nextErrors: [String: String] = [:]

            await withTaskGroup(of: (String, Result<CodexStatusSnapshot, Error>).self) { group in
                for server in servers {
                    group.addTask { [api] in
                        do {
                            return (server.id, .success(try await api.loadStatus(for: server)))
                        } catch {
                            return (server.id, .failure(error))
                        }
                    }
                }

                for await (serverID, result) in group {
                    switch result {
                    case let .success(status):
                        nextSnapshots[serverID] = status
                    case let .failure(error):
                        nextErrors[serverID] = error.localizedDescription
                    }
                }
            }

            await MainActor.run {
                self.serverSnapshots = nextSnapshots
                self.serverErrors = nextErrors
                self.lastError = nextErrors.isEmpty ? nil : "\(nextErrors.count) server\(nextErrors.count == 1 ? "" : "s") offline"
                self.ledState = self.aggregateLEDState(from: nextSnapshots.values)
                self.updateStatusIcon()
                if let menu {
                    self.rebuildMenu(menu, loading: false)
                }
            }
        }
    }

    private func updateStatusIcon() {
        updateBlinkTimer()
        statusItem.button?.image = MenuBarIcon.image(state: ledState, isLit: ledState == .off || blinkLit)
        statusItem.button?.title = ""
        statusItem.button?.toolTip = "Agent Buddy: \(ledState.label)"
    }

    private func updateBlinkTimer() {
        if ledState == .off {
            blinkTimer?.invalidate()
            blinkTimer = nil
            blinkLit = true
            return
        }
        guard blinkTimer == nil else { return }
        blinkTimer = Timer(timeInterval: 0.7, repeats: true) { [weak self] _ in
            guard let self else { return }
            self.blinkLit.toggle()
            self.statusItem.button?.image = MenuBarIcon.image(state: self.ledState, isLit: self.blinkLit)
        }
        RunLoop.main.add(blinkTimer!, forMode: .common)
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
        } else if visibleSessions().isEmpty {
            let item = NSMenuItem(title: "No active sessions", action: nil, keyEquivalent: "")
            item.isEnabled = false
            menu.addItem(item)
        } else {
            for entry in visibleSessionEntries() {
                menu.addItem(sessionItem(entry.session, server: entry.server))
            }
        }

        menu.addItem(.separator())
        menu.addItem(actionItem(title: "Open Codex-Buddy", action: #selector(openAgentBuddy(_:)), keyEquivalent: ""))
        menu.addItem(actionItem(title: "Dance", action: #selector(danceAction(_:)), keyEquivalent: "d"))
        menu.addItem(actionItem(title: "Refresh Sessions", action: #selector(refreshMenuAction(_:)), keyEquivalent: "r"))
        menu.addItem(actionItem(title: "Quit", action: #selector(quit(_:)), keyEquivalent: "q"))
    }

    private func headerItem(loading: Bool) -> NSMenuItem {
        let count = visibleSessions().count
        let suffix = loading ? "Loading" : "\(count) session\(count == 1 ? "" : "s")"
        let item = NSMenuItem(title: "Agent Buddy  \(ledState.label)  \(suffix)", action: nil, keyEquivalent: "")
        item.isEnabled = false
        return item
    }

    private func sessionItem(_ session: CodexSessionSummary) -> NSMenuItem {
        let item = NSMenuItem(title: truncate(session.title, limit: 34), action: nil, keyEquivalent: "")
        item.image = MenuBarIcon.menuDot(state: ledState(for: session))
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

    private func sessionItem(_ session: CodexSessionSummary, server: CodexServerEndpoint) -> NSMenuItem {
        let item = sessionItem(session)
        item.title = "\(truncate(session.title, limit: 28))  ·  \(server.name)"
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
        item.image = nil
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

    private func visibleSessionEntries() -> [(server: CodexServerEndpoint, session: CodexSessionSummary)] {
        let entries = api.servers
            .flatMap { server in
                (serverSnapshots[server.id]?.sessions ?? []).map { session in
                    (server: server, session: session)
                }
            }
            .sorted {
                ($0.session.updatedAt ?? .distantPast) > ($1.session.updatedAt ?? .distantPast)
            }
        var seen = Set<String>()
        return entries.filter { entry in
            seen.insert(entry.session.id).inserted
        }
    }

    private func visibleSessions() -> [CodexSessionSummary] {
        visibleSessionEntries().map(\.session)
    }

    private func aggregateLEDState(from snapshots: Dictionary<String, CodexStatusSnapshot>.Values) -> LEDState {
        snapshots.reduce(.off) { current, snapshot in
            strongerLEDState(current, LEDState.aggregate(snapshot: snapshot))
        }
    }

    private func strongerLEDState(_ current: LEDState, _ candidate: LEDState) -> LEDState {
        rank(candidate) > rank(current) ? candidate : current
    }

    private func rank(_ state: LEDState) -> Int {
        switch state {
        case .goal:
            return 40
        case .approval:
            return 30
        case .attention:
            return 20
        case .working:
            return 10
        case .off:
            return 0
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

    private func ledState(for session: CodexSessionSummary) -> LEDState {
        if session.goalState == "achieved" || session.goalState == "complete" || session.goalState == "completed" {
            return .goal
        }
        let detail = (session.stateDetail ?? "").lowercased()
        let reason = (session.openReason ?? "").lowercased()
        if session.needsApproval || reason == "approval" || detail.contains("permissionrequest") || detail.contains("permission request") {
            return .approval
        }
        if session.needsOpen || session.state.normalized == .open || session.state.normalized == .error {
            return .attention
        }
        if session.state.normalized == .run || session.state.normalized == .runningBash {
            return .working
        }
        return .off
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

    @objc func refreshMenuAction(_ sender: Any? = nil) {
        if let menu = statusItem.menu {
            rebuildMenu(menu, loading: true)
            refreshSessionsForMenu(menu)
        }
    }

    @objc func danceAction(_ sender: Any? = nil) {
        deviceOutputs.sendDanceCommand()
    }

    @objc func openAgentBuddy(_ sender: Any? = nil) {
        statusItem.menu?.cancelTracking()
        DispatchQueue.main.async { [weak self] in
            self?.presentAgentBuddyWindow()
        }
    }

    private func presentAgentBuddyWindow() {
        NSApp.setActivationPolicy(.regular)
        if settingsWindow == nil {
            let controller = SettingsWindowController(api: api)
            controller.onSave = { [weak self] in
                self?.resetConnectionState()
                self?.deviceOutputs.reloadSettings()
                self?.restartStream()
                self?.refreshInitialStatusForDeviceOutputs()
                self?.updateStatusIcon()
            }
            controller.onClose = { [weak self] in
                NSApp.setActivationPolicy(.accessory)
            }
            settingsWindow = controller
        }
        guard let window = settingsWindow?.window else { return }
        window.collectionBehavior = [.moveToActiveSpace, .fullScreenAuxiliary]
        if !window.isVisible {
            window.center()
        }
        window.level = .floating
        window.isReleasedWhenClosed = false
        settingsWindow?.showWindow(self)
        window.setIsVisible(true)
        window.makeKeyAndOrderFront(self)
        window.orderFrontRegardless()
        NSApp.activate(ignoringOtherApps: true)
        NSRunningApplication.current.activate(options: [.activateAllWindows, .activateIgnoringOtherApps])
        settingsWindow?.refresh()
    }

    @objc private func addServer() {
        let alert = NSAlert()
        alert.messageText = "Add Agent Buddy Server"
        alert.informativeText = "Enter a server URL, for example http://192.168.1.10:8787"
        alert.addButton(withTitle: "Add")
        alert.addButton(withTitle: "Cancel")

        let field = NSTextField(frame: NSRect(x: 0, y: 0, width: 360, height: 24))
        field.stringValue = AgentBuddyAPI.defaultBaseURL
        alert.accessoryView = field

        NSApp.activate(ignoringOtherApps: true)
        guard alert.runModal() == .alertFirstButtonReturn else { return }
        guard api.addServer(url: field.stringValue) != nil else {
            showError("Invalid server URL.")
            return
        }
        resetConnectionState()
        restartStream()
        updateStatusIcon()
    }

    @objc private func removeCurrentServer() {
        api.removeServer(id: api.activeServer.id)
        resetConnectionState()
        restartStream()
        updateStatusIcon()
    }

    private func resetConnectionState() {
        snapshot = .offline
        serverSnapshots = [:]
        serverErrors = [:]
        ledState = .off
        lastError = nil
    }

    private func showError(_ message: String) {
        let alert = NSAlert()
        alert.messageText = "Agent Buddy"
        alert.informativeText = message
        alert.addButton(withTitle: "OK")
        NSApp.activate(ignoringOtherApps: true)
        alert.runModal()
    }

    @objc private func togglePlaceholder(_ sender: NSMenuItem) {
        guard let key = sender.representedObject as? String else { return }
        let next = sender.state != .on
        UserDefaults.standard.set(next, forKey: key)
        sender.state = next ? .on : .off
    }

    @objc func quit(_ sender: Any? = nil) {
        NSApp.terminate(nil)
    }
}
