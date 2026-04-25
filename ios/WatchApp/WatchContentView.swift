import SwiftUI

struct WatchContentView: View {
    @ObservedObject var model: WatchAppModel
    @State private var presentedErrorMessage: String?
    @Environment(\.scenePhase) private var scenePhase

    var body: some View {
        List {
            Section {
                VStack(alignment: .leading, spacing: 8) {
                    HStack(alignment: .top, spacing: 10) {
                        VStack(alignment: .leading, spacing: 3) {
                            statusBadge(model.snapshot.overallState)
                            Text(model.snapshot.watchActiveSessionTitle ?? "No active session")
                                .font(.caption2)
                                .foregroundStyle(.secondary)
                        }
                        Spacer(minLength: 0)
                        Button {
                            Task { await model.refresh() }
                        } label: {
                            if model.isRefreshing {
                                ProgressView()
                            } else {
                                Image(systemName: "arrow.clockwise")
                            }
                        }
                        .buttonStyle(.bordered)
                        .controlSize(.small)
                    }
                    HStack(spacing: 4) {
                        Text("Updated")
                            .font(.caption2)
                            .foregroundStyle(.secondary)
                        Text(CodexFormatters.shortTime(model.snapshot.serverTime))
                            .font(.caption2)
                            .foregroundStyle(.secondary)
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
                                Text(session.watchListTitle)
                                    .font(.headline)
                                Spacer()
                                statusBadge(session.state)
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

    private func statusBadge(_ state: CodexState) -> some View {
        let tint = watchBadgeColor(state)

        return Text(state.displayName)
            .font(.caption2.weight(.bold))
            .padding(.horizontal, 8)
            .padding(.vertical, 4)
            .foregroundStyle(.white)
            .background(tint, in: Capsule())
            .overlay {
                Capsule()
                    .strokeBorder(tint.opacity(0.55), lineWidth: 1)
            }
    }

    private func watchBadgeColor(_ state: CodexState) -> Color {
        switch state.normalized {
        case .offline:
            return .gray
        case .idle:
            return .green
        case .running:
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
