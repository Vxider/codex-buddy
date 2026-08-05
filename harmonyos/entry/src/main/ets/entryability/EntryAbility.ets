import { AbilityConstant, UIAbility, Want, window } from '@kit.AbilityKit';
import { hilog } from '@kit.PerformanceAnalysisKit';

export default class EntryAbility extends UIAbility {
  onCreate(_want: Want, _launchParam: AbilityConstant.LaunchParam): void {
    hilog.info(0x0000, 'AgentBuddy', 'EntryAbility created');
  }

  onWindowStageCreate(windowStage: window.WindowStage): void {
    windowStage.loadContent('pages/Index', (error?: object) => {
      if (error !== undefined) {
        hilog.error(0x0000, 'AgentBuddy', 'Failed to load page: %{public}s', `${error}`);
      }
    });
  }

  onWindowStageDestroy(): void {
    hilog.info(0x0000, 'AgentBuddy', 'Window stage destroyed');
  }

  onForeground(): void {
    hilog.info(0x0000, 'AgentBuddy', 'EntryAbility foreground');
  }

  onBackground(): void {
    hilog.info(0x0000, 'AgentBuddy', 'EntryAbility background');
  }
}
