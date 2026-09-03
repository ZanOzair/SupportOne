import { useCallback, useState } from 'react';
import { startWizard, wizardConfirm, wizardMove } from './api';
import type { Translate } from './i18n';
import { ConsentGate } from './repair';
import { applicable, type Confirmation, type WizardSession, type WizardSummary } from './types';

/**
 * WizardList offers the guided walkthroughs. Each one asks its questions in
 * order, stops at the first thing that is wrong, and never reports a repair
 * the re-check did not confirm.
 */
export function WizardList({
  wizards,
  t,
  onError,
}: {
  wizards: WizardSummary[];
  t: Translate;
  onError: (message: string) => void;
}) {
  const [session, setSession] = useState<WizardSession | null>(null);
  const [busy, setBusy] = useState(false);

  const start = useCallback(
    (id: string) => {
      setBusy(true);
      startWizard(id)
        .then(setSession)
        .catch((err: Error) => onError(err.message))
        .finally(() => setBusy(false));
    },
    [onError],
  );

  const move = useCallback(
    (kind: 'next' | 'skip' | 'stop') => {
      if (!session) {
        return;
      }
      setBusy(true);
      wizardMove(kind, session.session_id)
        .then(setSession)
        .catch((err: Error) => onError(err.message))
        .finally(() => setBusy(false));
    },
    [session, onError],
  );

  const confirm = useCallback(
    (confirmation: Confirmation) => {
      if (!session) {
        return;
      }
      setBusy(true);
      wizardConfirm(session.session_id, confirmation)
        .then(setSession)
        .catch((err: Error) => onError(err.message))
        .finally(() => setBusy(false));
    },
    [session, onError],
  );

  if (wizards.length === 0) {
    return null;
  }

  return (
    <section className="mt-10">
      <h2 className="text-lg font-semibold">{t('ui.wizards.heading')}</h2>
      <p className="mt-1 max-w-prose text-sm text-slate-600 dark:text-slate-400">
        {t('ui.wizards.note')}
      </p>

      {!session && (
        <ul className="mt-3 space-y-3">
          {wizards.map((wizard) => (
            <li
              key={wizard.id}
              className="rounded-lg border border-slate-200 bg-white p-4 dark:border-slate-800 dark:bg-slate-900"
            >
              <p className="font-medium">{t(wizard.title)}</p>
              <p className="mt-1 text-sm text-slate-600 dark:text-slate-400">
                {t(wizard.complaint)}
              </p>
              <button
                type="button"
                onClick={() => start(wizard.id)}
                disabled={busy}
                className="mt-3 rounded-md bg-sky-700 px-3 py-1.5 text-sm font-medium text-white hover:bg-sky-800 disabled:opacity-60 dark:bg-sky-600 dark:hover:bg-sky-500"
              >
                {t('ui.wizards.start')}
              </button>
            </li>
          ))}
        </ul>
      )}

      {session && (
        <WizardRun
          session={session}
          t={t}
          busy={busy}
          onMove={move}
          onConfirm={confirm}
          onLeave={() => setSession(null)}
        />
      )}
    </section>
  );
}

function WizardRun({
  session,
  t,
  busy,
  onMove,
  onConfirm,
  onLeave,
}: {
  session: WizardSession;
  t: Translate;
  busy: boolean;
  onMove: (kind: 'next' | 'skip' | 'stop') => void;
  onConfirm: (confirmation: Confirmation) => void;
  onLeave: () => void;
}) {
  const { progress } = session;
  const running = progress.outcome === 'running';

  return (
    <div className="mt-3 rounded-lg border border-slate-200 bg-white p-4 dark:border-slate-800 dark:bg-slate-900">
      {progress.done.length > 0 && (
        <ol className="mb-4 space-y-1 text-sm">
          {progress.done.map((step) => (
            <li key={step.step_id} className="flex flex-wrap items-baseline gap-2">
              <span className="text-slate-500 dark:text-slate-400">
                {t(`ui.wizards.status.${step.status}`)}
              </span>
              <span>{t(step.finding.summary, step.finding.summary_args)}</span>
            </li>
          ))}
        </ol>
      )}

      {running && progress.step && (
        <>
          <p className="font-medium">{t(progress.step.title)}</p>
          <p className="mt-1">
            {t(progress.step.finding.summary, progress.step.finding.summary_args)}
          </p>
          {progress.step.error && (
            <p className="mt-1 text-sm text-slate-500 dark:text-slate-400">{progress.step.error}</p>
          )}
          {progress.advice && (
            <p className="mt-3 max-w-prose text-sm text-slate-600 dark:text-slate-400">
              {t(progress.advice)}
            </p>
          )}

          {progress.offer && applicable(progress.offer) ? (
            <ConsentGate
              plan={progress.offer}
              t={t}
              busy={busy}
              onApply={onConfirm}
              onCancel={() => onMove('skip')}
            />
          ) : (
            <div className="mt-4 flex flex-wrap gap-3">
              <button
                type="button"
                onClick={() => onMove('skip')}
                disabled={busy}
                className="rounded-md bg-sky-700 px-3 py-1.5 text-sm font-medium text-white hover:bg-sky-800 disabled:opacity-60 dark:bg-sky-600 dark:hover:bg-sky-500"
              >
                {t('ui.wizards.continue')}
              </button>
            </div>
          )}

          <button
            type="button"
            onClick={() => onMove('stop')}
            disabled={busy}
            className="mt-3 text-sm text-slate-600 underline underline-offset-2 hover:no-underline disabled:opacity-50 dark:text-slate-400"
          >
            {t('ui.wizards.stop')}
          </button>
        </>
      )}

      {!running && (
        <>
          <p className="font-medium">{t(`wizard.outcome.${progress.outcome}`)}</p>
          <button
            type="button"
            onClick={onLeave}
            className="mt-4 rounded-md border border-slate-300 px-3 py-1.5 text-sm dark:border-slate-700"
          >
            {t('ui.wizards.done')}
          </button>
        </>
      )}
    </div>
  );
}
