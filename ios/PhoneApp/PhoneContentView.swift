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
                    Button("Server") {
                        showSettings = true
                    }
                }
                ToolbarItem(placement: .topBarTrailing) {
                    Button {
                        Task { await model.refresh() }
                    } label: {
                        if model.isLoading {
                            ProgressView()
                        } else {
                            Text("Refresh")
                        }
                    }
                    .disabled(model.isLoading)
                }
            }
            .refreshable {
                await model.refresh()
            }
            .task {
                if model.snapshot.sessions.isEmpty {
                    await model.refresh()
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
            .onChange(of: scenePhase) { _, newPhase in
                guard newPhase == .active else { return }
                Task { await model.refresh() }
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
                    Text(model.snapshot.activeSessionDisplayTitle ?? model.snapshot.activeSessionID ?? "No active session")
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
            Text(model.baseURLText.isEmpty ? "Set a base URL in Server settings." : model.baseURLText)
                .font(.footnote)
                .foregroundStyle(.secondary)
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
                                Text(session.listTitle)
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
                        if !session.assistantSummary.isEmpty {
                            Text(session.assistantSummary)
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

    var body: some View {
        NavigationStack {
            Form {
                Section("Codex Buddy Base URL") {
                    TextField("https://codex-box.tailnet.ts.net", text: $model.baseURLText)
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled()
                        .keyboardType(.URL)
                    Text("Use the Tailscale URL of the machine running the codex-buddy webserver.")
                        .font(.footnote)
                        .foregroundStyle(.secondary)
                }
            }
            .navigationTitle("Server")
            .toolbar {
                ToolbarItem(placement: .topBarLeading) {
                    Button("Close") { dismiss() }
                }
                ToolbarItem(placement: .topBarTrailing) {
                    Button("Save") {
                        model.saveServerURL()
                        Task { await model.refresh() }
                        dismiss()
                    }
                }
            }
        }
    }
}
