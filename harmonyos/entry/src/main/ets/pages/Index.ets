import { common } from '@kit.AbilityKit';
import {
  CodexSessionSummary,
  CodexState,
  CodexStatusSnapshot,
  EMPTY_STATUS,
  LightState,
  firstSessionSummary,
  lightColor,
  lightLabel,
  sessionLightState,
  stateLabel,
  statusLight
} from '../common/CodexModels';
import { AgentBuddyApi } from '../network/AgentBuddyApi';
import { StatusMonitor } from '../network/StatusMonitor';
import { LiveWindowController } from '../live/LiveWindowController';
import { AgentBuddySettings, SettingsStore } from '../store/SettingsStore';

@Entry
@Component
struct Index {
  @State private snapshot: CodexStatusSnapshot = EMPTY_STATUS;
  @State private connected: boolean = false;
  @State private errorMessage: string = '';
  @State private baseUrl: string = '';
  @State private serverName: string = 'Agent Buddy';
  @State private liveWindowEnabled: boolean = true;
  @State private liveWindowActive: boolean = false;
  @State private showSettings: boolean = true;
  @State private busy: boolean = false;

  private context?: common.UIAbilityContext;
  private readonly api: AgentBuddyApi = new AgentBuddyApi();
  private readonly monitor: StatusMonitor = new StatusMonitor(this.api);
  private readonly liveWindow: LiveWindowController = new LiveWindowController();

  aboutToAppear(): void {
    this.context = getContext(this) as common.UIAbilityContext;
    this.loadSettings();
  }

  aboutToDisappear(): void {
    this.monitor.stop();
  }

  build() {
    Column() {
      this.header()
      Scroll() {
        Column() {
          if (this.baseUrl.length === 0 || this.showSettings) {
            this.settingsPanel()
          } else {
            this.statusPanel()
          }
        }
        .width('100%')
        .padding({ left: 16, right: 16, bottom: 24 })
      }
      .layoutWeight(1)
      .scrollBar(BarState.Off)
    }
    .width('100%')
    .height('100%')
    .backgroundColor('#FFF5F6F8')
  }

  @Builder
  private header() {
    Row() {
      Column() {
        Text('Agent Buddy')
          .fontSize(24)
          .fontWeight(FontWeight.Bold)
          .fontColor('#FF1F2329')
        Text('Codex status')
          .fontSize(12)
          .fontColor('#FF6B7280')
          .margin({ top: 3 })
      }
      .alignItems(HorizontalAlign.Start)

      Blank()

      Circle()
        .width(18)
        .height(18)
        .fill(lightColor(this.currentLight()))
        .margin({ right: 8 })
      Text(lightLabel(this.currentLight()))
        .fontSize(13)
        .fontColor(lightColor(this.currentLight()))
        .fontWeight(FontWeight.Medium)
    }
    .width('100%')
    .padding({ left: 20, right: 20, top: 20, bottom: 16 })
    .backgroundColor('#FFFFFFFF')
  }

  @Builder
  private settingsPanel() {
    Column() {
      Text('连接 Agent Buddy')
        .fontSize(20)
        .fontWeight(FontWeight.Bold)
        .fontColor('#FF1F2329')
        .alignSelf(ItemAlign.Start)
      Text('输入运行 agent-buddy webserver 的地址。')
        .fontSize(13)
        .fontColor('#FF6B7280')
        .margin({ top: 8, bottom: 18 })
        .alignSelf(ItemAlign.Start)

      TextInput({ text: this.baseUrl, placeholder: 'http://192.168.1.20:8787' })
        .height(48)
        .width('100%')
        .backgroundColor('#FFF5F6F8')
        .borderRadius(12)
        .onChange((value: string) => {
          this.baseUrl = value;
        })

      TextInput({ text: this.serverName, placeholder: '服务器名称' })
        .height(48)
        .width('100%')
        .backgroundColor('#FFF5F6F8')
        .borderRadius(12)
        .margin({ top: 10 })
        .onChange((value: string) => {
          this.serverName = value;
        })

      Row() {
        Text('启用实况窗')
          .fontSize(15)
          .fontColor('#FF1F2329')
        Blank()
        Toggle({ type: ToggleType.Switch, isOn: this.liveWindowEnabled })
          .onChange((value: boolean) => {
            this.liveWindowEnabled = value;
            this.saveSettings();
            if (!value) {
              this.stopLiveWindow();
            } else if (this.connected) {
              this.publishLiveWindow();
            }
          })
      }
      .width('100%')
      .margin({ top: 18, bottom: 18 })

      Button('保存并连接')
        .width('100%')
        .height(48)
        .fontSize(16)
        .fontColor('#FFFFFFFF')
        .backgroundColor('#FF1F2329')
        .borderRadius(12)
        .enabled(!this.busy)
        .onClick(() => {
          this.saveAndConnect();
        })

      if (this.errorMessage.length > 0) {
        Text(this.errorMessage)
          .fontSize(13)
          .fontColor(lightColor(LightState.RED))
          .margin({ top: 14 })
      }
    }
    .width('100%')
    .padding(20)
    .margin({ top: 16 })
    .backgroundColor('#FFFFFFFF')
    .borderRadius(16)
  }

  @Builder
  private statusPanel() {
    Column() {
      Row() {
        Column() {
          Text(this.serverName)
            .fontSize(18)
            .fontWeight(FontWeight.Bold)
            .fontColor('#FF1F2329')
            .alignSelf(ItemAlign.Start)
          Text(this.baseUrl)
            .fontSize(12)
            .fontColor('#FF6B7280')
            .margin({ top: 4 })
            .alignSelf(ItemAlign.Start)
        }
        .alignItems(HorizontalAlign.Start)
        Blank()
        Button('设置')
          .height(34)
          .fontSize(13)
          .fontColor('#FF1F2329')
          .backgroundColor('#FFF5F6F8')
          .borderRadius(17)
          .onClick(() => {
            this.showSettings = true;
          })
      }
      .width('100%')

      this.aggregateStatus()

      if (this.snapshot.sessions.length === 0) {
        Column() {
          Circle()
            .width(46)
            .height(46)
            .fill(lightColor(this.currentLight()))
          Text(this.connected ? '没有活动会话' : '连接失败')
            .fontSize(16)
            .fontColor('#FF1F2329')
            .margin({ top: 12 })
          Text(this.connected ? '等待 Codex 开始工作' : this.errorMessage)
            .fontSize(13)
            .fontColor('#FF6B7280')
            .margin({ top: 6 })
            .maxLines(2)
        }
        .width('100%')
        .padding({ top: 36, bottom: 36 })
      } else {
        ForEach(this.snapshot.sessions, (session: CodexSessionSummary) => {
          this.sessionRow(session)
        }, (session: CodexSessionSummary) => session.sessionId)
      }

      Row() {
        Button('刷新')
          .layoutWeight(1)
          .height(44)
          .fontColor('#FF1F2329')
          .backgroundColor('#FFF5F6F8')
          .borderRadius(12)
          .onClick(() => {
            this.refreshNow();
          })
        Button(this.liveWindowActive ? '停止实况窗' : '启用实况窗')
          .layoutWeight(1)
          .height(44)
          .fontColor('#FFFFFFFF')
          .backgroundColor(lightColor(this.currentLight()))
          .borderRadius(12)
          .margin({ left: 10 })
          .onClick(() => {
            if (this.liveWindowActive) {
              this.stopLiveWindow();
            } else {
              this.liveWindowEnabled = true;
              this.publishLiveWindow();
            }
          })
      }
      .width('100%')
      .margin({ top: 8, bottom: 12 })

      if (this.errorMessage.length > 0) {
        Text(this.errorMessage)
          .fontSize(13)
          .fontColor(lightColor(LightState.RED))
          .margin({ top: 4, bottom: 8 })
      }
    }
    .width('100%')
    .padding(20)
    .margin({ top: 16 })
    .backgroundColor('#FFFFFFFF')
    .borderRadius(16)
  }

  @Builder
  private aggregateStatus() {
    const state: LightState = this.currentLight();
    Row() {
      Circle()
        .width(58)
        .height(58)
        .fill(lightColor(state))
        .margin({ right: 14 })
      Column() {
        Text(lightLabel(state))
          .fontSize(19)
          .fontWeight(FontWeight.Bold)
          .fontColor(lightColor(state))
          .alignSelf(ItemAlign.Start)
        Text(`${stateLabel(this.snapshot.overallState)} · ${this.snapshot.sessionsCount} 个会话`)
          .fontSize(13)
          .fontColor('#FF6B7280')
          .margin({ top: 4 })
          .alignSelf(ItemAlign.Start)
        Text(firstSessionSummary(this.snapshot))
          .fontSize(13)
          .fontColor('#FF1F2329')
          .margin({ top: 5 })
          .maxLines(2)
          .alignSelf(ItemAlign.Start)
      }
      .alignItems(HorizontalAlign.Start)
    }
    .width('100%')
    .padding({ top: 22, bottom: 22 })
    .margin({ top: 18 })
  }

  @Builder
  private sessionRow(session: CodexSessionSummary) {
    const state: LightState = sessionLightState(session);
    Column() {
      Row() {
        Circle()
          .width(14)
          .height(14)
          .fill(lightColor(state))
          .margin({ right: 10 })
        Text(sessionTitle(session))
          .fontSize(16)
          .fontWeight(FontWeight.Medium)
          .fontColor('#FF1F2329')
          .maxLines(1)
        Blank()
        Text(lightLabel(state))
          .fontSize(12)
          .fontColor(lightColor(state))
      }
      .width('100%')
      Text(sessionSummary(session))
        .fontSize(14)
        .fontColor('#FF4B5563')
        .margin({ top: 10 })
        .maxLines(3)
        .alignSelf(ItemAlign.Start)
      if (session.canContinue && session.continueAction !== undefined) {
        Button(session.continueAction.label || '继续')
          .height(36)
          .fontSize(13)
          .fontColor('#FFFFFFFF')
          .backgroundColor(lightColor(LightState.YELLOW))
          .borderRadius(18)
          .margin({ top: 12 })
          .onClick(() => {
            this.continueSession(session);
          })
      }
    }
    .width('100%')
    .padding({ top: 16, bottom: 16 })
    .border({ width: { bottom: 1 }, color: '#FFE9EBEF' })
  }

  private currentLight(): LightState {
    if (this.baseUrl.trim().length === 0) {
      return LightState.OFF;
    }
    return statusLight(this.snapshot, this.connected);
  }

  private async loadSettings(): Promise<void> {
    if (this.context === undefined) {
      return;
    }
    const settings: AgentBuddySettings = await SettingsStore.load(this.context);
    this.serverName = settings.name;
    this.baseUrl = settings.baseUrl;
    this.liveWindowEnabled = settings.liveWindowEnabled;
    this.showSettings = this.baseUrl.length === 0;
    if (this.baseUrl.length > 0) {
      this.startMonitor();
    }
  }

  private startMonitor(): void {
    this.monitor.start(this.baseUrl, (snapshot: CodexStatusSnapshot, connected: boolean, error: string): void => {
      this.snapshot = snapshot;
      this.connected = connected;
      this.errorMessage = error;
      if (this.liveWindowEnabled && this.baseUrl.length > 0) {
        this.publishLiveWindow();
      }
    });
  }

  private saveAndConnect(): void {
    const normalized: string = this.baseUrl.trim().replace(/\/$/, '');
    if (!(normalized.startsWith('http://') || normalized.startsWith('https://'))) {
      this.errorMessage = '请输入 http:// 或 https:// 开头的服务器地址';
      return;
    }
    this.baseUrl = normalized;
    this.errorMessage = '';
    this.showSettings = false;
    this.saveSettings();
    this.startMonitor();
  }

  private saveSettings(): void {
    if (this.context === undefined) {
      return;
    }
    const settings: AgentBuddySettings = {
      name: this.serverName.trim() || 'Agent Buddy',
      baseUrl: this.baseUrl.trim().replace(/\/$/, ''),
      liveWindowEnabled: this.liveWindowEnabled
    };
    SettingsStore.save(this.context, settings).catch((error: Object): void => {
      this.errorMessage = `${error}`;
    });
  }

  private refreshNow(): void {
    this.busy = true;
    this.monitor.refreshNow().finally((): void => {
      this.busy = false;
    });
  }

  private continueSession(session: CodexSessionSummary): void {
    if (this.baseUrl.length === 0) {
      return;
    }
    this.busy = true;
    this.api.continueSession(this.baseUrl, session).then((snapshot: CodexStatusSnapshot): void => {
      this.snapshot = snapshot;
      this.connected = true;
      this.errorMessage = '';
      if (this.liveWindowEnabled) {
        this.publishLiveWindow();
      }
    }).catch((error: Object): void => {
      this.connected = false;
      this.errorMessage = `${error}`;
      this.publishLiveWindow();
    }).finally((): void => {
      this.busy = false;
    });
  }

  private publishLiveWindow(): void {
    if (!this.liveWindowEnabled || this.baseUrl.length === 0) {
      return;
    }
    this.liveWindow.publish(this.snapshot, this.connected).then((active: boolean): void => {
      this.liveWindowActive = active;
    });
  }

  private stopLiveWindow(): void {
    this.liveWindow.stop().then((): void => {
      this.liveWindowActive = false;
    });
  }
}

function sessionTitle(session: CodexSessionSummary): string {
  return session.compactTitle || session.displayTitle || session.shortSessionId || session.sessionId;
}

function sessionSummary(session: CodexSessionSummary): string {
  if (session.needsOpen && session.openSummary.length > 0) {
    return session.openSummary;
  }
  return session.compactSummary || session.summary || session.stateDetail || stateLabel(session.state);
}
