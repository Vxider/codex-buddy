import SwiftUI

struct PhoneContentView: View {
    @ObservedObject var model: PhoneAppModel
    @State private var showSettings = false
    @State private var presentedErrorMessage: String?
    @Environment(\.scenePhase) private var scenePhase

    private var openSessions: [CodexSessionSummary] {
        model.snapshot.sessions.filter { $0.needsOpen }
    }

    private var primarySessions: [CodexSessionSummary] {
        model.snapshot.sessions.filter { !$0.needsOpen }
    }

    var body: some View {
        NavigationStack {
            List {
                summarySection
                sessionsSection
            }
            .navigationTitle("Agent Buddy")
            .toolbar {
                ToolbarItem(placement: .topBarLeading) {
                    Button {
                        showSettings = true
                    } label: {
                        Image(systemName: "gearshape")
                    }
                }
                ToolbarItem(placement: .topBarTrailing) {
                    Button {
                        Task { await model.refresh() }
                    } label: {
                        Image(systemName: "arrow.clockwise")
                    }
                    .disabled(model.isLoading)
                }
            }
            .refreshable {
                await model.refresh()
            }
            .task(id: scenePhase) {
                guard scenePhase == .active else { return }
                await model.refreshSilently()
                while !Task.isCancelled {
                    try? await Task.sleep(for: .seconds(5))
                    guard !Task.isCancelled else { break }
                    await model.refreshSilently()
                }
            }
            .sheet(isPresented: $showSettings) {
                ServerSettingsView(model: model)
            }
            .onOpenURL { url in
                guard let link = CodexDeepLink(url: url) else { return }
                switch link {
                case .sessions:
                    break
                case .serverSettings:
                    showSettings = true
                }
            }
            .onChange(of: model.errorMessage) { _, newValue in
                presentedErrorMessage = newValue
            }
            .alert("Connection Error", isPresented: Binding(
                get: { presentedErrorMessage != nil },
                set: { isPresented in
                    guard !isPresented else { return }
                    presentedErrorMessage = nil
                    Task { @MainActor in
                        model.errorMessage = nil
                    }
                }
            )) {
                Button("OK", role: .cancel) {}
            } message: {
                Text(presentedErrorMessage ?? "Unknown error")
            }
        }
    }

    private var summarySection: some View {
        Section("Status") {
            HStack(alignment: .center, spacing: 12) {
                Text(model.snapshot.overallState.face)
                    .font(.system(size: 36))
                VStack(alignment: .leading, spacing: 4) {
                    Text(model.isOffline ? CodexState.offline.displayName : model.snapshot.overallState.displayName)
                        .font(.headline)
                    Text("Updated \(CodexFormatters.shortTime(model.snapshot.serverTime))")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
            }
        }
    }

    private var sessionsSection: some View {
        Section("Sessions") {
            if model.snapshot.sessions.isEmpty {
                Text("No active sessions")
                    .foregroundStyle(.secondary)
            } else {
                if !openSessions.isEmpty {
                    openSessionsCard
                }
                ForEach(primarySessions) { session in
                    sessionRow(session)
                }
            }
        }
    }

    private var openSessionsCard: some View {
        VStack(alignment: .leading, spacing: 12) {
            HStack(alignment: .top) {
                VStack(alignment: .leading, spacing: 4) {
                    Text("Open")
                        .font(.headline)
                    Text("\(openSessions.count) session\(openSessions.count == 1 ? "" : "s")")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
                Spacer()
                sessionStateBadge(.open)
            }
            ForEach(openSessions) { session in
                VStack(alignment: .leading, spacing: 8) {
                    HStack(alignment: .top) {
                        VStack(alignment: .leading, spacing: 4) {
                            Text(session.phoneListTitle)
                                .font(.subheadline.weight(.semibold))
                            Text(session.shortSessionID ?? CodexFormatters.shortSessionLabel(session.sessionID))
                                .font(.caption)
                                .foregroundStyle(.secondary)
                        }
                        Spacer()
                    }
                    if !session.phoneSummary.isEmpty {
                        Text(session.phoneSummary)
                            .font(.subheadline)
                    }
                    HStack {
                        Text("Updated \(CodexFormatters.relativeTime(session.updatedAt))")
                            .font(.caption)
                            .foregroundStyle(.secondary)
                        Spacer()
                        if session.canContinue {
                            Button(session.continueAction?.label ?? "Continue") {
                                Task { await model.continueSession(session) }
                            }
                            .buttonStyle(.borderedProminent)
                            .tint(.teal)
                            .controlSize(.small)
                        }
                    }
                }
                if session.id != openSessions.last?.id {
                    Divider()
                }
            }
        }
        .padding(.vertical, 4)
    }

    private func sessionRow(_ session: CodexSessionSummary) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack(alignment: .top) {
                VStack(alignment: .leading, spacing: 4) {
                    Text(session.phoneListTitle)
                        .font(.headline)
                    Text(session.shortSessionID ?? CodexFormatters.shortSessionLabel(session.sessionID))
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
                Spacer()
                sessionStateBadge(session.state)
            }
            if !session.phoneAssistantSummary.isEmpty {
                Text(session.phoneAssistantSummary)
                    .font(.subheadline)
            }
            HStack {
                Text("Updated \(CodexFormatters.relativeTime(session.updatedAt))")
                    .font(.caption)
                    .foregroundStyle(.secondary)
                Spacer()
                if session.canContinue {
                    Button(session.continueAction?.label ?? "Continue") {
                        Task { await model.continueSession(session) }
                    }
                    .buttonStyle(.borderedProminent)
                    .tint(.teal)
                    .controlSize(.small)
                }
            }
        }
        .padding(.vertical, 4)
    }

    private func sessionStateBadge(_ state: CodexState) -> some View {
        let tint = state.badgeColor

        return Text(state.displayName)
            .font(.caption.weight(.bold))
        .padding(.horizontal, 10)
        .padding(.vertical, 6)
        .foregroundStyle(.white)
        .background(tint, in: Capsule())
        .overlay {
            Capsule()
                .strokeBorder(tint.opacity(0.55), lineWidth: 1)
        }
    }
}

private extension CodexState {
    var badgeColor: Color {
        switch normalized {
        case .offline:
            return .gray
        case .idle:
            return .green
        case .run, .running:
            return .blue
        case .open:
            return .orange
        case .error:
            return .red
        case .runningBash:
            return .blue
        }
    }
}

struct ServerSettingsView: View {
    @ObservedObject var model: PhoneAppModel
    @Environment(\.dismiss) private var dismiss
    @State private var editorDestination: ServerEditorDestination?

    var body: some View {
        NavigationStack {
            Form {
                Section("Current Server") {
                    if let activeServer = model.activeServer {
                        VStack(alignment: .leading, spacing: 4) {
                            Text(activeServer.displayName)
                                .font(.headline)
                            Text(activeServer.baseURL)
                                .font(.footnote)
                                .foregroundStyle(.secondary)
                        }
                    } else {
                        Text("Add a server to start connecting.")
                            .foregroundStyle(.secondary)
                    }
                }

                Section("Saved Servers") {
                    if model.servers.isEmpty {
                        Text("No servers configured")
                            .foregroundStyle(.secondary)
                    } else {
                        ForEach(model.servers) { server in
                            Button {
                                Task { await model.selectServer(server) }
                            } label: {
                                HStack(spacing: 12) {
                                    VStack(alignment: .leading, spacing: 4) {
                                        Text(server.displayName)
                                            .foregroundStyle(.primary)
                                        Text(server.baseURL)
                                            .font(.footnote)
                                            .foregroundStyle(.secondary)
                                    }
                                    Spacer()
                                    if model.selectedServerID == server.id {
                                        Image(systemName: "checkmark.circle.fill")
                                            .foregroundStyle(.tint)
                                    }
                                }
                            }
                            .buttonStyle(.plain)
                            .swipeActions(edge: .trailing, allowsFullSwipe: false) {
                                Button("Edit") {
                                    editorDestination = .edit(server)
                                }
                                Button("Delete", role: .destructive) {
                                    model.deleteServer(server)
                                }
                            }
                        }
                    }
                }
            }
            .navigationTitle("Servers")
            .toolbar {
                ToolbarItem(placement: .topBarLeading) {
                    Button("Close") { dismiss() }
                }
                ToolbarItem(placement: .topBarTrailing) {
                    Button {
                        editorDestination = .add
                    } label: {
                        Image(systemName: "plus")
                    }
                }
            }
            .sheet(item: $editorDestination) { destination in
                ServerEditorView(model: model, editingServer: destination.server)
            }
        }
    }
}

private enum ServerEditorDestination: Identifiable {
    case add
    case edit(AgentBuddyServer)

    var id: String {
        switch self {
        case .add:
            return "add"
        case let .edit(server):
            return "edit-\(server.id.uuidString)"
        }
    }

    var server: AgentBuddyServer? {
        switch self {
        case .add:
            return nil
        case let .edit(server):
            return server
        }
    }
}

private struct ServerEditorView: View {
    @ObservedObject var model: PhoneAppModel
    let editingServer: AgentBuddyServer?

    @Environment(\.dismiss) private var dismiss
    @State private var name: String
    @State private var baseURL: String
    @State private var errorMessage: String?

    init(model: PhoneAppModel, editingServer: AgentBuddyServer?) {
        self.model = model
        self.editingServer = editingServer
        _name = State(initialValue: editingServer?.name ?? "")
        _baseURL = State(initialValue: editingServer?.baseURL ?? "")
    }

    var body: some View {
        NavigationStack {
            Form {
                Section("Server") {
                    TextField("Office", text: $name)
                    TextField("http://100.82.10.4:8787", text: $baseURL)
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled()
                        .keyboardType(.URL)
                    Text("You can save multiple HTTP or HTTPS Agent Buddy servers and switch between them at any time.")
                        .font(.footnote)
                        .foregroundStyle(.secondary)
                }
            }
            .navigationTitle(editingServer == nil ? "Add Server" : "Edit Server")
            .toolbar {
                ToolbarItem(placement: .topBarLeading) {
                    Button("Cancel") { dismiss() }
                }
                ToolbarItem(placement: .topBarTrailing) {
                    Button("Save") {
                        do {
                            try model.saveServer(name: name, baseURL: baseURL, editing: editingServer)
                            Task { await model.refreshSilently() }
                            dismiss()
                        } catch {
                            errorMessage = error.localizedDescription
                        }
                    }
                    .disabled(baseURL.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
                }
            }
            .alert("Invalid Server", isPresented: Binding(
                get: { errorMessage != nil },
                set: { isPresented in
                    guard !isPresented else { return }
                    errorMessage = nil
                }
            )) {
                Button("OK", role: .cancel) {}
            } message: {
                Text(errorMessage ?? "Unknown error")
            }
        }
    }
}
