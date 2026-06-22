#include <Arduino.h>
#include <BLEDevice.h>
#include <BLEScan.h>
#include <BLEUtils.h>
#include <driver/rmt.h>
#include <esp_err.h>
#include <stdarg.h>

namespace {

constexpr uint8_t kMotorPin = 10;
constexpr uint8_t kLedPin = 3;
constexpr uint8_t kLedCount = 21;
constexpr uint8_t kLedGroups = 3;
constexpr uint8_t kLedsPerGroup = 7;
constexpr uint8_t kBrightness = 13;
constexpr rmt_channel_t kRmtChannel = RMT_CHANNEL_0;
constexpr uint32_t kOfflineAfterMs = 45000;
constexpr uint32_t kDemoStepMs = 4500;
constexpr uint32_t kRenderFrameMs = 33;
constexpr uint32_t kSignalFlashPeriodMs = 1400;
constexpr uint32_t kSignalFlashOnMs = 820;
constexpr uint32_t kBleScanDurationSeconds = 1;
constexpr uint32_t kBleScanPeriodMs = 2000;
constexpr uint32_t kDanceDurationMs = 5000;
constexpr uint8_t kDanceMotorDuty = 180;

struct Color {
  uint8_t r;
  uint8_t g;
  uint8_t b;
};

enum class CodexState {
  Boot,
  Offline,
  Idle,
  Run,
  Open,
  Error,
};

enum class SignalColor {
  State,
  Off,
  Red,
  Yellow,
  Green,
  Purple,
};

Color leds[kLedCount]{};
CodexState currentState = CodexState::Boot;
SignalColor currentSignal = SignalColor::State;
uint32_t lastDemoStep = 0;
uint32_t lastRender = 0;
uint32_t stateStartedAt = 0;
uint32_t lastFrameAt = 0;
uint8_t motorDuty = 0;
bool motorManual = false;
uint32_t motorAutoOffAt = 0;
uint32_t lastBleScanAt = 0;
bool bleScanRunning = false;
uint8_t lastBleSeq = 0;
bool haveBleSeq = false;
String serialLine;
BLEScan *bleScan = nullptr;

void sendWs2812();
Color redSignal();
Color yellowSignal();
Color greenSignal();
void renderPurplePipeline(uint32_t now);

void logLine(const char *message) {
  Serial.println(message);
#if ARDUINO_USB_CDC_ON_BOOT
  Serial0.println(message);
#endif
}

void logPrintf(const char *format, ...) {
  char buffer[160];
  va_list args;
  va_start(args, format);
  vsnprintf(buffer, sizeof(buffer), format, args);
  va_end(args);
  logLine(buffer);
}

uint8_t scale(uint8_t value, uint8_t brightness = kBrightness) {
  return static_cast<uint8_t>((static_cast<uint16_t>(value) * brightness) / 255);
}

Color rgb(uint8_t r, uint8_t g, uint8_t b) {
  return Color{scale(r), scale(g), scale(b)};
}

Color scaleColor(Color color, uint8_t brightness) {
  return Color{
      static_cast<uint8_t>((static_cast<uint16_t>(color.r) * brightness) / 255),
      static_cast<uint8_t>((static_cast<uint16_t>(color.g) * brightness) / 255),
      static_cast<uint8_t>((static_cast<uint16_t>(color.b) * brightness) / 255),
  };
}

uint8_t breathe(uint32_t now, uint32_t periodMs = 1600, uint8_t low = 70, uint8_t high = 255) {
  const float phase = (now % periodMs) / static_cast<float>(periodMs);
  const float wave = 0.5f + 0.5f * sinf(phase * TWO_PI);
  return static_cast<uint8_t>(low + (high - low) * wave);
}

uint8_t flashBrightness(uint32_t now) {
  const uint32_t phaseMs = now % kSignalFlashPeriodMs;
  if (phaseMs >= kSignalFlashOnMs) {
    return 0;
  }
  return 255;
}

void setState(CodexState state) {
  if (currentState == state) {
    return;
  }
  currentState = state;
  stateStartedAt = millis();
  Serial.print("state ");
  switch (state) {
    case CodexState::Boot:
      Serial.println("boot");
      break;
    case CodexState::Offline:
      Serial.println("offline");
      break;
    case CodexState::Idle:
      Serial.println("idle");
      break;
    case CodexState::Run:
      Serial.println("run");
      break;
    case CodexState::Open:
      Serial.println("open");
      break;
    case CodexState::Error:
      Serial.println("error");
      break;
  }
}

void setMotor(uint8_t duty) {
  motorDuty = duty;
  analogWrite(kMotorPin, motorDuty);
}

void triggerDance(uint32_t now) {
  motorManual = false;
  setMotor(kDanceMotorDuty);
  motorAutoOffAt = now + kDanceDurationMs;
  currentSignal = SignalColor::Purple;
  stateStartedAt = now;
  Serial.println("dance motor 5s");
}

void clearLeds() {
  for (auto &led : leds) {
    led = Color{0, 0, 0};
  }
}

void blackoutLeds() {
  clearLeds();
  for (uint8_t i = 0; i < 4; i++) {
    sendWs2812();
    delay(2);
  }
}

void fillAll(Color color) {
  for (auto &led : leds) {
    led = color;
  }
}

void fillGroup(uint8_t group, Color color) {
  const uint8_t start = group * kLedsPerGroup;
  for (uint8_t i = 0; i < kLedsPerGroup && start + i < kLedCount; i++) {
    leds[start + i] = color;
  }
}

void fillSignalGroup(uint8_t group, Color color, uint32_t now) {
  const uint8_t brightness = flashBrightness(now);
  if (brightness == 0) {
    return;
  }
  fillGroup(group, scaleColor(color, brightness));
}

Color namedColor(const String &name) {
  if (name == "red") {
    return redSignal();
  }
  if (name == "yellow") {
    return yellowSignal();
  }
  if (name == "green") {
    return greenSignal();
  }
  if (name == "purple" || name == "violet") {
    return rgb(160, 80, 255);
  }
  return Color{0, 0, 0};
}

void sendWs2812() {
  rmt_item32_t items[kLedCount * 24]{};
  size_t pos = 0;

  for (const auto &led : leds) {
    const uint8_t bytes[3] = {led.g, led.r, led.b};
    for (uint8_t byte : bytes) {
      for (int bit = 7; bit >= 0; bit--) {
        const bool one = byte & (1 << bit);
        items[pos].level0 = 1;
        items[pos].duration0 = one ? 8 : 4;
        items[pos].level1 = 0;
        items[pos].duration1 = one ? 5 : 9;
        pos++;
      }
    }
  }

  esp_err_t err = rmt_write_items(kRmtChannel, items, pos, true);
  if (err != ESP_OK) {
    Serial.printf("rmt_write_items failed: %d\n", err);
  }
  delayMicroseconds(80);
}

void setupWs2812() {
  rmt_config_t config{};
  config.rmt_mode = RMT_MODE_TX;
  config.channel = kRmtChannel;
  config.gpio_num = static_cast<gpio_num_t>(kLedPin);
  config.mem_block_num = 1;
  config.clk_div = 8;  // 80 MHz APB / 8 = 10 MHz, one tick = 100 ns.
  config.tx_config.loop_en = false;
  config.tx_config.carrier_en = false;
  config.tx_config.idle_output_en = true;
  config.tx_config.idle_level = RMT_IDLE_LEVEL_LOW;

  ESP_ERROR_CHECK(rmt_config(&config));
  ESP_ERROR_CHECK(rmt_driver_install(kRmtChannel, 0, 0));
}

Color redSignal() {
  return rgb(255, 0, 0);
}

Color yellowSignal() {
  return rgb(255, 191, 0);
}

Color greenSignal() {
  return rgb(0, 255, 72);
}

void renderBoot(uint32_t now) {
  clearLeds();
  const uint8_t head = ((now - stateStartedAt) / 120) % kLedCount;
  leds[head] = scaleColor(rgb(160, 80, 255), breathe(now, 1200, 90, 255));
  leds[(head + kLedCount - 1) % kLedCount] = scaleColor(rgb(160, 80, 255), breathe(now, 1200, 35, 120));
}

void renderOffline(uint32_t now) {
  clearLeds();
}

void renderIdle(uint32_t now) {
  clearLeds();
  fillSignalGroup(2, greenSignal(), now);
}

void renderRun(uint32_t now) {
  clearLeds();
  fillSignalGroup(2, greenSignal(), now);
}

void renderOpen(uint32_t now) {
  clearLeds();
  fillSignalGroup(1, yellowSignal(), now);
}

void renderError(uint32_t now) {
  clearLeds();
  fillSignalGroup(0, redSignal(), now);
}

void renderPurplePipeline(uint32_t now) {
  clearLeds();
  const uint8_t base = ((now - stateStartedAt) / 110) % kLedsPerGroup;
  const uint8_t breath = breathe(now, 1800, 80, 255);
  for (uint8_t group = 0; group < kLedGroups; group++) {
    const uint8_t groupStart = group * kLedsPerGroup;
    Color color = redSignal();
    if (group == 1) {
      color = yellowSignal();
    } else if (group == 2) {
      color = greenSignal();
    }
    for (uint8_t trail = 0; trail < 4; trail++) {
      const uint8_t offset = (base + kLedsPerGroup - trail) % kLedsPerGroup;
      const uint8_t idx = groupStart + offset;
      const uint8_t trailBrightness = static_cast<uint8_t>((static_cast<uint16_t>(breath) * (4 - trail)) / 4);
      leds[idx] = scaleColor(color, trailBrightness);
    }
  }
}

void renderStatus(uint32_t now) {
  if (now - lastRender < kRenderFrameMs) {
    return;
  }
  lastRender = now;

  switch (currentSignal) {
    case SignalColor::State:
      switch (currentState) {
        case CodexState::Boot:
          renderBoot(now);
          break;
        case CodexState::Offline:
          renderOffline(now);
          break;
        case CodexState::Idle:
          renderIdle(now);
          break;
        case CodexState::Run:
          renderRun(now);
          break;
        case CodexState::Open:
          renderOpen(now);
          break;
        case CodexState::Error:
          renderError(now);
          break;
      }
      break;
    case SignalColor::Off:
      renderOffline(now);
      break;
    case SignalColor::Red:
      renderError(now);
      break;
    case SignalColor::Yellow:
      renderOpen(now);
      break;
    case SignalColor::Green:
      renderIdle(now);
      break;
    case SignalColor::Purple:
      renderPurplePipeline(now);
      break;
  }
  sendWs2812();
}

void applyStateText(String state) {
  state.trim();
  state.toLowerCase();
  if (state == "idle") {
    setState(CodexState::Idle);
  } else if (state == "run" || state == "running" || state == "running_bash") {
    setState(CodexState::Run);
  } else if (state == "open" || state == "attention") {
    setState(CodexState::Open);
  } else if (state == "error" || state == "failed") {
    setState(CodexState::Error);
  } else if (state == "offline") {
    setState(CodexState::Offline);
    currentSignal = SignalColor::Off;
    blackoutLeds();
  } else {
    Serial.printf("unknown state: %s\n", state.c_str());
  }
}

void applyLEDText(String led) {
  led.trim();
  led.toLowerCase();
  if (led == "red" || led == "approval" || led == "error") {
    currentSignal = SignalColor::Red;
  } else if (led == "yellow" || led == "attention" || led == "open") {
    currentSignal = SignalColor::Yellow;
  } else if (led == "green" || led == "working" || led == "idle") {
    currentSignal = SignalColor::Green;
  } else if (led == "purple" || led == "violet" || led == "goal" || led == "blocked") {
    currentSignal = SignalColor::Purple;
  } else if (led == "state" || led == "auto") {
    currentSignal = SignalColor::State;
  } else if (led == "off" || led == "-") {
    currentSignal = SignalColor::Off;
    blackoutLeds();
  } else {
    Serial.printf("unknown led: %s\n", led.c_str());
  }
}

void applyBLELEDCode(uint8_t code) {
  switch (code) {
    case 1:
      currentSignal = SignalColor::Red;
      break;
    case 2:
      currentSignal = SignalColor::Yellow;
      break;
    case 3:
      currentSignal = SignalColor::Green;
      break;
    case 4:
      currentSignal = SignalColor::Purple;
      break;
    default:
      currentSignal = SignalColor::Off;
      blackoutLeds();
      break;
  }
}

void applyBLEStateCode(uint8_t code) {
  switch (code) {
    case 1:
      setState(CodexState::Idle);
      break;
    case 2:
      setState(CodexState::Run);
      break;
    case 3:
      setState(CodexState::Open);
      break;
    case 4:
      setState(CodexState::Error);
      break;
    case 5:
      setState(CodexState::Run);
      break;
    default:
      setState(CodexState::Offline);
      break;
  }
}

bool handleBLEPayload(const uint8_t *data, size_t len) {
  if (data == nullptr || len < 9) {
    return false;
  }
  for (size_t i = 0; i + 8 < len; i++) {
    if (data[i] != 0xFF || data[i + 1] != 0xFF ||
        data[i + 2] != 'C' || data[i + 3] != 'B' || data[i + 4] != 1) {
      continue;
    }
    const uint8_t led = data[i + 5];
    const uint8_t state = data[i + 6];
    const uint8_t flags = data[i + 7];
    const uint8_t seq = data[i + 8];
    const bool fresh = !haveBleSeq || seq != lastBleSeq;
    logPrintf("ble led=%u state=%u flags=%u seq=%u", led, state, flags, seq);
    lastBleSeq = seq;
    haveBleSeq = true;
    lastFrameAt = millis();
    applyBLEStateCode(state);
    applyBLELEDCode(led);
    if (fresh && (flags & 0x01) != 0) {
      triggerDance(lastFrameAt);
      renderPurplePipeline(lastFrameAt);
      sendWs2812();
    }
    return true;
  }
  return false;
}

class CodexAdvertisedDeviceCallbacks : public BLEAdvertisedDeviceCallbacks {
  void onResult(BLEAdvertisedDevice advertisedDevice) override {
    if (!advertisedDevice.haveManufacturerData()) {
      return;
    }
    const std::string manufacturer = advertisedDevice.getManufacturerData();
    handleBLEPayload(reinterpret_cast<const uint8_t *>(manufacturer.data()), manufacturer.size());
  }
};

CodexAdvertisedDeviceCallbacks bleCallbacks;

void runDemoBeforeFirstFrame(uint32_t now) {
  (void)now;
}

String frameValue(const String &line, const String &key) {
  const String prefix = key + "=";
  int start = line.indexOf(prefix);
  if (start < 0) {
    return "";
  }
  start += prefix.length();
  int end = line.indexOf(' ', start);
  if (end < 0) {
    end = line.length();
  }
  return line.substring(start, end);
}

bool handleFrame(String line) {
  line.trim();
  if (!line.startsWith("CB1 ")) {
    return false;
  }

  const String state = frameValue(line, "state");
  const String led = frameValue(line, "led");
  if (state.length() == 0 && led.length() == 0) {
    Serial.println("bad frame: missing state or led");
    return true;
  }

  lastFrameAt = millis();
  if (state.length() > 0) {
    applyStateText(state);
  }
  if (led.length() > 0) {
    applyLEDText(led);
  }
  return true;
}

void handleCommand(String line) {
  line.trim();
  if (line.length() == 0) {
    return;
  }

  if (handleFrame(line)) {
    return;
  }

  String lower = line;
  lower.toLowerCase();
  if (lower == "auto") {
    setMotor(0);
    Serial.println("motor 0");
    return;
  }
  if (lower == "dance") {
    const uint32_t now = millis();
    triggerDance(now);
    renderPurplePipeline(now);
    sendWs2812();
    return;
  }
  if (lower.startsWith("motor ")) {
    const int value = constrain(lower.substring(6).toInt(), 0, 255);
    motorManual = true;
    motorAutoOffAt = 0;
    setMotor(static_cast<uint8_t>(value));
    Serial.printf("motor %d\n", value);
    return;
  }
  if (lower == "blackout") {
    currentSignal = SignalColor::Off;
    blackoutLeds();
    Serial.println("blackout ok");
    return;
  }
  if (lower.startsWith("pixel ")) {
    const int firstSpace = lower.indexOf(' ');
    const int secondSpace = lower.indexOf(' ', firstSpace + 1);
    if (secondSpace < 0) {
      Serial.println("usage: pixel <0-20> <red|yellow|green|purple|off>");
      return;
    }
    const int index = lower.substring(firstSpace + 1, secondSpace).toInt();
    String colorName = lower.substring(secondSpace + 1);
    colorName.trim();
    currentSignal = SignalColor::Off;
    clearLeds();
    if (index >= 0 && index < kLedCount) {
      leds[index] = namedColor(colorName);
      sendWs2812();
      Serial.printf("pixel %d %s\n", index, colorName.c_str());
    } else {
      blackoutLeds();
      Serial.println("pixel index out of range");
    }
    return;
  }
  if (lower == "help") {
    Serial.println("commands: CB1 state=<state> led=<red|yellow|green|purple|off>, dance, led <color>, pixel <0-20> <color>, blackout, idle, run, open, error, offline, motor 0-255, auto");
    return;
  }
  if (lower.startsWith("led ")) {
    applyLEDText(lower.substring(4));
    Serial.println("led ok");
    return;
  }

  applyStateText(lower);
}

void setupBle() {
  BLEDevice::init("");
  bleScan = BLEDevice::getScan();
  bleScan->setAdvertisedDeviceCallbacks(&bleCallbacks, false);
  bleScan->setActiveScan(false);
  bleScan->setInterval(160);
  bleScan->setWindow(80);
}

void onBleScanComplete(BLEScanResults) {
  if (bleScan != nullptr) {
    bleScan->clearResults();
  }
  bleScanRunning = false;
}

void pollBle(uint32_t now) {
  if (bleScan == nullptr || bleScanRunning || now - lastBleScanAt < kBleScanPeriodMs) {
    return;
  }
  lastBleScanAt = now;
  bleScanRunning = bleScan->start(kBleScanDurationSeconds, onBleScanComplete, false);
}

void readSerialCommands() {
  while (Serial.available() > 0) {
    const char c = static_cast<char>(Serial.read());
    if (c == '\n' || c == '\r') {
      handleCommand(serialLine);
      serialLine = "";
    } else if (serialLine.length() < 96) {
      serialLine += c;
    }
  }
}

}  // namespace

void setup() {
  Serial.begin(115200);
  delay(500);
  Serial.println("codex-buddy esp32-c3 uart firmware");
  Serial.printf("motor gpio=%u ws2812 gpio=%u leds=%u groups=%u\n", kMotorPin, kLedPin, kLedCount, kLedGroups);
  Serial.println("type help for serial commands");

  pinMode(kMotorPin, OUTPUT);
  setMotor(0);
  setupWs2812();
  blackoutLeds();
  setupBle();
  setState(CodexState::Offline);
  currentSignal = SignalColor::Off;
}

void loop() {
  const uint32_t now = millis();
  readSerialCommands();
  pollBle(now);
  const uint32_t currentNow = millis();
  if (!motorManual && motorAutoOffAt != 0 && static_cast<int32_t>(currentNow - motorAutoOffAt) >= 0) {
    setMotor(0);
    motorAutoOffAt = 0;
  }
  runDemoBeforeFirstFrame(currentNow);
  if (lastFrameAt != 0 && currentNow - lastFrameAt > kOfflineAfterMs) {
    setState(CodexState::Offline);
    currentSignal = SignalColor::Off;
    blackoutLeds();
  }
  renderStatus(currentNow);
}
