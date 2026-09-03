export type Severity = 'ok' | 'attention' | 'urgent' | 'unknown';

export interface CheckResult {
  check_id: string;
  severity: Severity;
  summary: string;
  summary_args?: unknown[];
  detail?: Record<string, unknown>;
  error?: string;
  started_at: string;
  duration_ns: number;
}

export interface Snapshot {
  schema: number;
  agent_version: string;
  generated_at: string;
  host: { os: string; arch: string };
  results: CheckResult[];
  skipped_needs_admin?: string[];
}

export interface Session {
  version: string;
  os: string;
  arch: string;
  lang: string;
  languages: string[];
  audit_path: string;
}

/** What the user chooses to strip before a report leaves this computer. */
export interface RedactionPolicy {
  hostnames: boolean;
  usernames: boolean;
  serials: boolean;
  addresses: boolean;
}

export const noRedaction: RedactionPolicy = {
  hostnames: false,
  usernames: false,
  serials: false,
  addresses: false,
};

/** The most protective starting point, which the user can then relax. */
export const fullRedaction: RedactionPolicy = {
  hostnames: true,
  usernames: true,
  serials: true,
  addresses: true,
};

export function redacts(policy: RedactionPolicy): boolean {
  return Object.values(policy).some(Boolean);
}
