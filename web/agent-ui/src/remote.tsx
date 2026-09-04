import { useCallback, useState } from 'react';
import { declineRemote, endRemote, planRemote, startRemote } from './api';
import type { Translate } from './i18n';
import type { RemotePlan, RemoteSession, RemoteState } from './types';

/**
 * RemoteHelp is the consent step in front of a remote-help session.
 *
 * SupportOne implements no remote desktop protocol. This panel finds the
 * programs already on the machine, says in plain words what letting someone in
 * allows, takes an agreement against that exact list, and records the start and
 * the end. It cannot watch the session or limit it, and it says so where the
 * user is deciding rather than in a document they will not read.
 */
export function RemoteHelp({
  state,
  t,
  onChange,
  onError,
}: {
  state: RemoteState;
  t: Translate;
  onChange: (state: RemoteState) => void;
  onError: (message: string) => void;
}) {
  const [technician, setTechnician] = useState('');
  const [toolID, setToolID] = useState('');
  const [plan, setPlan] = useState<RemotePlan | null>(null);
  const [ended, setEnded] = useState<RemoteSession | null>(null);
  const [busy, setBusy] = useState(false);

  const installed = state.tools.filter((tool) => tool.installed);

  const describe = useCallback(() => {
    setBusy(true);
    setEnded(null);
    planRemote(technician, toolID)
      .then(setPlan)
      .catch((err: Error) => onError(err.message))
      .finally(() => setBusy(false));
  }, [technician, toolID, onError]);

  // The list sent back is the one rendered above, unedited. The agent refuses
  // a confirmation that does not repeat it, which is what makes showing it the
  // condition of starting rather than a courtesy.
  const start = useCallback(() => {
    if (!plan) {
      return;
    }
    setBusy(true);
    startRemote(plan.token, plan.consequences)
      .then((session) => {
        setPlan(null);
        onChange({ ...state, session });
      })
      .catch((err: Error) => onError(err.message))
      .finally(() => setBusy(false));
  }, [plan, state, onChange, onError]);

  // Saying no is a decision, and the audit log records it as one rather than
  // showing a question that was asked and never answered.
  const decline = useCallback(() => {
    setPlan(null);
    declineRemote().catch((err: Error) => onError(err.message));
  }, [onError]);

  const finish = useCallback(() => {
    setBusy(true);
    endRemote()
      .then((session) => {
        setEnded(session);
        onChange({ ...state, session: null });
      })
      .catch((err: Error) => onError(err.message))
      .finally(() => setBusy(false));
  }, [state, onChange, onError]);

  if (!state.available) {
    return null;
  }

  return (
    <section className="mt-10 rounded-lg border border-slate-200 bg-white p-4 dark:border-slate-800 dark:bg-slate-900">
      <h2 className="text-lg font-semibold">{t('remote.heading')}</h2>

      <ul className="mt-3 max-w-prose space-y-1 text-sm text-slate-700 dark:text-slate-300">
        {state.consequences.map((key) => (
          <li key={key}>{t(key)}</li>
        ))}
      </ul>

      {state.session ? (
        <div className="mt-4 rounded-md border border-amber-300 bg-amber-50 p-4 dark:border-amber-800 dark:bg-amber-950/40">
          <p className="text-sm font-medium">{t('ui.remote.open', [state.session.technician])}</p>
          <p className="mt-1 text-sm">{t('ui.remote.cannot_end')}</p>
          <button
            type="button"
            onClick={finish}
            disabled={busy}
            className="mt-3 rounded-md bg-sky-700 px-3 py-1.5 text-sm font-medium text-white hover:bg-sky-800 disabled:opacity-60 dark:bg-sky-600 dark:hover:bg-sky-500"
          >
            {t('ui.remote.mark_ended')}
          </button>
        </div>
      ) : plan ? (
        <div className="mt-4 rounded-md border border-amber-300 bg-amber-50 p-4 dark:border-amber-800 dark:bg-amber-950/40">
          <p className="text-sm">{t('ui.remote.about_to', [plan.technician])}</p>
          <p className="mt-1 text-sm">
            {plan.tool.id
              ? t('ui.remote.will_start', [plan.tool.name])
              : t('ui.remote.starts_nothing')}
          </p>

          <div className="mt-3 flex flex-wrap gap-3">
            <button
              type="button"
              onClick={start}
              disabled={busy}
              className="rounded-md bg-sky-700 px-3 py-1.5 text-sm font-medium text-white hover:bg-sky-800 disabled:opacity-60 dark:bg-sky-600 dark:hover:bg-sky-500"
            >
              {t('ui.remote.allow')}
            </button>
            <button
              type="button"
              onClick={decline}
              className="rounded-md border border-slate-300 px-3 py-1.5 text-sm dark:border-slate-700"
            >
              {t('ui.remote.stop')}
            </button>
          </div>
        </div>
      ) : (
        <div className="mt-4">
          <div className="grid gap-3 sm:grid-cols-2">
            <label className="text-sm">
              <span className="block font-medium">{t('ui.remote.who')}</span>
              <input
                type="text"
                value={technician}
                onChange={(event) => setTechnician(event.target.value)}
                className="mt-1 w-full rounded-md border border-slate-300 bg-white px-2 py-1 dark:border-slate-700 dark:bg-slate-900"
              />
            </label>

            <label className="text-sm">
              <span className="block font-medium">{t('ui.remote.which_tool')}</span>
              <select
                value={toolID}
                onChange={(event) => setToolID(event.target.value)}
                className="mt-1 w-full rounded-md border border-slate-300 bg-white px-2 py-1 dark:border-slate-700 dark:bg-slate-900"
              >
                <option value="">{t('ui.remote.no_tool')}</option>
                {installed.map((tool) => (
                  <option key={tool.id} value={tool.id}>
                    {tool.name}
                  </option>
                ))}
              </select>
            </label>
          </div>

          {installed.length === 0 && (
            <p className="mt-2 max-w-prose text-sm text-slate-600 dark:text-slate-400">
              {t('ui.remote.none_installed')}
            </p>
          )}
          <p className="mt-2 max-w-prose text-sm text-slate-600 dark:text-slate-400">
            {t('ui.remote.no_install')}
          </p>

          <button
            type="button"
            onClick={describe}
            disabled={busy || technician.trim() === ''}
            className="mt-3 rounded-md border border-slate-300 px-3 py-1.5 text-sm disabled:opacity-50 dark:border-slate-700"
          >
            {t('ui.remote.describe')}
          </button>
        </div>
      )}

      {ended && (
        <p className="mt-4 max-w-prose text-sm text-slate-700 dark:text-slate-300">
          {t('ui.remote.ended')} {t('remote.close_anything')}
        </p>
      )}
    </section>
  );
}
