import SwiftUI

struct PhoneContentView: View {
    @ObservedObject var model: PhoneAppModel
    @State private var showSettings = false
    @Environment(\.scenePhase) private var scenePhase

    var body: some View {
        NavigationStack {
            List {
                summarySection
                sessionsSection
            }
            .navigationTitle("Codex Buddy")
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
            .alert("Connection Error", isPresented: Binding(
                get: { model.errorMessage != nil },
                set: { if !$0 { model.errorMessage = nil } }
            )) {
                Button("OK", role: .cancel) {}
            } message: {
                Text(model.errorMessage ?? "Unknown error")
            }
        }
    }

    private var summarySection: some View {
        Section("Status") {
            HStack(alignment: .center, spacing: 12) {
                Text(model.snapshot.overallState.face)
                    .font(.system(size: 36))
                VStack(alignment: .leading, spacing: 4) {
                    Text(model.snapshot.overallState.displayName)
                        .font(.headline)
                    Text(model.snapshot.activeSessionCompactTitle ?? model.snapshot.activeSessionDisplayTitle ?? model.snapshot.activeSessionID ?? "No active session")
                        .font(.subheadline)
                        .foregroundStyle(.secondary)
                    Text("Updated \(CodexFormatters.shortTime(model.snapshot.serverTime))")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
            }
            if let detail = model.snapshot.overallStateDetail, !detail.isEmpty {
                Text(detail)
                    .font(.footnote)
                    .foregroundStyle(.secondary)
            }
        }
    }

    private var sessionsSection: some View {
        Section("Sessions") {
            if model.snapshot.sessions.isEmpty {
                Text("No active sessions")
                    .foregroundStyle(.secondary)
            } else {
                ForEach(model.snapshot.sessions) { session in
                    VStack(alignment: .leading, spacing: 8) {
                        HStack(alignment: .top) {
                            VStack(alignment: .leading, spacing: 4) {
                                Text(session.phoneTitle)
                                    .font(.headline)
                                Text(session.shortSessionID ?? CodexFormatters.shortSessionLabel(session.sessionID))
                                    .font(.caption)
                                    .foregroundStyle(.secondary)
                            }
                            Spacer()
                            Text(session.state.face)
                                .font(.title3)
                            Text(session.state.displayName)
                                .font(.caption)
                                .foregroundStyle(.secondary)
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
                                .controlSize(.small)
                            }
                        }
                    }
                    .padding(.vertical, 4)
                }
            }
        }
    }
}

struct ServerSettingsView: View {
    @ObservedObject var model: PhoneAppModel
    @Environment(\.dismiss) private var dismiss
    @State private var showEditor = false
    @State private var editingServer: CodexBuddyServer?

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
                                    editingServer = server
                                    showEditor = true
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
                        editingServer = nil
                        showEditor = true
                    } label: {
                        Image(systemName: "plus")
                    }
                }
            }
            .sheet(isPresented: $showEditor) {
                ServerEditorView(model: model, editingServer: editingServer)
            }
        }
    }
}

private struct ServerEditorView: View {
    @ObservedObject var model: PhoneAppModel
    let editingServer: CodexBuddyServer?

    @Environment(\.dismiss) private var dismiss
    @State private var name: String
    @State private var baseURL: String
    @State private var errorMessage: String?

    init(model: PhoneAppModel, editingServer: CodexBuddyServer?) {
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
                    Text("You can save multiple HTTP or HTTPS Codex Buddy servers and switch between them at any time.")
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
                set: { if !$0 { errorMessage = nil } }
            )) {
                Button("OK", role: .cancel) {}
            } message: {
                Text(errorMessage ?? "Unknown error")
            }
        }
    }
}
