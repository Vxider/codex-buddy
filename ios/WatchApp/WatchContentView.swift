import SwiftUI

struct WatchContentView: View {
    @ObservedObject var model: WatchAppModel
    @State private var presentedErrorMessage: String?
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
                        Text(model.snapshot.watchActiveSessionTitle ?? "No active session")
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
                                    Text(session.watchListTitle)
                                        .font(.headline)
                                    Text(session.shortSessionID ?? CodexFormatters.shortSessionLabel(session.sessionID))
                                        .font(.caption2)
                                        .foregroundStyle(.secondary)
                                }
                            }
                            if !session.watchAssistantSummary.isEmpty {
                                Text(session.watchAssistantSummary)
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
                                    .tint(.teal)
                                }
                            }
                        }
                        .padding(.vertical, 4)
                    }
                }
            }
        }
        .navigationTitle("Codex")
        .onOpenURL { _ in }
        .onChange(of: scenePhase) { _, _ in }
        .onChange(of: model.errorMessage) { _, newValue in
            presentedErrorMessage = newValue
        }
        .alert("Bridge Error", isPresented: Binding(
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
