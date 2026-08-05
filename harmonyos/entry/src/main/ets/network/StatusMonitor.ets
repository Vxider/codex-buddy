import { CodexStatusSnapshot } from '../common/CodexModels';
import { AgentBuddyApi, offlineStatus } from './AgentBuddyApi';

export type StatusUpdateHandler = (snapshot: CodexStatusSnapshot, connected: boolean, error: string) => void;

export class StatusMonitor {
  private static readonly POLL_INTERVAL_MS: number = 3000;
  private readonly api: AgentBuddyApi;
  private timer: number = -1;
  private baseUrl: string = '';
  private handler?: StatusUpdateHandler;
  private refreshing: boolean = false;

  constructor(api: AgentBuddyApi = new AgentBuddyApi()) {
    this.api = api;
  }

  start(baseUrl: string, handler: StatusUpdateHandler): void {
    this.stop();
    this.baseUrl = baseUrl.trim().replace(/\/$/, '');
    this.handler = handler;
    if (this.baseUrl.length === 0) {
      return;
    }
    this.refresh();
    this.timer = setInterval((): void => {
      this.refresh();
    }, StatusMonitor.POLL_INTERVAL_MS);
  }

  stop(): void {
    if (this.timer >= 0) {
      clearInterval(this.timer);
    }
    this.timer = -1;
    this.handler = undefined;
    this.baseUrl = '';
    this.refreshing = false;
  }

  async refreshNow(): Promise<void> {
    await this.refresh();
  }

  private async refresh(): Promise<void> {
    if (this.refreshing || this.baseUrl.length === 0 || this.handler === undefined) {
      return;
    }
    this.refreshing = true;
    const currentHandler: StatusUpdateHandler = this.handler;
    const currentBaseUrl: string = this.baseUrl;
    try {
      const snapshot: CodexStatusSnapshot = await this.api.loadStatus(currentBaseUrl);
      currentHandler(snapshot, true, '');
    } catch (error) {
      currentHandler(offlineStatus(), false, errorMessage(error));
    } finally {
      this.refreshing = false;
    }
  }
}

function errorMessage(error: Object): string {
  if (error instanceof Error) {
    return error.message;
  }
  return `${error}`;
}
