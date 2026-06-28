import SwiftUI

@main
struct AgentBuddyWatchApp: App {
    @StateObject private var model = WatchAppModel()

    var body: some Scene {
        WindowGroup {
            NavigationStack {
                WatchContentView(model: model)
            }
        }
    }
}
