import { common } from '@kit.AbilityKit';
import { preferences } from '@kit.ArkData';

export interface AgentBuddySettings {
  name: string;
  baseUrl: string;
  liveWindowEnabled: boolean;
}

const DEFAULT_SETTINGS: AgentBuddySettings = {
  name: 'Agent Buddy',
  baseUrl: '',
  liveWindowEnabled: true
};

export class SettingsStore {
  private static readonly STORE_NAME: string = 'agent_buddy_settings';

  static async load(context: common.UIAbilityContext): Promise<AgentBuddySettings> {
    try {
      const store = await preferences.getPreferences(context, SettingsStore.STORE_NAME);
      const name: string = await store.get('name', DEFAULT_SETTINGS.name) as string;
      const baseUrl: string = await store.get('base_url', DEFAULT_SETTINGS.baseUrl) as string;
      const liveWindowEnabled: boolean = await store.get(
        'live_window_enabled',
        DEFAULT_SETTINGS.liveWindowEnabled
      ) as boolean;
      return {
        name: name || DEFAULT_SETTINGS.name,
        baseUrl: baseUrl || '',
        liveWindowEnabled
      };
    } catch (_) {
      return {
        name: DEFAULT_SETTINGS.name,
        baseUrl: DEFAULT_SETTINGS.baseUrl,
        liveWindowEnabled: DEFAULT_SETTINGS.liveWindowEnabled
      };
    }
  }

  static async save(context: common.UIAbilityContext, settings: AgentBuddySettings): Promise<void> {
    const store = await preferences.getPreferences(context, SettingsStore.STORE_NAME);
    await store.put('name', settings.name.trim() || DEFAULT_SETTINGS.name);
    await store.put('base_url', settings.baseUrl.trim());
    await store.put('live_window_enabled', settings.liveWindowEnabled);
    await store.flush();
  }
}
