import type {
  Advice,
  ApplyResult,
  AssistAnswer,
  AssistPayload,
  AssistState,
  Confirmation,
  FixSummary,
  Plan,
  RedactionPolicy,
  RemotePlan,
  RemoteSession,
  RemoteState,
  Session,
  Snapshot,
  WizardSession,
  WizardSummary,
} from './types';

/**
 * The session token arrives once, in the URL the agent opened. It is kept in
 * memory for the life of the page and removed from the address bar, so it does
 * not end up in browser history, in a bookmark, or in a screenshot of the
 * address bar sent to support.
 */
const token = (() => {
  const params = new URLSearchParams(window.location.search);
  const value = params.get('t') ?? '';
  if (value) {
    params.delete('t');
    const query = params.toString();
    window.history.replaceState({}, '', window.location.pathname + (query ? `?${query}` : ''));
  }
  return value;
})();

export const hasToken = token !== '';

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    ...init,
    headers: {
      ...(init?.headers ?? {}),
      Authorization: `Bearer ${token}`,
    },
  });
  if (!response.ok) {
    throw new Error(`${response.status} ${await response.text()}`);
  }
  return (await response.json()) as T;
}

export function fetchSession(): Promise<Session> {
  return request<Session>('/api/session');
}

export function fetchMessages(
  lang: string,
): Promise<{ lang: string; messages: Record<string, string> }> {
  return request(`/api/messages?lang=${encodeURIComponent(lang)}`);
}

export function fetchSnapshot(): Promise<Snapshot> {
  return request<Snapshot>('/api/snapshot');
}

export function runChecks(): Promise<Snapshot> {
  return request<Snapshot>('/api/snapshot', { method: 'POST' });
}

/** Returns exactly what the chosen policy would leave in the report. */
export function previewRedaction(policy: RedactionPolicy): Promise<Snapshot> {
  return request<Snapshot>('/api/preview', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(policy),
  });
}

export function reportURL(format: 'html' | 'json', policy: RedactionPolicy, lang: string): string {
  const params = new URLSearchParams({ format, lang, t: token });
  for (const [key, value] of Object.entries(policy)) {
    if (value) {
      params.set(key, '1');
    }
  }
  return `/api/report?${params.toString()}`;
}

export function closeSession(): Promise<{ status: string }> {
  return request('/api/close', { method: 'POST' });
}

/**
 * Tells the agent to stop, from a page that is being closed.
 *
 * A window closing is the last thing that happens on it: fetch is cancelled
 * with the page, so the request never leaves. sendBeacon is the browser's
 * answer to that, and it is why this cannot reuse `closeSession` — a beacon
 * carries no headers, so the token goes in the query string instead, the same
 * place the agent already accepts it from. Nothing is awaited because there is
 * nothing left to await it.
 */
export function closeSessionOnUnload(): void {
  if (!token || typeof navigator.sendBeacon !== 'function') {
    return;
  }
  navigator.sendBeacon(`/api/close?t=${encodeURIComponent(token)}`);
}

function post<T>(path: string, body: unknown): Promise<T> {
  return request<T>(path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
}

export function fetchFixes(): Promise<FixSummary[]> {
  return request<FixSummary[]>('/api/fixes');
}

/** Describes a repair. This is read-only: nothing changes until apply. */
export function planFix(fixID: string): Promise<Plan> {
  return post<Plan>('/api/fixes/plan', { fix_id: fixID });
}

export function applyFix(confirmation: Confirmation): Promise<ApplyResult> {
  return post<ApplyResult>('/api/fixes/apply', confirmation);
}

export function rollbackFix(fixID: string): Promise<{ status: string }> {
  return post('/api/fixes/rollback', { fix_id: fixID });
}

export function fetchWizards(): Promise<WizardSummary[]> {
  return request<WizardSummary[]>('/api/wizards');
}

export function startWizard(wizardID: string): Promise<WizardSession> {
  return post<WizardSession>('/api/wizards/start', { wizard_id: wizardID });
}

export function wizardMove(
  move: 'next' | 'skip' | 'stop',
  sessionID: string,
): Promise<WizardSession> {
  return post<WizardSession>(`/api/wizards/${move}`, { session_id: sessionID });
}

export function wizardConfirm(
  sessionID: string,
  confirmation: Confirmation,
): Promise<WizardSession> {
  return post<WizardSession>('/api/wizards/confirm', {
    session_id: sessionID,
    confirmation,
  });
}

/** The offline explanation of the current snapshot, worst first. */
export function fetchAdvice(): Promise<Advice[]> {
  return request<Advice[]>('/api/explain');
}

export function fetchAssistState(): Promise<AssistState> {
  return request<AssistState>('/api/assist');
}

/**
 * Builds the exact bytes that would leave this computer under the chosen
 * redaction, and sends none of them. What the user confirms is the payload,
 * not a description of it.
 */
export function prepareAssist(policy: RedactionPolicy): Promise<AssistPayload> {
  return post<AssistPayload>('/api/assist/prepare', policy);
}

export function askAssist(token: string): Promise<AssistAnswer> {
  return post<AssistAnswer>('/api/assist/ask', { token });
}

export function discardAssist(token: string): Promise<{ status: string }> {
  return post('/api/assist/discard', { token });
}

/**
 * Remote help wraps a program the user already has. SupportOne implements no
 * remote desktop protocol, starts only a program from its compiled-in list,
 * and can see nothing once a session begins.
 */
export function fetchRemoteState(): Promise<RemoteState> {
  return request<RemoteState>('/api/remote');
}

/** Describes a session before it starts. Nothing is launched by this. */
export function planRemote(technician: string, toolID: string): Promise<RemotePlan> {
  return post<RemotePlan>('/api/remote/plan', { technician, tool_id: toolID });
}

/**
 * Records the agreement and starts the tool.
 *
 * The acknowledged list is the one the panel displayed, sent back unedited: a
 * confirmation that does not repeat the plan is refused by the agent, which is
 * what makes displaying it the condition of starting.
 */
export function startRemote(token: string, acknowledged: string[]): Promise<RemoteSession> {
  return post<RemoteSession>('/api/remote/start', { token, acknowledged });
}

/** Records that the user read the plan and said no, and discards the plan. */
export function declineRemote(): Promise<{ status: string }> {
  return post('/api/remote/decline', {});
}

/** Closes the record. It does not close the connection; nothing here can. */
export function endRemote(): Promise<RemoteSession> {
  return post<RemoteSession>('/api/remote/end', {});
}
