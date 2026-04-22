import SwiftUI

struct WatchContentView: View {
    @ObservedObject var model: WatchAppModel
    @Environment(\.scenePhase) private var scenePhase

    var body: some View {
        List {
            Section {
                HStack(spacing: 10) {
                    Text(model.snapshot.overallState.face)
                        .font(.title)
                    VStack(alignment: .leading, spacing: 2) {
                        Text(model.snapshot.overallState.displayName)
                            .font(.headline)
                        Text(model.snapshot.activeSessionMicroTitle ?? model.snapshot.activeSessionCompactTitle ?? model.snapshot.activeSessionDisplayTitle ?? model.snapshot.activeSessionID ?? "No active session")
                            .font(.caption2)
                            .foregroundStyle(.secondary)
                    }
                }
                Text("Updated \(CodexFormatters.shortTime(model.snapshot.serverTime))")
                    .font(.caption2)
                    .foregroundStyle(.secondary)
                Button {
                    Task { await model.refresh() }
                } label: {
                    if model.isRefreshing {
                        ProgressView()
                    } else {
                        Text("Refresh")
                    }
                }
            }

            Section("Sessions") {
                if model.snapshot.sessions.isEmpty {
                    Text("No active sessions")
                        .foregroundStyle(.secondary)
                } else {
                    ForEach(model.snapshot.sessions) { session in
                        VStack(alignment: .leading, spacing: 6) {
                            HStack(alignment: .top) {
                                Text(session.state.face)
                                VStack(alignment: .leading, spacing: 2) {
                                    Text(session.watchTitle)
                                        .font(.headline)
                                    Text(session.shortSessionID ?? CodexFormatters.shortSessionLabel(session.sessionID))
                                        .font(.caption2)
                                        .foregroundStyle(.secondary)
                                }
                            }
                            if !session.watchSummary.isEmpty {
                                Text(session.watchSummary)
                                    .font(.caption)
                            }
                            HStack {
                                Text(CodexFormatters.relativeTime(session.updatedAt))
                                    .font(.caption2)
                                    .foregroundStyle(.secondary)
                                Spacer()
                                if session.canContinue {
                                    Button("Continue") {
                                        Task { await model.continueSession(session) }
                                    }
                                    .buttonStyle(.borderedProminent)
                                }
                            }
                        }
                        .padding(.vertical, 4)
                    }
                }
            }
        }
        .navigationTitle("Codex")
        .task {
            if model.snapshot.sessions.isEmpty {
                await model.refresh()
            }
        }
        .onOpenURL { _ in
            Task { await model.refresh() }
        }
        .onChange(of: scenePhase) { _, newPhase in
            guard newPhase == .active else { return }
            Task { await model.refresh() }
        }
        .alert("Bridge Error", isPresented: Binding(
            get: { model.errorMessage != nil },
            set: { if !$0 { model.errorMessage = nil } }
        )) {
            Button("OK", role: .cancel) {}
        } message: {
            Text(model.errorMessage ?? "Unknown error")
        }
    }
}
