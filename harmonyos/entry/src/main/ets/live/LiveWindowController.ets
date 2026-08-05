import { Want, wantAgent } from '@kit.AbilityKit';
import { liveViewManager } from '@kit.LiveViewKit';
import {
  CodexStatusSnapshot,
  CodexState,
  LightState,
  firstSessionSummary,
  lightColor,
  lightLabel,
  statusLight
} from '../common/CodexModels';

export class LiveWindowController {
  private static readonly LIVE_VIEW_ID: number = 7020508;
  private sequence: number = 0;
  private active: boolean = false;
  private lastKey: string = '';

  async publish(snapshot: CodexStatusSnapshot, connected: boolean): Promise<boolean> {
    const state: LightState = statusLight(snapshot, connected);
    const summary: string = compactText(firstSessionSummary(snapshot), 72);
    const key: string = `${state}|${snapshot.overallState}|${snapshot.overallStateDetail}|${summary}`;
    if (key === this.lastKey && this.active) {
      return true;
    }

    try {
      if (!await liveViewManager.isLiveViewEnabled()) {
        return false;
      }
      if (!this.active) {
        try {
          const activeView: liveViewManager.LiveView = await liveViewManager.getActiveLiveView(
            LiveWindowController.LIVE_VIEW_ID
          );
          if (activeView.sequence >= 0 && activeView.sequence < 2147483647) {
            this.sequence = activeView.sequence;
          }
          this.active = true;
        } catch (_) {
          this.active = false;
        }
      }

      const nextSequence: number = this.sequence + 1;
      const liveView: liveViewManager.LiveView = await this.buildLiveView(
        snapshot,
        connected,
        state,
        summary,
        nextSequence
      );
      if (this.active) {
        try {
          await liveViewManager.updateLiveView(liveView);
        } catch (_) {
          this.active = false;
          await liveViewManager.startLiveView(liveView);
        }
      } else {
        await liveViewManager.startLiveView(liveView);
        this.active = true;
      }
      this.sequence = nextSequence;
      this.lastKey = key;
      return true;
    } catch (_) {
      return false;
    }
  }

  async stop(): Promise<boolean> {
    if (!this.active) {
      return true;
    }
    try {
      if (!await liveViewManager.isLiveViewEnabled()) {
        return false;
      }
      const view: liveViewManager.LiveView = await this.buildLiveView(
        {
          serverTime: '',
          overallState: CodexState.IDLE,
          overallStateDetail: 'stopped',
          activeSessionId: '',
          sessionsCount: 0,
          sessions: []
        },
        true,
        LightState.OFF,
        'Agent Buddy',
        this.sequence + 1,
        true
      );
      await liveViewManager.stopLiveView(view);
      this.active = false;
      this.lastKey = '';
      return true;
    } catch (_) {
      this.active = false;
      return false;
    }
  }

  isActive(): boolean {
    return this.active;
  }

  private async buildLiveView(
    snapshot: CodexStatusSnapshot,
    connected: boolean,
    state: LightState,
    summary: string,
    sequence: number,
    stopping: boolean = false
  ): Promise<liveViewManager.LiveView> {
    const color: string = lightColor(state);
    const title: string = connected ? lightLabel(state) : 'Codex 异常中断';
    const progress: number = progressFor(state);
    const content: Array<liveViewManager.RichText> = [
      { text: '● ', textColor: color },
      { text: title },
      { text: ` · ${summary}` }
    ];
    const action: Want = await this.buildWantAgent();
    return {
      id: LiveWindowController.LIVE_VIEW_ID,
      event: 'TIMER',
      sequence,
      isMute: true,
      liveViewData: {
        primary: {
          title: 'Agent Buddy',
          content,
          keepTime: 15,
          clickAction: action,
          layoutData: {
            layoutType: liveViewManager.LayoutType.LAYOUT_TYPE_PROGRESS,
            progress,
            color,
            backgroundColor: '#FF252932',
            indicatorType: liveViewManager.IndicatorType.INDICATOR_TYPE_OVERLAY,
            indicatorIcon: iconFor(state),
            lineType: liveViewManager.LineType.LINE_TYPE_NORMAL_SOLID_LINE
          }
        },
        capsule: {
          type: liveViewManager.CapsuleType.CAPSULE_TYPE_TEXT,
          status: stopping ? -1 : 1,
          icon: iconFor(state),
          title,
          content: summary,
          backgroundColor: color
        }
      }
    };
  }

  private async buildWantAgent(): Promise<Want> {
    const info: wantAgent.WantAgentInfo = {
      wants: [
        {
          bundleName: 'com.vxider.agentbuddy.next',
          abilityName: 'EntryAbility'
        } as Want
      ],
      actionType: wantAgent.OperationType.START_ABILITIES,
      requestCode: 7020508,
      actionFlags: [wantAgent.WantAgentFlags.UPDATE_PRESENT_FLAG]
    };
    return await wantAgent.getWantAgent(info) as Want;
  }
}

function progressFor(state: LightState): number {
  switch (state) {
    case LightState.RED:
      return 100;
    case LightState.YELLOW:
      return 50;
    case LightState.PURPLE:
      return 75;
    case LightState.GREEN:
      return 60;
    default:
      return 0;
  }
}

function iconFor(state: LightState): string {
  switch (state) {
    case LightState.RED:
      return 'status_light_red.svg';
    case LightState.YELLOW:
      return 'status_light_yellow.svg';
    case LightState.PURPLE:
      return 'status_light_purple.svg';
    case LightState.GREEN:
      return 'status_light_green.svg';
    default:
      return 'status_light_off.svg';
  }
}

function compactText(value: string, limit: number): string {
  const text: string = value.replace(/\s+/g, ' ').trim();
  return text.length > limit ? `${text.substring(0, limit - 1)}…` : text;
}
