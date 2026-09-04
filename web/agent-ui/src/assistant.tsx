import { useCallback, useState } from 'react';
import { askAssist, discardAssist, prepareAssist } from './api';
import type { Translate } from './i18n';
import {
  fullRedaction,
  type AssistAnswer,
  type AssistPayload,
  type AssistState,
  type RedactionPolicy,
} from './types';
import { Toggle } from './components';

/**
 * Assistant is the optional second tier, and the only thing in this interface
 * that can reach outside the computer.
 *
 * The flow is deliberately two steps. Prepare builds the exact bytes and sends
 * none of them; the user reads the payload itself, not a summary of it, and
 * only then is there anything to confirm. Declining discards the payload, so a
 * refused send does not sit waiting for a later click.
 */
export function Assistant({
  state,
  t,
  onError,
  onFix,
}: {
  state: AssistState;
  t: Translate;
  onError: (message: string) => void;
  onFix?: (id: string) => void;
}) {
  // The protective option is the one already selected. Relaxing it is the
  // deliberate act, the same way it is for a saved report.
  const [policy, setPolicy] = useState<RedactionPolicy>(fullRedaction);
  const [payload, setPayload] = useState<AssistPayload | null>(null);
  const [answer, setAnswer] = useState<AssistAnswer | null>(null);
  const [busy, setBusy] = useState(false);

  const prepare = useCallback(() => {
    setBusy(true);
    setAnswer(null);
    prepareAssist(policy)
      .then(setPayload)
      .catch((err: Error) => onError(err.message))
      .finally(() => setBusy(false));
  }, [policy, onError]);

  const send = useCallback(() => {
    if (!payload) {
      return;
    }
    setBusy(true);
    askAssist(payload.token)
      .then((got) => {
        setAnswer(got);
        setPayload(null);
      })
      .catch((err: Error) => onError(err.message))
      .finally(() => setBusy(false));
  }, [payload, onError]);

  const cancel = useCallback(() => {
    if (!payload) {
      return;
    }
    const { token } = payload;
    setPayload(null);
    discardAssist(token).catch((err: Error) => onError(err.message));
  }, [payload, onError]);

  if (!state.enabled) {
    return null;
  }

  return (
    <section className="mt-10 rounded-lg border border-slate-200 bg-white p-4 dark:border-slate-800 dark:bg-slate-900">
      <h2 className="text-lg font-semibold">{t('ui.assist.heading')}</h2>
      <p className="mt-1 max-w-prose text-sm text-slate-600 dark:text-slate-400">
        {t('ui.assist.note')}
      </p>
      {state.endpoint && (
        <p className="mt-1 font-mono text-xs break-all text-slate-500 dark:text-slate-400">
          {state.endpoint}
        </p>
      )}

      {!payload && !answer && (
        <>
          <fieldset className="mt-3">
            <legend className="text-sm font-medium">{t('ui.redaction')}</legend>
            <div className="mt-2 grid gap-2 sm:grid-cols-2">
              <Toggle
                id="assist-hostnames"
                checked={policy.hostnames}
                label={t('ui.redact_hostnames')}
                onChange={(value) => setPolicy({ ...policy, hostnames: value })}
              />
              <Toggle
                id="assist-usernames"
                checked={policy.usernames}
                label={t('ui.redact_usernames')}
                onChange={(value) => setPolicy({ ...policy, usernames: value })}
              />
              <Toggle
                id="assist-serials"
                checked={policy.serials}
                label={t('ui.redact_serials')}
                onChange={(value) => setPolicy({ ...policy, serials: value })}
              />
              <Toggle
                id="assist-addresses"
                checked={policy.addresses}
                label={t('ui.redact_addresses')}
                onChange={(value) => setPolicy({ ...policy, addresses: value })}
              />
            </div>
          </fieldset>

          <button
            type="button"
            onClick={prepare}
            disabled={busy}
            className="mt-4 rounded-md border border-slate-300 px-3 py-1.5 text-sm disabled:opacity-50 dark:border-slate-700"
          >
            {t('ui.assist.prepare')}
          </button>
        </>
      )}

      {payload && (
        <div className="mt-4 rounded-md border border-amber-300 bg-amber-50 p-4 dark:border-amber-800 dark:bg-amber-950/40">
          <p className="text-sm">{t('ui.assist.destination', [payload.host, payload.model])}</p>
          <p className="mt-1 text-sm">{t('ui.assist.size', [payload.bytes])}</p>
          <p className="mt-1 text-sm">
            {payload.redacted ? t('ui.assist.redacted') : t('ui.assist.not_redacted')}
          </p>

          <p className="mt-3 text-sm font-medium">{t('ui.assist.payload')}</p>
          <pre className="mt-1 max-h-96 overflow-auto rounded-md border border-slate-200 bg-slate-50 p-3 text-xs dark:border-slate-800 dark:bg-slate-950">
            {payload.body}
          </pre>

          <div className="mt-4 flex flex-wrap gap-3">
            <button
              type="button"
              onClick={send}
              disabled={busy}
              className="rounded-md bg-amber-700 px-3 py-1.5 text-sm font-medium text-white hover:bg-amber-800 disabled:opacity-50 dark:bg-amber-600 dark:hover:bg-amber-500"
            >
              {t('ui.assist.send')}
            </button>
            <button
              type="button"
              onClick={cancel}
              className="rounded-md border border-slate-300 px-3 py-1.5 text-sm dark:border-slate-700"
            >
              {t('ui.assist.cancel')}
            </button>
          </div>
        </div>
      )}

      {answer && (
        <div className="mt-4">
          <p className="text-sm font-medium">
            {t('ui.assist.answer_from', [answer.model ?? t('ui.assist.unnamed_model')])}
          </p>
          {answer.notes && <p className="mt-2 text-sm whitespace-pre-line">{answer.notes}</p>}

          {answer.fixes && answer.fixes.length > 0 && (
            <div className="mt-3">
              <p className="text-sm font-medium">{t('ui.assist.suggested')}</p>
              <div className="mt-2 flex flex-wrap gap-2">
                {answer.fixes.map((id) => (
                  <button
                    key={id}
                    type="button"
                    onClick={() => onFix?.(id)}
                    className="rounded-md border border-slate-300 px-2.5 py-1 text-xs dark:border-slate-700"
                  >
                    {t('ui.advice.repair_available', [id])}
                  </button>
                ))}
              </div>
            </div>
          )}

          {answer.discarded > 0 && (
            <p className="mt-3 text-sm text-slate-600 dark:text-slate-400">
              {t('ui.assist.discarded', [answer.discarded])}
            </p>
          )}

          <p className="mt-3 max-w-prose text-sm text-slate-600 dark:text-slate-400">
            {t('ui.assist.caveat')}
          </p>
        </div>
      )}
    </section>
  );
}
