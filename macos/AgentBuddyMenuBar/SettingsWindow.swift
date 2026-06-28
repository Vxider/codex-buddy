import AppKit

final class SettingsWindowController: NSWindowController, NSTableViewDataSource, NSTableViewDelegate, NSWindowDelegate {
    private let api: AgentBuddyAPI
    private var servers: [CodexServerEndpoint]
    private var selectedServerID: String
    private let serverTable = NSTableView()
    private let sessionTable = NSTableView()
    private let nameField = NSTextField()
    private let urlField = NSTextField()
    private let detailTitle = NSTextField(labelWithString: "")
    private let detailSubtitle = NSTextField(labelWithString: "")
    private let bleButton = NSButton(checkboxWithTitle: "BLE Broadcast", target: nil, action: nil)
    private let usbButton = NSButton(checkboxWithTitle: "USB Serial", target: nil, action: nil)
    private var serverLEDStates: [String: LEDState] = [:]
    private var sessionRows: [CodexSessionSummary] = []
    private var sessionPlaceholder = "Loading sessions..."
    private var draftServerID: String?
    private var isApplyingSelection = false
    var onSave: (() -> Void)?
    var onClose: (() -> Void)?

    init(api: AgentBuddyAPI) {
        self.api = api
        self.servers = api.servers
        self.selectedServerID = api.servers.first?.id ?? api.activeServer.id
        let window = NSWindow(
            contentRect: NSRect(x: 0, y: 0, width: 980, height: 560),
            styleMask: [.titled, .closable],
            backing: .buffered,
            defer: false
        )
        window.title = "Codex-Buddy"
        window.center()
        window.isReleasedWhenClosed = false
        super.init(window: window)
        window.delegate = self
        buildUI()
        reloadSelection()
    }

    required init?(coder: NSCoder) {
        nil
    }

    func refresh() {
        reloadSelection()
    }

    private func buildUI() {
        guard let contentView = window?.contentView else { return }
        contentView.wantsLayer = true
        contentView.layer?.backgroundColor = NSColor.windowBackgroundColor.cgColor

        let sidebar = NSView()
        sidebar.wantsLayer = true
        sidebar.layer?.backgroundColor = NSColor.controlBackgroundColor.cgColor
        sidebar.translatesAutoresizingMaskIntoConstraints = false

        let middle = NSView()
        middle.translatesAutoresizingMaskIntoConstraints = false

        let detail = NSView()
        detail.translatesAutoresizingMaskIntoConstraints = false

        let divider1 = NSBox()
        divider1.boxType = .separator
        divider1.translatesAutoresizingMaskIntoConstraints = false
        let divider2 = NSBox()
        divider2.boxType = .separator
        divider2.translatesAutoresizingMaskIntoConstraints = false

        for view in [sidebar, divider1, middle, divider2, detail] {
            contentView.addSubview(view)
        }

        NSLayoutConstraint.activate([
            sidebar.leadingAnchor.constraint(equalTo: contentView.leadingAnchor),
            sidebar.topAnchor.constraint(equalTo: contentView.topAnchor),
            sidebar.bottomAnchor.constraint(equalTo: contentView.bottomAnchor),
            sidebar.widthAnchor.constraint(equalToConstant: 250),

            divider1.leadingAnchor.constraint(equalTo: sidebar.trailingAnchor),
            divider1.topAnchor.constraint(equalTo: contentView.topAnchor),
            divider1.bottomAnchor.constraint(equalTo: contentView.bottomAnchor),
            divider1.widthAnchor.constraint(equalToConstant: 1),

            middle.leadingAnchor.constraint(equalTo: divider1.trailingAnchor),
            middle.topAnchor.constraint(equalTo: contentView.topAnchor),
            middle.bottomAnchor.constraint(equalTo: contentView.bottomAnchor),
            middle.widthAnchor.constraint(equalToConstant: 330),

            divider2.leadingAnchor.constraint(equalTo: middle.trailingAnchor),
            divider2.topAnchor.constraint(equalTo: contentView.topAnchor),
            divider2.bottomAnchor.constraint(equalTo: contentView.bottomAnchor),
            divider2.widthAnchor.constraint(equalToConstant: 1),

            detail.leadingAnchor.constraint(equalTo: divider2.trailingAnchor),
            detail.trailingAnchor.constraint(equalTo: contentView.trailingAnchor),
            detail.topAnchor.constraint(equalTo: contentView.topAnchor),
            detail.bottomAnchor.constraint(equalTo: contentView.bottomAnchor)
        ])

        buildSidebar(sidebar)
        buildServerList(middle)
        buildSessionDetail(detail)
    }

    private func buildSidebar(_ container: NSView) {
        let title = NSTextField(labelWithString: "Codex-Buddy")
        title.font = .boldSystemFont(ofSize: 20)
        title.translatesAutoresizingMaskIntoConstraints = false

        let subtitle = NSTextField(labelWithString: "Aggregating \(servers.count) server\(servers.count == 1 ? "" : "s")")
        subtitle.textColor = .secondaryLabelColor
        subtitle.translatesAutoresizingMaskIntoConstraints = false

        let nav = NSTextField(labelWithString: "▣ Servers")
        nav.font = .boldSystemFont(ofSize: 15)
        nav.translatesAutoresizingMaskIntoConstraints = false

        let globalLabel = NSTextField(labelWithString: "ESP32 device outputs")
        globalLabel.font = .boldSystemFont(ofSize: 12)
        globalLabel.translatesAutoresizingMaskIntoConstraints = false

        bleButton.state = UserDefaults.standard.bool(forKey: DeviceOutputController.bleEnabledKey) ? .on : .off
        usbButton.state = UserDefaults.standard.bool(forKey: DeviceOutputController.usbEnabledKey) ? .on : .off
        bleButton.target = self
        bleButton.action = #selector(deviceOutputToggled)
        usbButton.target = self
        usbButton.action = #selector(deviceOutputToggled)
        let switches = NSStackView(views: [bleButton, usbButton])
        switches.orientation = .vertical
        switches.alignment = .leading
        switches.spacing = 8
        switches.translatesAutoresizingMaskIntoConstraints = false

        for view in [title, subtitle, nav, globalLabel, switches] {
            container.addSubview(view)
        }

        NSLayoutConstraint.activate([
            title.leadingAnchor.constraint(equalTo: container.leadingAnchor, constant: 24),
            title.topAnchor.constraint(equalTo: container.topAnchor, constant: 28),
            subtitle.leadingAnchor.constraint(equalTo: title.leadingAnchor),
            subtitle.topAnchor.constraint(equalTo: title.bottomAnchor, constant: 4),
            nav.leadingAnchor.constraint(equalTo: title.leadingAnchor),
            nav.topAnchor.constraint(equalTo: subtitle.bottomAnchor, constant: 34),
            globalLabel.leadingAnchor.constraint(equalTo: title.leadingAnchor),
            globalLabel.topAnchor.constraint(equalTo: nav.bottomAnchor, constant: 48),
            switches.leadingAnchor.constraint(equalTo: title.leadingAnchor),
            switches.topAnchor.constraint(equalTo: globalLabel.bottomAnchor, constant: 10)
        ])
    }

    private func buildServerList(_ container: NSView) {
        let title = NSTextField(labelWithString: "Servers")
        title.font = .boldSystemFont(ofSize: 24)
        title.translatesAutoresizingMaskIntoConstraints = false

        serverTable.addTableColumn(NSTableColumn(identifier: NSUserInterfaceItemIdentifier("server")))
        serverTable.headerView = nil
        serverTable.delegate = self
        serverTable.dataSource = self
        serverTable.rowHeight = 48
        serverTable.intercellSpacing = NSSize(width: 0, height: 4)
        serverTable.selectionHighlightStyle = .regular

        let scrollView = NSScrollView()
        scrollView.documentView = serverTable
        scrollView.hasVerticalScroller = true
        scrollView.drawsBackground = false
        scrollView.borderType = .noBorder
        scrollView.translatesAutoresizingMaskIntoConstraints = false

        let nameLabel = NSTextField(labelWithString: "Name")
        let urlLabel = NSTextField(labelWithString: "Server URL")
        for view in [nameLabel, urlLabel, nameField, urlField] {
            view.translatesAutoresizingMaskIntoConstraints = false
        }
        urlField.placeholderString = AgentBuddyAPI.defaultBaseURL

        let addButton = NSButton(title: "Add", target: self, action: #selector(addServer))
        let removeButton = NSButton(title: "Remove", target: self, action: #selector(removeServer))
        let saveButton = NSButton(title: "Save", target: self, action: #selector(saveSettings))
        for button in [addButton, removeButton, saveButton] {
            button.bezelStyle = .rounded
        }

        let buttonRow = NSStackView(views: [addButton, removeButton, saveButton])
        buttonRow.orientation = .horizontal
        buttonRow.spacing = 8
        buttonRow.translatesAutoresizingMaskIntoConstraints = false

        for view in [title, scrollView, nameLabel, nameField, urlLabel, urlField, buttonRow] {
            container.addSubview(view)
        }

        NSLayoutConstraint.activate([
            title.leadingAnchor.constraint(equalTo: container.leadingAnchor, constant: 22),
            title.topAnchor.constraint(equalTo: container.topAnchor, constant: 28),

            scrollView.leadingAnchor.constraint(equalTo: title.leadingAnchor),
            scrollView.trailingAnchor.constraint(equalTo: container.trailingAnchor, constant: -14),
            scrollView.topAnchor.constraint(equalTo: title.bottomAnchor, constant: 18),
            scrollView.heightAnchor.constraint(equalToConstant: 130),

            nameLabel.leadingAnchor.constraint(equalTo: title.leadingAnchor),
            nameLabel.topAnchor.constraint(equalTo: scrollView.bottomAnchor, constant: 18),
            nameField.leadingAnchor.constraint(equalTo: title.leadingAnchor),
            nameField.trailingAnchor.constraint(equalTo: scrollView.trailingAnchor),
            nameField.topAnchor.constraint(equalTo: nameLabel.bottomAnchor, constant: 6),

            urlLabel.leadingAnchor.constraint(equalTo: title.leadingAnchor),
            urlLabel.topAnchor.constraint(equalTo: nameField.bottomAnchor, constant: 14),
            urlField.leadingAnchor.constraint(equalTo: title.leadingAnchor),
            urlField.trailingAnchor.constraint(equalTo: scrollView.trailingAnchor),
            urlField.topAnchor.constraint(equalTo: urlLabel.bottomAnchor, constant: 6),

            buttonRow.leadingAnchor.constraint(equalTo: title.leadingAnchor),
            buttonRow.topAnchor.constraint(equalTo: urlField.bottomAnchor, constant: 16)
        ])
    }

    private func buildSessionDetail(_ container: NSView) {
        detailTitle.font = .boldSystemFont(ofSize: 24)
        detailTitle.translatesAutoresizingMaskIntoConstraints = false
        detailSubtitle.textColor = .secondaryLabelColor
        detailSubtitle.translatesAutoresizingMaskIntoConstraints = false

        sessionTable.addTableColumn(NSTableColumn(identifier: NSUserInterfaceItemIdentifier("session")))
        sessionTable.headerView = nil
        sessionTable.delegate = self
        sessionTable.dataSource = self
        sessionTable.rowHeight = 72
        sessionTable.intercellSpacing = NSSize(width: 0, height: 0)
        sessionTable.selectionHighlightStyle = .regular

        let scrollView = NSScrollView()
        scrollView.documentView = sessionTable
        scrollView.hasVerticalScroller = true
        scrollView.drawsBackground = false
        scrollView.borderType = .noBorder
        scrollView.translatesAutoresizingMaskIntoConstraints = false

        for view in [detailTitle, detailSubtitle, scrollView] {
            container.addSubview(view)
        }

        NSLayoutConstraint.activate([
            detailTitle.leadingAnchor.constraint(equalTo: container.leadingAnchor, constant: 24),
            detailTitle.trailingAnchor.constraint(equalTo: container.trailingAnchor, constant: -24),
            detailTitle.topAnchor.constraint(equalTo: container.topAnchor, constant: 34),
            detailSubtitle.leadingAnchor.constraint(equalTo: detailTitle.leadingAnchor),
            detailSubtitle.trailingAnchor.constraint(equalTo: detailTitle.trailingAnchor),
            detailSubtitle.topAnchor.constraint(equalTo: detailTitle.bottomAnchor, constant: 6),

            scrollView.leadingAnchor.constraint(equalTo: detailTitle.leadingAnchor),
            scrollView.trailingAnchor.constraint(equalTo: detailTitle.trailingAnchor),
            scrollView.topAnchor.constraint(equalTo: detailSubtitle.bottomAnchor, constant: 18),
            scrollView.bottomAnchor.constraint(equalTo: container.bottomAnchor, constant: -24)
        ])
    }

    func numberOfRows(in tableView: NSTableView) -> Int {
        if tableView == serverTable {
            return servers.count
        }
        return sessionRows.isEmpty ? 1 : sessionRows.count
    }

    func tableView(_ tableView: NSTableView, viewFor tableColumn: NSTableColumn?, row: Int) -> NSView? {
        if tableView == sessionTable {
            return sessionCell(row: row)
        }
        return serverCell(row: row)
    }

    private func serverCell(row: Int) -> NSView? {
        let identifier = NSUserInterfaceItemIdentifier("serverCell")
        let cell = serverTable.makeView(withIdentifier: identifier, owner: self) as? NSTableCellView ?? NSTableCellView()
        if cell.textField == nil {
            let textField = NSTextField(labelWithString: "")
            textField.translatesAutoresizingMaskIntoConstraints = false
            textField.maximumNumberOfLines = 2
            textField.lineBreakMode = .byTruncatingTail
            let imageView = NSImageView()
            imageView.translatesAutoresizingMaskIntoConstraints = false
            cell.imageView = imageView
            cell.addSubview(imageView)
            cell.addSubview(textField)
            cell.textField = textField
            cell.identifier = identifier
            NSLayoutConstraint.activate([
                imageView.leadingAnchor.constraint(equalTo: cell.leadingAnchor, constant: 8),
                imageView.centerYAnchor.constraint(equalTo: cell.centerYAnchor),
                imageView.widthAnchor.constraint(equalToConstant: 14),
                imageView.heightAnchor.constraint(equalToConstant: 14),
                textField.leadingAnchor.constraint(equalTo: imageView.trailingAnchor, constant: 8),
                textField.trailingAnchor.constraint(equalTo: cell.trailingAnchor, constant: -10),
                textField.centerYAnchor.constraint(equalTo: cell.centerYAnchor)
            ])
        }
        let server = servers[row]
        cell.textField?.stringValue = "\(server.name)\n\(server.url)"
        cell.imageView?.image = MenuBarIcon.menuDot(state: serverLEDStates[server.id] ?? .off)
        return cell
    }

    private func sessionCell(row: Int) -> NSView? {
        if sessionRows.isEmpty {
            return placeholderSessionCell(sessionPlaceholder)
        }

        let identifier = NSUserInterfaceItemIdentifier("sessionCell")
        let cell = sessionTable.makeView(withIdentifier: identifier, owner: self) as? NSTableCellView ?? NSTableCellView()
        let titleTag = 101
        let subtitleTag = 102
        let messageTag = 103
        let dotTag = 104
        let dot: NSImageView
        let title: NSTextField
        let subtitle: NSTextField
        let message: NSTextField

        if let existingTitle = cell.viewWithTag(titleTag) as? NSTextField,
           let existingSubtitle = cell.viewWithTag(subtitleTag) as? NSTextField,
           let existingMessage = cell.viewWithTag(messageTag) as? NSTextField,
           let existingDot = cell.viewWithTag(dotTag) as? NSImageView {
            dot = existingDot
            title = existingTitle
            subtitle = existingSubtitle
            message = existingMessage
        } else {
            cell.identifier = identifier
            dot = NSImageView()
            dot.tag = dotTag
            title = NSTextField(labelWithString: "")
            title.tag = titleTag
            title.font = .boldSystemFont(ofSize: 13)
            subtitle = NSTextField(labelWithString: "")
            subtitle.tag = subtitleTag
            subtitle.textColor = .secondaryLabelColor
            subtitle.font = .systemFont(ofSize: 12)
            message = NSTextField(labelWithString: "")
            message.tag = messageTag
            message.textColor = .secondaryLabelColor
            message.font = .systemFont(ofSize: 12)
            message.maximumNumberOfLines = 1
            message.lineBreakMode = .byTruncatingTail
            dot.translatesAutoresizingMaskIntoConstraints = false
            cell.addSubview(dot)
            for view in [title, subtitle, message] {
                view.translatesAutoresizingMaskIntoConstraints = false
                cell.addSubview(view)
            }
            NSLayoutConstraint.activate([
                dot.leadingAnchor.constraint(equalTo: cell.leadingAnchor, constant: 8),
                dot.topAnchor.constraint(equalTo: cell.topAnchor, constant: 12),
                dot.widthAnchor.constraint(equalToConstant: 14),
                dot.heightAnchor.constraint(equalToConstant: 14),
                title.leadingAnchor.constraint(equalTo: dot.trailingAnchor, constant: 8),
                title.trailingAnchor.constraint(equalTo: cell.trailingAnchor, constant: -8),
                title.topAnchor.constraint(equalTo: cell.topAnchor, constant: 8),
                subtitle.leadingAnchor.constraint(equalTo: title.leadingAnchor),
                subtitle.trailingAnchor.constraint(equalTo: title.trailingAnchor),
                subtitle.topAnchor.constraint(equalTo: title.bottomAnchor, constant: 4),
                message.leadingAnchor.constraint(equalTo: title.leadingAnchor),
                message.trailingAnchor.constraint(equalTo: title.trailingAnchor),
                message.topAnchor.constraint(equalTo: subtitle.bottomAnchor, constant: 4)
            ])
        }

        let session = sessionRows[row]
        dot.image = MenuBarIcon.menuDot(state: LEDState.forSession(session))
        title.stringValue = session.title
        subtitle.stringValue = "\(session.state.label) · \(session.subtitle)"
        message.stringValue = session.message.isEmpty ? "No recent message" : session.message
        return cell
    }

    private func placeholderSessionCell(_ text: String) -> NSView? {
        let identifier = NSUserInterfaceItemIdentifier("placeholderSessionCell")
        let cell = sessionTable.makeView(withIdentifier: identifier, owner: self) as? NSTableCellView ?? NSTableCellView()
        if cell.textField == nil {
            let label = NSTextField(labelWithString: "")
            label.textColor = .secondaryLabelColor
            label.translatesAutoresizingMaskIntoConstraints = false
            cell.addSubview(label)
            cell.textField = label
            cell.identifier = identifier
            NSLayoutConstraint.activate([
                label.leadingAnchor.constraint(equalTo: cell.leadingAnchor, constant: 30),
                label.trailingAnchor.constraint(equalTo: cell.trailingAnchor, constant: -8),
                label.centerYAnchor.constraint(equalTo: cell.centerYAnchor)
            ])
        }
        cell.textField?.stringValue = text
        return cell
    }

    func tableViewSelectionDidChange(_ notification: Notification) {
        guard notification.object as? NSTableView == serverTable else { return }
        guard !isApplyingSelection else { return }
        let row = serverTable.selectedRow
        guard row >= 0, row < servers.count else { return }
        selectedServerID = servers[row].id
        draftServerID = nil
        nameField.stringValue = servers[row].name
        urlField.stringValue = servers[row].url
        refreshSessions()
    }

    private func reloadSelection() {
        servers = api.servers
        let selectedIndex = servers.firstIndex { $0.id == selectedServerID } ?? 0
        selectedServerID = servers[selectedIndex].id
        isApplyingSelection = true
        serverTable.reloadData()
        serverTable.selectRowIndexes(IndexSet(integer: selectedIndex), byExtendingSelection: false)
        isApplyingSelection = false
        nameField.stringValue = servers[selectedIndex].name
        urlField.stringValue = servers[selectedIndex].url
        refreshServerLEDStates()
        refreshSessions()
    }

    private func refreshServerLEDStates() {
        let currentServers = servers
        Task { [weak self, api] in
            var states: [String: LEDState] = [:]
            await withTaskGroup(of: (String, LEDState).self) { group in
                for server in currentServers {
                    group.addTask { [api] in
                        do {
                            return (server.id, LEDState.aggregate(snapshot: try await api.loadStatus(for: server)))
                        } catch {
                            return (server.id, .off)
                        }
                    }
                }
                for await (id, state) in group {
                    states[id] = state
                }
            }
            await MainActor.run {
                self?.serverLEDStates = states
                self?.serverTable.reloadData()
            }
        }
    }

    private func refreshSessions() {
        guard let server = servers.first(where: { $0.id == selectedServerID }) else { return }
        detailTitle.stringValue = server.name
        detailSubtitle.stringValue = "Loading sessions from \(server.url)"
        renderSessionPlaceholder("Loading sessions...")

        Task { [weak self, api] in
            do {
                let status = try await api.loadStatus(for: server)
                await MainActor.run {
                    self?.renderSessions(status.sessions, server: server)
                }
            } catch {
                await MainActor.run {
                    self?.detailSubtitle.stringValue = server.url
                    self?.renderSessionPlaceholder("Connection failed: \(error.localizedDescription)")
                }
            }
        }
    }

    private func renderSessions(_ sessions: [CodexSessionSummary], server: CodexServerEndpoint) {
        detailSubtitle.stringValue = "\(server.url) · \(sessions.count) session\(sessions.count == 1 ? "" : "s")"
        serverLEDStates[server.id] = LEDState.aggregate(snapshot: CodexStatusSnapshot(
            serverTime: nil,
            overallState: sessions.isEmpty ? .idle : .run,
            overallStateDetail: nil,
            sessionsCount: sessions.count,
            sessions: sessions
        ))
        serverTable.reloadData()
        guard !sessions.isEmpty else {
            renderSessionPlaceholder("No active sessions")
            return
        }
        sessionRows = sessions
        sessionPlaceholder = ""
        sessionTable.rowHeight = 72
        sessionTable.reloadData()
    }

    private func renderSessionPlaceholder(_ text: String) {
        sessionRows = []
        sessionPlaceholder = text
        sessionTable.rowHeight = 48
        sessionTable.reloadData()
    }

    @objc private func addServer() {
        let draft = CodexServerEndpoint(id: UUID().uuidString, name: "New Server", url: "")
        draftServerID = draft.id
        selectedServerID = draft.id
        servers.append(draft)
        isApplyingSelection = true
        serverTable.reloadData()
        serverTable.selectRowIndexes(IndexSet(integer: servers.count - 1), byExtendingSelection: false)
        isApplyingSelection = false
        nameField.stringValue = draft.name
        urlField.stringValue = ""
        detailTitle.stringValue = draft.name
        detailSubtitle.stringValue = "Enter a URL and save this server."
        renderSessionPlaceholder("No sessions until the server is saved")
        urlField.becomeFirstResponder()
    }

    @objc private func removeServer() {
        if draftServerID == selectedServerID {
            draftServerID = nil
            selectedServerID = api.servers.first?.id ?? ""
            reloadSelection()
            return
        }
        api.removeServer(id: selectedServerID)
        selectedServerID = api.servers.first?.id ?? ""
        reloadSelection()
        onSave?()
    }

    @objc private func saveSettings() {
        if draftServerID == selectedServerID {
            guard let server = api.addServer(url: urlField.stringValue) else {
                showError("Enter a valid, non-duplicate server URL.")
                return
            }
            _ = api.updateServer(id: server.id, name: nameField.stringValue, url: server.url)
            selectedServerID = server.id
            draftServerID = nil
        } else if api.updateServer(id: selectedServerID, name: nameField.stringValue, url: urlField.stringValue) == nil {
            showError("Enter a valid, non-duplicate server URL.")
            return
        }
        UserDefaults.standard.set(bleButton.state == .on, forKey: DeviceOutputController.bleEnabledKey)
        UserDefaults.standard.set(usbButton.state == .on, forKey: DeviceOutputController.usbEnabledKey)
        reloadSelection()
        onSave?()
    }

    @objc private func deviceOutputToggled() {
        UserDefaults.standard.set(bleButton.state == .on, forKey: DeviceOutputController.bleEnabledKey)
        UserDefaults.standard.set(usbButton.state == .on, forKey: DeviceOutputController.usbEnabledKey)
        onSave?()
    }

    private func showError(_ message: String) {
        let alert = NSAlert()
        alert.messageText = "Codex-Buddy"
        alert.informativeText = message
        alert.addButton(withTitle: "OK")
        if let window {
            alert.beginSheetModal(for: window)
        } else {
            alert.runModal()
        }
    }

    func windowShouldClose(_ sender: NSWindow) -> Bool {
        onClose?()
        return true
    }
}
