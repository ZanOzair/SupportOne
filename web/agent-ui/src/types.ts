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

/** What a fix says it will do, before it does any of it. */
export interface Explanation {
  summary: string;
  changes: string[];
  undo: string;
}

export interface FixSummary {
  id: string;
  explanation: Explanation;
  requires_admin: boolean;
  reversible: boolean;
}

export interface RestoreAvailability {
  available: boolean;
  kind: string;
  reason?: string;
}

/** The plan the user is shown, and the token that proves they were shown it. */
export interface Plan {
  fix_id: string;
  explanation: Explanation;
  requires_admin: boolean;
  elevated: boolean;
  reversible: boolean;
  restore: RestoreAvailability;
  blocked?: string;
  token: string;
  dry_run: boolean;
}

export interface Confirmation {
  token: string;
  acknowledged: string[];
  accept_without_restore_point: boolean;
}

export interface Outcome {
  fix_id: string;
  applied: boolean;
  dry_run: boolean;
  detail?: string;
  detail_args?: unknown[];
  started_at: string;
  duration_ns: number;
}

export interface ApplyResult {
  outcome: Outcome;
  restore_point?: { kind: string; reference?: string; label: string; created: string };
  reversible: boolean;
}

/** A plan is offerable only when nothing blocks it and the rights are held. */
export function applicable(plan: Plan): boolean {
  return !plan.blocked && (!plan.requires_admin || plan.elevated);
}

export interface WizardSummary {
  id: string;
  title: string;
  complaint: string;
  steps: number;
}

export type StepStatus =
  | 'clean'
  | 'found'
  | 'fixed'
  | 'applied'
  | 'no_help'
  | 'declined'
  | 'blocked'
  | 'unknown';

export type WizardOutcome =
  | 'running'
  | 'fixed'
  | 'unresolved'
  | 'unverified'
  | 'no_fault'
  | 'stopped';

export interface Finding {
  ok: boolean;
  summary: string;
  summary_args?: unknown[];
  unknown: boolean;
}

export interface StepRecord {
  step_id: string;
  title: string;
  status: StepStatus;
  finding: Finding;
  fix_id?: string;
  error?: string;
}

export interface WizardProgress {
  wizard_id: string;
  step?: StepRecord;
  offer?: Plan;
  advice?: string;
  outcome: WizardOutcome;
  done: StepRecord[];
}

export interface WizardSession {
  session_id: string;
  progress: WizardProgress;
}

/** What SupportOne says about one finding, from the table built into the binary. */
export interface Advice {
  check_id: string;
  severity: Severity;
  cause: string;
  steps?: string[];
  fixes?: string[];
  wizards?: string[];
  escalate: boolean;
}

/** Whether the optional second tier is switched on at all. */
export interface AssistState {
  enabled: boolean;
  endpoint?: string;
  pending: number;
}

/** Exactly what would leave this computer, and nothing that is not in it will. */
export interface AssistPayload {
  endpoint: string;
  model: string;
  host: string;
  body: string;
  bytes: number;
  redacted: boolean;
  token: string;
}

export interface AssistAnswer {
  notes: string;
  fixes?: string[];
  discarded: number;
  model?: string;
}
