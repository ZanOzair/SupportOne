import { useCallback, useEffect, useState } from 'react';
import {
  closeSession,
  fetchMessages,
  fetchSession,
  fetchSnapshot,
  hasToken,
  previewRedaction,
  reportURL,
  runChecks,
} from './api';
import { ResultCard, SummaryCounts, Toggle } from './components';
import { translator } from './i18n';
import { fullRedaction, redacts, type RedactionPolicy, type Session, type Snapshot } from './types';

export default function App() {
  const [session, setSession] = useState<Session | null>(null);
  const [messages, setMessages] = useState<Record<string, string>>({});
  const [snapshot, setSnapshot] = useState<Snapshot | null>(null);
  const [lang, setLang] = useState('');
  const [busy, setBusy] = useState(false);
  const [closed, setClosed] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // The payload preview starts fully redacted: the protective option is the
  // one already selected, and relaxing it is the deliberate act.
  const [policy, setPolicy] = useState<RedactionPolicy>(fullRedaction);
  const [preview, setPreview] = useState<Snapshot | null>(null);

  const t = translator(messages);

  useEffect(() => {
    if (!hasToken) {
      return;
    }
    fetchSession()
      .then((info) => {
        setSession(info);
        setLang(info.lang);
      })
      .catch((err: Error) => setError(err.message));
  }, []);

  useEffect(() => {
    if (!lang) {
      return;
    }
    fetchMessages(lang)
      .then((catalog) => setMessages(catalog.messages))
      .catch((err: Error) => setError(err.message));
  }, [lang]);

  useEffect(() => {
    if (!session) {
      return;
    }
    fetchSnapshot()
      .then(setSnapshot)
      .catch((err: Error) => setError(err.message));
  }, [session]);

  const recheck = useCallback(() => {
    setBusy(true);
    setPreview(null);
    runChecks()
      .then(setSnapshot)
      .catch((err: Error) => setError(err.message))
      .finally(() => setBusy(false));
  }, []);

  const showPreview = useCallback(() => {
    previewRedaction(policy)
      .then(setPreview)
      .catch((err: Error) => setError(err.message));
  }, [policy]);

  const end = useCallback(() => {
    closeSession()
      .then(() => setClosed(true))
      .catch((err: Error) => setError(err.message));
  }, []);

  if (!hasToken) {
    return (
      <Shell>
        <p className="max-w-prose">
          This page was opened without the token the agent generates for a session. Close it and
          start the agent again; the link it opens carries the token.
        </p>
      </Shell>
    );
  }

  if (closed) {
    return (
      <Shell>
        <p>{t('ui.closed')}</p>
      </Shell>
    );
  }

  return (
    <Shell>
      <header className="mb-6">
        <div className="flex flex-wrap items-start justify-between gap-4">
          <div>
            <h1 className="text-2xl font-semibold">{t('ui.heading')}</h1>
            <p className="mt-1 max-w-prose text-slate-600 dark:text-slate-400">
              {t('ui.subheading')}
            </p>
          </div>

          {session && session.languages.length > 1 && (
            <label className="flex items-center gap-2 text-sm">
              <span>{t('ui.language')}</span>
              <select
                value={lang}
                onChange={(event) => setLang(event.target.value)}
                className="rounded-md border border-slate-300 bg-white px-2 py-1 dark:border-slate-700 dark:bg-slate-900"
              >
                {session.languages.map((code) => (
                  <option key={code} value={code}>
                    {code}
                  </option>
                ))}
              </select>
            </label>
          )}
        </div>

        {session && (
          <dl className="mt-4 text-sm text-slate-600 dark:text-slate-400">
            <div>
              {t('ui.machine')}: {session.os} ({session.arch}) · {session.version}
            </div>
            {snapshot && (
              <div>
                {t('ui.checked_at')}: {new Date(snapshot.generated_at).toLocaleString()}
              </div>
            )}
          </dl>
        )}
      </header>

      {error && (
        <p
          role="alert"
          className="mb-4 rounded-md border border-red-300 bg-red-50 p-3 text-sm text-red-900 dark:border-red-900 dark:bg-red-950 dark:text-red-200"
        >
          {t('ui.error')}: {error}
        </p>
      )}

      {!snapshot && !error && <p>{t('ui.running')}</p>}

      {snapshot && (
        <>
          <section className="mb-6">
            <SummaryCounts results={snapshot.results} t={t} />
            {snapshot.results.every((r) => r.severity === 'ok') && (
              <p className="mt-3 max-w-prose">{t('ui.nothing_wrong')}</p>
            )}
          </section>

          <div className="mb-6 flex flex-wrap gap-3">
            <button
              type="button"
              onClick={recheck}
              disabled={busy}
              className="rounded-md bg-sky-700 px-3 py-1.5 text-sm font-medium text-white hover:bg-sky-800 disabled:opacity-60 dark:bg-sky-600 dark:hover:bg-sky-500"
            >
              {busy ? t('ui.rechecking') : t('ui.recheck')}
            </button>
            <button
              type="button"
              onClick={end}
              className="rounded-md border border-slate-300 px-3 py-1.5 text-sm dark:border-slate-700"
            >
              {t('ui.close')}
            </button>
          </div>

          <ul className="space-y-3">
            {snapshot.results.map((result) => (
              <ResultCard key={result.check_id} result={result} t={t} />
            ))}
          </ul>

          {snapshot.skipped_needs_admin && snapshot.skipped_needs_admin.length > 0 && (
            <section className="mt-8">
              <h2 className="text-lg font-semibold">{t('ui.skipped')}</h2>
              <p className="mt-1 max-w-prose text-sm text-slate-600 dark:text-slate-400">
                {t('ui.skipped_note')}
              </p>
              <ul className="mt-2 space-y-1">
                {snapshot.skipped_needs_admin.map((id) => (
                  <li key={id} className="font-mono text-xs text-slate-500 dark:text-slate-400">
                    {id}
                  </li>
                ))}
              </ul>
            </section>
          )}

          <section className="mt-10 rounded-lg border border-slate-200 bg-white p-4 dark:border-slate-800 dark:bg-slate-900">
            <h2 className="text-lg font-semibold">{t('ui.save')}</h2>
            <p className="mt-1 max-w-prose text-sm text-slate-600 dark:text-slate-400">
              {t('ui.redaction_note')}
            </p>

            <fieldset className="mt-3">
              <legend className="text-sm font-medium">{t('ui.redaction')}</legend>
              <div className="mt-2 grid gap-2 sm:grid-cols-2">
                <Toggle
                  id="redact-hostnames"
                  checked={policy.hostnames}
                  label={t('ui.redact_hostnames')}
                  onChange={(value) => setPolicy({ ...policy, hostnames: value })}
                />
                <Toggle
                  id="redact-usernames"
                  checked={policy.usernames}
                  label={t('ui.redact_usernames')}
                  onChange={(value) => setPolicy({ ...policy, usernames: value })}
                />
                <Toggle
                  id="redact-serials"
                  checked={policy.serials}
                  label={t('ui.redact_serials')}
                  onChange={(value) => setPolicy({ ...policy, serials: value })}
                />
                <Toggle
                  id="redact-addresses"
                  checked={policy.addresses}
                  label={t('ui.redact_addresses')}
                  onChange={(value) => setPolicy({ ...policy, addresses: value })}
                />
              </div>
            </fieldset>

            <div className="mt-4 flex flex-wrap gap-3">
              <a
                href={reportURL('html', policy, lang)}
                className="rounded-md bg-sky-700 px-3 py-1.5 text-sm font-medium text-white hover:bg-sky-800 dark:bg-sky-600 dark:hover:bg-sky-500"
              >
                {t('ui.save_html')}
              </a>
              <a
                href={reportURL('json', policy, lang)}
                className="rounded-md border border-slate-300 px-3 py-1.5 text-sm dark:border-slate-700"
              >
                {t('ui.save_json')}
              </a>
              <button
                type="button"
                onClick={preview ? () => setPreview(null) : showPreview}
                className="rounded-md border border-slate-300 px-3 py-1.5 text-sm dark:border-slate-700"
              >
                {preview ? t('ui.hide_preview') : t('ui.preview')}
              </button>
            </div>

            {preview && (
              <div className="mt-4">
                <p className="text-sm text-slate-600 dark:text-slate-400">{t('ui.preview_note')}</p>
                <pre className="mt-2 max-h-96 overflow-auto rounded-md border border-slate-200 bg-slate-50 p-3 text-xs dark:border-slate-800 dark:bg-slate-950">
                  {JSON.stringify(preview, null, 2)}
                </pre>
                {!redacts(policy) && (
                  <p className="mt-2 text-sm text-amber-800 dark:text-amber-300">
                    {t('ui.redaction')}: 0
                  </p>
                )}
              </div>
            )}
          </section>

          <footer className="mt-10 border-t border-slate-200 pt-4 text-sm text-slate-600 dark:border-slate-800 dark:text-slate-400">
            <p className="max-w-prose">{t('ui.offline_note')}</p>
            {session?.audit_path && (
              <p className="mt-2 font-mono text-xs break-all">
                {t('ui.audit')}: {session.audit_path}
              </p>
            )}
          </footer>
        </>
      )}
    </Shell>
  );
}

function Shell({ children }: { children: React.ReactNode }) {
  return <main className="mx-auto max-w-3xl px-4 py-8">{children}</main>;
}
