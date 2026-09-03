import type { RedactionPolicy, Session, Snapshot } from './types';

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
