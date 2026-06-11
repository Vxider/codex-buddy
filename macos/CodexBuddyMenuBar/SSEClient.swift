import Foundation

final class SSEClient: NSObject, URLSessionDataDelegate {
    private let api: CodexBuddyAPI
    private let decoder = makeCodexDecoder()
    private var session: URLSession?
    private var task: URLSessionDataTask?
    private var buffer = ""
    private var eventName = ""
    private var eventData = ""
    private var reconnectWorkItem: DispatchWorkItem?
    private var active = false
    var onStatus: ((CodexStatusSnapshot) -> Void)?
    var onDisconnect: ((Error?) -> Void)?

    init(api: CodexBuddyAPI) {
        self.api = api
        super.init()
    }

    func start() {
        stop()
        active = true
        do {
            let url = try api.streamURL()
            var request = URLRequest(url: url)
            request.setValue("text/event-stream", forHTTPHeaderField: "Accept")
            let session = URLSession(configuration: .default, delegate: self, delegateQueue: nil)
            let task = session.dataTask(with: request)
            self.session = session
            self.task = task
            task.resume()
        } catch {
            onDisconnect?(error)
            scheduleReconnect()
        }
    }

    func stop() {
        active = false
        reconnectWorkItem?.cancel()
        reconnectWorkItem = nil
        task?.cancel()
        task = nil
        session?.invalidateAndCancel()
        session = nil
        buffer = ""
        eventName = ""
        eventData = ""
    }

    func urlSession(_ session: URLSession, dataTask: URLSessionDataTask, didReceive data: Data) {
        guard dataTask === task else { return }
        guard let chunk = String(data: data, encoding: .utf8) else { return }
        buffer += chunk
        while let range = buffer.range(of: "\n") {
            let line = String(buffer[..<range.lowerBound]).trimmingCharacters(in: CharacterSet(charactersIn: "\r"))
            buffer.removeSubrange(buffer.startIndex...range.lowerBound)
            consumeLine(line)
        }
    }

    func urlSession(_ session: URLSession, task: URLSessionTask, didCompleteWithError error: Error?) {
        guard task === self.task else { return }
        guard active else { return }
        onDisconnect?(error)
        scheduleReconnect()
    }

    private func consumeLine(_ line: String) {
        if line.isEmpty {
            flushEvent()
            return
        }
        if line.hasPrefix("event:") {
            eventName = line.dropFirst("event:".count).trimmingCharacters(in: .whitespaces)
            return
        }
        if line.hasPrefix("data:") {
            if !eventData.isEmpty {
                eventData += "\n"
            }
            eventData += line.dropFirst("data:".count).trimmingCharacters(in: .whitespaces)
        }
    }

    private func flushEvent() {
        defer {
            eventName = ""
            eventData = ""
        }
        guard eventName == "status", !eventData.isEmpty, let data = eventData.data(using: .utf8) else {
            return
        }
        if let status = try? decoder.decode(CodexStatusSnapshot.self, from: data) {
            onStatus?(status)
        }
    }

    private func scheduleReconnect() {
        guard active else { return }
        reconnectWorkItem?.cancel()
        let item = DispatchWorkItem { [weak self] in
            guard let self else { return }
            self.reconnectWorkItem = nil
            self.start()
        }
        reconnectWorkItem = item
        DispatchQueue.main.asyncAfter(deadline: .now() + 3, execute: item)
    }
}
