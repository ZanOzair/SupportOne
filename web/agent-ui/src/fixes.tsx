import { useCallback, useState } from 'react';
import { applyFix, planFix, rollbackFix } from './api';
import type { Translate } from './i18n';
import { AppliedOutcome, ConsentGate } from './repair';
import type { ApplyResult, Confirmation, FixSummary, Plan } from './types';

/**
 * RepairList shows what this build can change and, for each, the sequence the
 * agent always follows: describe, confirm, apply, offer to undo. Nothing here
 * can name a repair the binary was not built with — the list comes from the
 * registry.
 */
export function RepairList({
  fixes,
  t,
  onError,
}: {
  fixes: FixSummary[];
  t: Translate;
  onError: (message: string) => void;
}) {
  if (fixes.length === 0) {
    return null;
  }

  return (
    <section className="mt-10">
      <h2 className="text-lg font-semibold">{t('ui.fixes.heading')}</h2>
      <p className="mt-1 max-w-prose text-sm text-slate-600 dark:text-slate-400">
        {t('ui.fixes.note')}
      </p>

      <ul className="mt-3 space-y-3">
        {fixes.map((fix) => (
          <RepairCard key={fix.id} fix={fix} t={t} onError={onError} />
        ))}
      </ul>
    </section>
  );
}

function RepairCard({
  fix,
  t,
  onError,
}: {
  fix: FixSummary;
  t: Translate;
  onError: (message: string) => void;
}) {
  const [plan, setPlan] = useState<Plan | null>(null);
  const [result, setResult] = useState<ApplyResult | null>(null);
  const [busy, setBusy] = useState(false);

  const describe = useCallback(() => {
    setBusy(true);
    setResult(null);
    planFix(fix.id)
      .then(setPlan)
      .catch((err: Error) => onError(err.message))
      .finally(() => setBusy(false));
  }, [fix.id, onError]);

  const apply = useCallback(
    (confirmation: Confirmation) => {
      setBusy(true);
      applyFix(confirmation)
        .then((applied) => {
          setResult(applied);
          setPlan(null);
        })
        .catch((err: Error) => onError(err.message))
        .finally(() => setBusy(false));
    },
    [onError],
  );

  const undo = useCallback(() => {
    setBusy(true);
    rollbackFix(fix.id)
      .then(() => setResult(null))
      .catch((err: Error) => onError(err.message))
      .finally(() => setBusy(false));
  }, [fix.id, onError]);

  return (
    <li className="rounded-lg border border-slate-200 bg-white p-4 dark:border-slate-800 dark:bg-slate-900">
      <div className="flex flex-wrap items-baseline gap-3">
        <code className="font-mono text-xs text-slate-500 dark:text-slate-400">{fix.id}</code>
        {fix.requires_admin && (
          <span className="text-xs text-slate-500 dark:text-slate-400">
            {t('agent.checks.requires_admin')}
          </span>
        )}
        {!fix.reversible && (
          <span className="text-xs text-slate-500 dark:text-slate-400">
            {t('ui.fixes.not_reversible')}
          </span>
        )}
      </div>

      <p className="mt-2 leading-relaxed">{t(fix.explanation.summary)}</p>

      {!plan && !result && (
        <button
          type="button"
          onClick={describe}
          disabled={busy}
          className="mt-3 rounded-md border border-slate-300 px-3 py-1.5 text-sm disabled:opacity-50 dark:border-slate-700"
        >
          {t('ui.fixes.describe')}
        </button>
      )}

      {plan && (
        <ConsentGate plan={plan} t={t} busy={busy} onApply={apply} onCancel={() => setPlan(null)} />
      )}
      {result && <AppliedOutcome result={result} t={t} busy={busy} onUndo={undo} />}
    </li>
  );
}
