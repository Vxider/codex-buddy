import AppKit

final class SettingsWindowController: NSWindowController {
    private let api: CodexBuddyAPI
    private let serverField = NSTextField()
    private let bleButton = NSButton(checkboxWithTitle: "BLE LED", target: nil, action: nil)
    private let usbButton = NSButton(checkboxWithTitle: "USB LED", target: nil, action: nil)
    var onSave: (() -> Void)?

    init(api: CodexBuddyAPI) {
        self.api = api
        let window = NSWindow(
            contentRect: NSRect(x: 0, y: 0, width: 360, height: 190),
            styleMask: [.titled, .closable],
            backing: .buffered,
            defer: false
        )
        window.title = "Codex Buddy Settings"
        window.center()
        super.init(window: window)
        buildUI()
    }

    required init?(coder: NSCoder) {
        nil
    }

    private func buildUI() {
        guard let contentView = window?.contentView else { return }
        let stack = NSStackView()
        stack.orientation = .vertical
        stack.alignment = .leading
        stack.spacing = 14
        stack.translatesAutoresizingMaskIntoConstraints = false

        let serverLabel = NSTextField(labelWithString: "Server URL")
        serverField.stringValue = api.baseURLString
        serverField.placeholderString = CodexBuddyAPI.defaultBaseURL
        serverField.target = self
        serverField.action = #selector(saveSettings)
        serverField.translatesAutoresizingMaskIntoConstraints = false

        let switches = NSStackView(views: [bleButton, usbButton])
        switches.orientation = .vertical
        switches.alignment = .leading
        switches.spacing = 8

        let saveButton = NSButton(title: "Save", target: self, action: #selector(saveSettings))
        saveButton.bezelStyle = .rounded

        stack.addArrangedSubview(serverLabel)
        stack.addArrangedSubview(serverField)
        stack.addArrangedSubview(switches)
        stack.addArrangedSubview(saveButton)
        contentView.addSubview(stack)

        NSLayoutConstraint.activate([
            stack.leadingAnchor.constraint(equalTo: contentView.leadingAnchor, constant: 20),
            stack.trailingAnchor.constraint(equalTo: contentView.trailingAnchor, constant: -20),
            stack.topAnchor.constraint(equalTo: contentView.topAnchor, constant: 20),
            serverField.widthAnchor.constraint(equalTo: stack.widthAnchor)
        ])
    }

    @objc private func saveSettings() {
        api.baseURLString = serverField.stringValue
        onSave?()
        window?.close()
    }
}
