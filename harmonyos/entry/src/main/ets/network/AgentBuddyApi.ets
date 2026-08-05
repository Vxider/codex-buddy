import {
  CodexSessionSummary,
  CodexState,
  CodexStatusSnapshot,
  ContinueAction,
  EMPTY_STATUS,
  normalizeState
} from '../common/CodexModels';
import { http } from '@kit.NetworkKit';

export class AgentBuddyApi {
  async loadStatus(baseUrl: string): Promise<CodexStatusSnapshot> {
    const responseText: string = await this.request(baseUrl, '/v1/status', http.RequestMethod.GET);
    return parseStatus(responseText);
  }

  async continueSession(baseUrl: string, session: CodexSessionSummary): Promise<CodexStatusSnapshot> {
    if (session.continueAction === undefined) {
      throw new Error('该会话没有可执行的继续操作');
    }
    const action: ContinueAction = session.continueAction;
    const body: string = JSON.stringify({ action_token: action.actionToken });
    const responseText: string = await this.request(
      baseUrl,
      action.endpoint,
      action.method || http.RequestMethod.POST,
      body
    );
    const response: CodexContinueResponse = parseContinueResponse(responseText);
    return response.status === undefined ? await this.loadStatus(baseUrl) : response.status;
  }

  private async request(baseUrl: string, endpoint: string, method: string, body: string = ''): Promise<string> {
    const request = http.createHttp();
    try {
      const headers: Record<string, string> = {
        'Accept': 'application/json'
      };
      if (body.length > 0) {
        headers['Content-Type'] = 'application/json';
      }
      const response = await request.request(resolveEndpoint(baseUrl, endpoint), {
        method: method === 'POST' ? http.RequestMethod.POST : http.RequestMethod.GET,
        header: headers,
        extraData: body,
        expectDataType: http.HttpDataType.STRING,
        connectTimeout: 5000,
        readTimeout: 8000
      });
      const responseCode: number = response.responseCode;
      const responseText: string = response.result as string;
      if (responseCode < 200 || responseCode >= 300) {
        throw new Error(`Agent Buddy HTTP ${responseCode}: ${responseText}`);
      }
      return responseText;
    } finally {
      request.destroy();
    }
  }
}

interface JsonObject {
  [key: string]: Object;
}

export interface CodexContinueResponse {
  ok: boolean;
  message: string;
  session?: CodexSessionSummary;
  status?: CodexStatusSnapshot;
}

function resolveEndpoint(baseUrl: string, endpoint: string): string {
  if (endpoint.startsWith('http://') || endpoint.startsWith('https://')) {
    return endpoint;
  }
  const base: string = baseUrl.trim().replace(/\/$/, '');
  const path: string = endpoint.startsWith('/') ? endpoint : `/${endpoint}`;
  return `${base}${path}`;
}

function parseStatus(payload: string): CodexStatusSnapshot {
  const root: JsonObject = JSON.parse(payload) as JsonObject;
  const rawSessions: Array<Object> = (root['sessions'] as Array<Object> | undefined) ?? [];
  const sessions: CodexSessionSummary[] = rawSessions.map((item: Object): CodexSessionSummary => parseSession(item));
  return {
    serverTime: stringValue(root, 'server_time'),
    overallState: normalizeState(stringValue(root, 'overall_state')),
    overallStateDetail: stringValue(root, 'overall_state_detail'),
    activeSessionId: stringValue(root, 'active_session_id'),
    sessionsCount: numberValue(root, 'sessions_count', sessions.length),
    sessions
  };
}

function parseContinueResponse(payload: string): CodexContinueResponse {
  const root: JsonObject = JSON.parse(payload) as JsonObject;
  const result: CodexContinueResponse = {
    ok: booleanValue(root, 'ok', true),
    message: stringValue(root, 'message')
  };
  const rawStatus: Object | undefined = root['status'];
  if (rawStatus !== undefined && rawStatus !== null) {
    result.status = parseStatus(JSON.stringify(rawStatus));
  }
  const rawSession: Object | undefined = root['session'];
  if (rawSession !== undefined && rawSession !== null) {
    result.session = parseSession(rawSession);
  }
  return result;
}

function parseSession(value: Object): CodexSessionSummary {
  const item: JsonObject = value as JsonObject;
  const rawAction: Object | undefined = item['continue_action'];
  let action: ContinueAction | undefined;
  if (rawAction !== undefined && rawAction !== null) {
    const actionObject: JsonObject = rawAction as JsonObject;
    action = {
      method: stringValue(actionObject, 'method'),
      endpoint: stringValue(actionObject, 'endpoint'),
      actionToken: stringValue(actionObject, 'action_token'),
      label: stringValue(actionObject, 'label') || '继续'
    };
  }
  return {
    sessionId: stringValue(item, 'session_id'),
    shortSessionId: stringValue(item, 'short_session_id'),
    displayTitle: stringValue(item, 'display_title'),
    compactTitle: stringValue(item, 'compact_title'),
    microTitle: stringValue(item, 'micro_title'),
    state: normalizeState(stringValue(item, 'state')),
    stateDetail: stringValue(item, 'state_detail'),
    updatedAt: stringValue(item, 'updated_at'),
    summary: stringValue(item, 'summary'),
    compactSummary: stringValue(item, 'compact_summary'),
    microSummary: stringValue(item, 'micro_summary'),
    needsOpen: booleanValue(item, 'needs_open', false),
    needsApproval: booleanValue(item, 'needs_approval', false),
    openReason: stringValue(item, 'open_reason'),
    openSummary: stringValue(item, 'open_summary'),
    compactOpenSummary: stringValue(item, 'compact_open_summary'),
    microOpenSummary: stringValue(item, 'micro_open_summary'),
    canContinue: booleanValue(item, 'can_continue', action !== undefined),
    continueAction: action,
    goalState: stringValue(item, 'goal_state')
  };
}

function stringValue(item: JsonObject, key: string): string {
  const value: Object | undefined = item[key];
  if (value === undefined || value === null) {
    return '';
  }
  return `${value}`;
}

function numberValue(item: JsonObject, key: string, fallback: number): number {
  const value: Object | undefined = item[key];
  if (value === undefined || value === null) {
    return fallback;
  }
  const parsed: number = Number(value);
  return Number.isNaN(parsed) ? fallback : parsed;
}

function booleanValue(item: JsonObject, key: string, fallback: boolean): boolean {
  const value: Object | undefined = item[key];
  if (value === undefined || value === null) {
    return fallback;
  }
  if (value === true || value === 'true') {
    return true;
  }
  if (value === false || value === 'false') {
    return false;
  }
  return fallback;
}

export function offlineStatus(): CodexStatusSnapshot {
  return {
    serverTime: '',
    overallState: EMPTY_STATUS.overallState,
    overallStateDetail: 'network_error',
    activeSessionId: '',
    sessionsCount: 0,
    sessions: []
  };
}
