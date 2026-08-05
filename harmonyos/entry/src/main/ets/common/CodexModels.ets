export enum CodexState {
  OFFLINE = 'offline',
  IDLE = 'idle',
  RUN = 'run',
  RUNNING = 'running',
  RUNNING_BASH = 'running_bash',
  OPEN = 'open',
  ERROR = 'error'
}

export enum LightState {
  OFF = 'off',
  GREEN = 'green',
  YELLOW = 'yellow',
  RED = 'red',
  PURPLE = 'purple'
}

export interface ContinueAction {
  method: string;
  endpoint: string;
  actionToken: string;
  label: string;
}

export interface CodexSessionSummary {
  sessionId: string;
  shortSessionId: string;
  displayTitle: string;
  compactTitle: string;
  microTitle: string;
  state: CodexState;
  stateDetail: string;
  updatedAt: string;
  summary: string;
  compactSummary: string;
  microSummary: string;
  needsOpen: boolean;
  needsApproval: boolean;
  openReason: string;
  openSummary: string;
  compactOpenSummary: string;
  microOpenSummary: string;
  canContinue: boolean;
  continueAction?: ContinueAction;
  goalState: string;
}

export interface CodexStatusSnapshot {
  serverTime: string;
  overallState: CodexState;
  overallStateDetail: string;
  activeSessionId: string;
  sessionsCount: number;
  sessions: CodexSessionSummary[];
}

export const EMPTY_STATUS: CodexStatusSnapshot = {
  serverTime: '',
  overallState: CodexState.OFFLINE,
  overallStateDetail: '',
  activeSessionId: '',
  sessionsCount: 0,
  sessions: []
};

export function normalizeState(value: string): CodexState {
  switch (value) {
    case CodexState.IDLE:
      return CodexState.IDLE;
    case CodexState.RUN:
      return CodexState.RUN;
    case CodexState.RUNNING:
      return CodexState.RUNNING;
    case CodexState.RUNNING_BASH:
      return CodexState.RUNNING_BASH;
    case CodexState.OPEN:
      return CodexState.OPEN;
    case CodexState.ERROR:
      return CodexState.ERROR;
    default:
      return CodexState.OFFLINE;
  }
}

export function normalizedState(value: CodexState): CodexState {
  if (value === CodexState.RUNNING || value === CodexState.RUNNING_BASH) {
    return CodexState.RUN;
  }
  return value;
}

export function stateLabel(value: CodexState): string {
  switch (normalizedState(value)) {
    case CodexState.RUN:
      return 'RUN';
    case CodexState.IDLE:
      return 'IDLE';
    case CodexState.OPEN:
      return 'OPEN';
    case CodexState.ERROR:
      return 'ERROR';
    default:
      return 'OFFLINE';
  }
}

export function lightColor(state: LightState): string {
  switch (state) {
    case LightState.RED:
      return '#FFF04444';
    case LightState.YELLOW:
      return '#FFFFB020';
    case LightState.PURPLE:
      return '#FF9B6CFF';
    case LightState.GREEN:
      return '#FF25C281';
    default:
      return '#FF9AA1AB';
  }
}

export function lightLabel(state: LightState): string {
  switch (state) {
    case LightState.RED:
      return 'Codex 异常中断';
    case LightState.YELLOW:
      return '需要关注';
    case LightState.PURPLE:
      return '目标进行中';
    case LightState.GREEN:
      return '运行正常';
    default:
      return '未连接';
  }
}

export function firstSessionSummary(snapshot: CodexStatusSnapshot): string {
  if (snapshot.sessions.length === 0) {
    return snapshot.overallStateDetail || '等待 Codex 状态';
  }
  const first: CodexSessionSummary = snapshot.sessions[0];
  if (first.needsOpen && first.openSummary.length > 0) {
    return first.openSummary;
  }
  if (first.compactSummary.length > 0) {
    return first.compactSummary;
  }
  if (first.summary.length > 0) {
    return first.summary;
  }
  return first.displayTitle || first.sessionId;
}

export function statusLight(snapshot: CodexStatusSnapshot, connected: boolean): LightState {
  if (!connected) {
    return LightState.RED;
  }
  if (snapshot.overallState === CodexState.ERROR ||
    isCodexInterruptionText(snapshot.overallStateDetail)) {
    return LightState.RED;
  }

  let result: LightState = LightState.GREEN;
  for (const session of snapshot.sessions) {
    const sessionLight: LightState = sessionLightState(session);
    if (lightRank(sessionLight) > lightRank(result)) {
      result = sessionLight;
    }
  }
  if (snapshot.overallState === CodexState.OPEN && lightRank(LightState.YELLOW) > lightRank(result)) {
    result = LightState.YELLOW;
  }
  if (snapshot.overallState === CodexState.OFFLINE && snapshot.sessions.length === 0) {
    return LightState.YELLOW;
  }
  return result;
}

export function sessionLightState(session: CodexSessionSummary): LightState {
  if (session.state === CodexState.ERROR ||
    isCodexInterruptionText(session.stateDetail) ||
    isCodexInterruptionText(session.openSummary, session.summary)) {
    return LightState.RED;
  }
  if (session.needsApproval || session.openReason === 'approval' || session.needsOpen) {
    return LightState.YELLOW;
  }
  if (session.goalState === 'in_progress' || session.goalState === 'achieved' ||
    session.goalState === 'complete' || session.goalState === 'completed' ||
    session.goalState === 'done' || session.goalState === 'success' ||
    session.goalState === 'succeeded') {
    return LightState.PURPLE;
  }
  return LightState.GREEN;
}

function lightRank(state: LightState): number {
  switch (state) {
    case LightState.RED:
      return 50;
    case LightState.YELLOW:
      return 30;
    case LightState.PURPLE:
      return 20;
    case LightState.GREEN:
      return 10;
    default:
      return 0;
  }
}

export function isCodexInterruptionText(...values: string[]): boolean {
  const markers: string[] = [
    'unauthorized',
    'forbidden',
    'authentication failed',
    'authentication error',
    'invalid api key',
    'invalid token',
    'payment required',
    'billing error',
    'billing required',
    'quota exhausted',
    'quota exceeded',
    'quota limit',
    'rate limit',
    'rate_limit',
    'ratelimit',
    'too many requests',
    'usage limit',
    'usage_limit',
    'credits exhausted',
    'credit exhausted',
    'insufficient credits',
    'out of credits',
    'limit reached',
    'token budget exhausted',
    'usage exhausted',
    'network error',
    'network_error',
    'network failure',
    'network connection',
    'network unavailable',
    'connection refused',
    'connection reset',
    'connection closed',
    'connection lost',
    'connection timeout',
    'connect timeout',
    'request timeout',
    'timed out',
    'timeout',
    'dns',
    'socket',
    'tls handshake',
    'transport error',
    'unexpected eof',
    'service unavailable',
    'service_error',
    'service error',
    'internal server error',
    'internal_server_error',
    'server error',
    'gateway timeout',
    'gateway_timeout',
    'bad gateway',
    'bad_gateway',
    'disconnected',
    'server disconnected',
    'codex error',
    'codex interrupted',
    'turn failed',
    'turn aborted',
    'task failed',
    'turn interrupted',
    '认证失败',
    '未授权',
    '额度',
    '配额',
    '用光',
    '用完',
    '超额',
    '限额',
    '余额不足',
    '网络错误',
    '网络中断',
    '连接失败',
    '连接中断',
    '连接断开',
    '超时',
    '服务不可用',
    '请求失败',
    '异常中断'
  ];

  for (const value of values) {
    const text: string = value.toLowerCase().trim();
    if (text.length === 0) {
      continue;
    }
    for (const code of ['401', '402', '403', '429', '500', '502', '503', '504']) {
      const codePattern: RegExp = new RegExp(`(^|\\D)${code}(\\D|$)`);
      if (codePattern.test(text)) {
        return true;
      }
    }
    for (const marker of markers) {
      if (text.includes(marker)) {
        return true;
      }
    }
  }
  return false;
}
