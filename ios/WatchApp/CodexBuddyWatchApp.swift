import SwiftUI

@main
struct CodexBuddyWatchApp: App {
    @StateObject private var model = WatchAppModel()

    var body: some Scene {
        WindowGroup {
            NavigationStack {
                WatchContentView(model: model)
            }
        }
    }
}
