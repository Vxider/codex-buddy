import SwiftUI

@main
struct AgentBuddyPhoneApp: App {
    @StateObject private var model = PhoneAppModel()

    var body: some Scene {
        WindowGroup {
            PhoneContentView(model: model)
        }
    }
}
