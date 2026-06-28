import CoreBluetooth
import Darwin
import Foundation

final class DeviceOutputController {
    static let bleEnabledKey = "agentBuddyBLELEDEnabled"
    static let usbEnabledKey = "agentBuddyUSBLEDEnabled"

    private let defaults: UserDefaults
    private let queue = DispatchQueue(label: "agent-buddy.device-outputs")
    private let bleAdvertiser: BLEStatusAdvertiser
    private var usbPublisher: USBSerialStatusPublisher?
    private var lastFrame: StatusFrame?

    init(defaults: UserDefaults = .standard) {
        self.defaults = defaults
        self.bleAdvertiser = BLEStatusAdvertiser()
    }

    func reloadSettings() {
        queue.async { [weak self] in
            guard let self else { return }
            self.applySettings()
            if let lastFrame {
                self.publish(frame: lastFrame)
            }
        }
    }

    func publish(snapshot: CodexStatusSnapshot, ledState: LEDState) {
        let frame = StatusFrame(snapshot: snapshot, ledState: ledState)
        queue.async { [weak self] in
            guard let self else { return }
            self.lastFrame = frame
            self.applySettings()
            self.publish(frame: frame)
        }
    }

    func sendDanceCommand() {
        queue.async { [weak self] in
            guard let self else { return }
            self.applySettings()
            self.usbPublisher?.sendCommand("dance")
        }
    }

    func stop() {
        queue.sync {
            usbPublisher?.close()
            usbPublisher = nil
            bleAdvertiser.stop()
        }
    }

    private func applySettings() {
        if defaults.bool(forKey: Self.usbEnabledKey) {
            if usbPublisher == nil {
                usbPublisher = USBSerialStatusPublisher()
            }
        } else {
            usbPublisher?.close()
            usbPublisher = nil
        }

        if defaults.bool(forKey: Self.bleEnabledKey) {
            bleAdvertiser.start()
        } else {
            bleAdvertiser.stop()
        }
    }

    private func publish(frame: StatusFrame) {
        usbPublisher?.publish(frame)
        bleAdvertiser.publish(frame)
    }
}

struct StatusFrame: Equatable {
    let state: String
    let led: String
    let detail: String
    let sessionsCount: Int
    let summary: String
    let sequence: UInt8

    private static var nextSequence: UInt8 = 0

    init(snapshot: CodexStatusSnapshot, ledState: LEDState) {
        state = Self.stateName(snapshot.overallState)
        led = Self.ledName(ledState)
        detail = Self.sanitized(snapshot.overallStateDetail)
        sessionsCount = snapshot.sessionsCount
        summary = Self.sanitized(snapshot.sessions.first?.message)
        Self.nextSequence &+= 1
        sequence = Self.nextSequence
    }

    var serialLine: String {
        "CB1 state=\(state) led=\(led) detail=\(detail) sessions=\(sessionsCount) summary=\(summary)\n"
    }

    var blePayload: Data {
        Data([0xFF, 0xFF, 0x43, 0x42, 0x01, ledCode, stateCode, 0x00, sequence])
    }

    private var ledCode: UInt8 {
        switch led {
        case "red":
            return 1
        case "yellow":
            return 2
        case "green":
            return 3
        case "purple":
            return 4
        default:
            return 0
        }
    }

    private var stateCode: UInt8 {
        switch state {
        case "idle":
            return 1
        case "run":
            return 2
        case "open":
            return 3
        case "error":
            return 4
        default:
            return 0
        }
    }

    private static func stateName(_ state: CodexState) -> String {
        switch state.normalized {
        case .idle:
            return "idle"
        case .run, .running, .runningBash:
            return "run"
        case .open:
            return "open"
        case .error:
            return "error"
        case .offline:
            return "offline"
        }
    }

    private static func ledName(_ state: LEDState) -> String {
        switch state {
        case .off:
            return "off"
        case .working:
            return "green"
        case .attention:
            return "yellow"
        case .approval:
            return "red"
        case .goal:
            return "purple"
        }
    }

    private static func sanitized(_ value: String?) -> String {
        var result = (value ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
        result = result.replacingOccurrences(of: "\r", with: " ")
        result = result.replacingOccurrences(of: "\n", with: " ")
        result = result.replacingOccurrences(of: "|", with: " ")
        if result.count > 96 {
            result = String(result.prefix(96))
        }
        return result.isEmpty ? "-" : result
    }
}

private final class USBSerialStatusPublisher {
    private var fd: Int32 = -1
    private var devicePath: String?
    private var lastLine = ""

    func publish(_ frame: StatusFrame) {
        let line = frame.serialLine
        if line == lastLine {
            return
        }
        guard ensureOpen() else { return }
        let bytes = Array(line.utf8)
        let written = bytes.withUnsafeBufferPointer { pointer in
            write(fd, pointer.baseAddress, pointer.count)
        }
        if written == bytes.count {
            lastLine = line
        } else {
            NSLog("AgentBuddy USB serial write failed on \(devicePath ?? "-"): wrote \(written) of \(bytes.count)")
            close()
        }
    }

    func sendCommand(_ command: String) {
        writeLine(command.trimmingCharacters(in: .whitespacesAndNewlines) + "\n")
    }

    func close() {
        if fd >= 0 {
            Darwin.close(fd)
        }
        fd = -1
        devicePath = nil
        lastLine = ""
    }

    private func ensureOpen() -> Bool {
        if fd >= 0 {
            return true
        }
        guard let path = Self.detectDevice() else {
            NSLog("AgentBuddy USB serial device not found")
            return false
        }
        let nextFD = Darwin.open(path, O_WRONLY | O_NOCTTY | O_NONBLOCK)
        guard nextFD >= 0 else {
            NSLog("AgentBuddy USB serial open failed for \(path): errno \(errno)")
            return false
        }
        configure(fd: nextFD)
        fd = nextFD
        devicePath = path
        NSLog("AgentBuddy USB serial opened \(path)")
        return true
    }

    private func writeLine(_ line: String) {
        guard ensureOpen() else { return }
        let bytes = Array(line.utf8)
        let written = bytes.withUnsafeBufferPointer { pointer in
            write(fd, pointer.baseAddress, pointer.count)
        }
        if written != bytes.count {
            NSLog("AgentBuddy USB serial command write failed on \(devicePath ?? "-"): wrote \(written) of \(bytes.count)")
            close()
        }
    }

    private func configure(fd: Int32) {
        var options = termios()
        guard tcgetattr(fd, &options) == 0 else { return }
        cfmakeraw(&options)
        cfsetspeed(&options, speed_t(B115200))
        options.c_cflag |= tcflag_t(CLOCAL | CREAD)
        options.c_cflag &= ~tcflag_t(CSTOPB | PARENB | CRTSCTS)
        options.c_cflag = (options.c_cflag & ~tcflag_t(CSIZE)) | tcflag_t(CS8)
        _ = tcsetattr(fd, TCSANOW, &options)
    }

    private static func detectDevice() -> String? {
        guard let entries = try? FileManager.default.contentsOfDirectory(atPath: "/dev") else {
            return nil
        }
        let prefixes = [
            "cu.usbmodem",
            "cu.usbserial",
            "cu.SLAB_USBtoUART",
            "cu.wchusbserial",
            "cu.usbserial-"
        ]
        return entries
            .filter { name in prefixes.contains { name.hasPrefix($0) } }
            .sorted()
            .map { "/dev/\($0)" }
            .first
    }
}

private final class BLEStatusAdvertiser: NSObject, CBPeripheralManagerDelegate {
    private var manager: CBPeripheralManager?
    private var enabled = false
    private var lastFrame: StatusFrame?

    func start() {
        enabled = true
        if manager == nil {
            manager = CBPeripheralManager(delegate: self, queue: nil)
        } else {
            advertiseLastFrame()
        }
    }

    func stop() {
        enabled = false
        manager?.stopAdvertising()
    }

    func publish(_ frame: StatusFrame) {
        lastFrame = frame
        advertiseLastFrame()
    }

    func peripheralManagerDidUpdateState(_ peripheral: CBPeripheralManager) {
        advertiseLastFrame()
    }

    private func advertiseLastFrame() {
        guard enabled, let manager, manager.state == .poweredOn, let frame = lastFrame else {
            return
        }
        manager.stopAdvertising()
        manager.startAdvertising([
            CBAdvertisementDataLocalNameKey: "AgentBuddy",
            CBAdvertisementDataManufacturerDataKey: frame.blePayload
        ])
    }
}
