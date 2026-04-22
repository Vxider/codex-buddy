import SwiftUI

@main
struct CodexBuddyPhoneApp: App {
    @StateObject private var model = PhoneAppModel()

    var body: some Scene {
        WindowGroup {
            PhoneContentView(model: model)
        }
    }
}
