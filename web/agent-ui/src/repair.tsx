import { useState } from 'react';
import type { Translate } from './i18n';
import { applicable, type ApplyResult, type Confirmation, type Plan } from './types';

/**
 * ConsentGate is the browser half of internal/remediate's rule: a change
 * happens only against an acknowledgement that repeats the exact list of
 * changes the user was shown.
 *
 * That is why each line has its own checkbox rather than one "I agree". The
 * server compares what comes back against what it described, so ticking the
 * boxes is not a formality the interface invented — it is the only way to
 * produce a confirmation the gate will accept.
 */
export function ConsentGate({
  plan,
  t,
  busy,
  onApply,
  onCancel,
}: {
  plan: Plan;
  t: Translate;
  busy: boolean;
  onApply: (confirmation: Confirmation) => void;
  onCancel: () => void;
}) {
  const [acknowledged, setAcknowledged] = useState<string[]>([]);
  const [acceptNoRestore, setAcceptNoRestore] = useState(false);

  const allAcknowledged = plan.explanation.changes.every((change) => acknowledged.includes(change));
  const restoreSettled = plan.restore.available || acceptNoRestore;
  const ready = allAcknowledged && restoreSettled && applicable(plan) && !plan.dry_run;

  const toggle = (change: string, on: boolean) =>
    setAcknowledged((current) =>
      on ? [...current, change] : current.filter((entry) => entry !== change),
    );

  return (
    <div className="mt-3 rounded-md border border-amber-300 bg-amber-50 p-4 dark:border-amber-800 dark:bg-amber-950/40">
      <p className="font-medium">{t(plan.explanation.summary)}</p>

      <fieldset className="mt-3">
        <legend className="text-sm font-semibold">{t('ui.fix.changes')}</legend>
        <ul className="mt-2 space-y-2">
          {plan.explanation.changes.map((change) => (
            <li key={change}>
              <label className="flex items-start gap-2 text-sm">
                <input
                  type="checkbox"
                  checked={acknowledged.includes(change)}
                  onChange={(event) => toggle(change, event.target.checked)}
                  className="mt-0.5 size-4 shrink-0 rounded border-slate-400 dark:border-slate-600"
                />
                <span>{t(change)}</span>
              </label>
            </li>
          ))}
        </ul>
      </fieldset>

      <p className="mt-3 text-sm">
        <span className="font-semibold">{t('ui.fix.undo')}</span> {t(plan.explanation.undo)}
      </p>

      {plan.restore.available ? (
        <p className="mt-2 text-sm">{t('ui.fix.restore_available', [plan.restore.kind])}</p>
      ) : (
        <div className="mt-2">
          <p className="text-sm">
            {t('ui.fix.restore_unavailable', [plan.restore.reason ? t(plan.restore.reason) : ''])}
          </p>
          <label className="mt-1 flex items-start gap-2 text-sm">
            <input
              type="checkbox"
              checked={acceptNoRestore}
              onChange={(event) => setAcceptNoRestore(event.target.checked)}
              className="mt-0.5 size-4 shrink-0 rounded border-slate-400 dark:border-slate-600"
            />
            <span>{t('ui.fix.accept_no_restore')}</span>
          </label>
        </div>
      )}

      {plan.blocked && <p className="mt-3 text-sm">{t('ui.fix.blocked', [t(plan.blocked)])}</p>}
      {plan.requires_admin && !plan.elevated && (
        <p className="mt-3 text-sm">{t('ui.fix.needs_admin')}</p>
      )}
      {plan.dry_run && <p className="mt-3 text-sm">{t('agent.dry_run.active')}</p>}

      <div className="mt-4 flex flex-wrap gap-3">
        <button
          type="button"
          disabled={!ready || busy}
          onClick={() =>
            onApply({
              token: plan.token,
              // The list goes back in the order it was shown, which is what
              // the gate compares against.
              acknowledged: plan.explanation.changes,
              accept_without_restore_point: acceptNoRestore,
            })
          }
          className="rounded-md bg-amber-700 px-3 py-1.5 text-sm font-medium text-white hover:bg-amber-800 disabled:opacity-50 dark:bg-amber-600 dark:hover:bg-amber-500"
        >
          {t('ui.fix.apply')}
        </button>
        <button
          type="button"
          onClick={onCancel}
          className="rounded-md border border-slate-300 px-3 py-1.5 text-sm dark:border-slate-700"
        >
          {t('ui.fix.cancel')}
        </button>
      </div>

      {!allAcknowledged && (
        <p className="mt-2 text-sm text-slate-600 dark:text-slate-400">
          {t('ui.fix.acknowledge_first')}
        </p>
      )}
    </div>
  );
}

/** Outcome renders what a change actually did, and offers to undo it. */
export function AppliedOutcome({
  result,
  t,
  busy,
  onUndo,
}: {
  result: ApplyResult;
  t: Translate;
  busy: boolean;
  onUndo: () => void;
}) {
  return (
    <div className="mt-3 rounded-md border border-emerald-300 bg-emerald-50 p-4 text-sm dark:border-emerald-800 dark:bg-emerald-950/40">
      {result.outcome.detail && <p>{t(result.outcome.detail, result.outcome.detail_args)}</p>}
      {result.restore_point && (
        <p className="mt-1">{t('ui.fix.restore_point', [result.restore_point.kind])}</p>
      )}

      {result.reversible && result.outcome.applied && (
        <button
          type="button"
          onClick={onUndo}
          disabled={busy}
          className="mt-3 rounded-md border border-slate-300 px-3 py-1.5 text-sm disabled:opacity-50 dark:border-slate-700"
        >
          {t('ui.fix.undo_now')}
        </button>
      )}
    </div>
  );
}
