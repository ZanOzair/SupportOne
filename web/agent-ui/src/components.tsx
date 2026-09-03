import { useState } from 'react';
import type { CheckResult, Severity } from './types';
import type { Translate } from './i18n';

/**
 * Severity is carried by a written label as well as a colour. Someone who
 * cannot distinguish the colours, or who prints the page, reads the same
 * verdict.
 */
const severityStyles: Record<Severity, string> = {
  ok: 'bg-emerald-100 text-emerald-900 dark:bg-emerald-950 dark:text-emerald-200',
  attention: 'bg-amber-100 text-amber-900 dark:bg-amber-950 dark:text-amber-200',
  urgent: 'bg-red-100 text-red-900 dark:bg-red-950 dark:text-red-200',
  unknown: 'bg-slate-200 text-slate-800 dark:bg-slate-800 dark:text-slate-200',
};

export function SeverityChip({ severity, label }: { severity: Severity; label: string }) {
  return (
    <span
      className={`inline-block rounded-full px-2.5 py-0.5 text-xs font-semibold ${severityStyles[severity]}`}
    >
      {label}
    </span>
  );
}

export function SummaryCounts({ results, t }: { results: CheckResult[]; t: Translate }) {
  const order: Severity[] = ['urgent', 'attention', 'unknown', 'ok'];
  const counts = order.map((severity) => ({
    severity,
    value: results.filter((r) => r.severity === severity).length,
  }));

  return (
    <ul className="flex flex-wrap gap-3" aria-label={t('report.summary')}>
      {counts.map(({ severity, value }) => (
        <li key={severity} className="flex items-baseline gap-1.5">
          <span className="text-lg font-semibold tabular-nums">{value}</span>
          <SeverityChip severity={severity} label={t(`severity.${severity}`)} />
        </li>
      ))}
    </ul>
  );
}

export function ResultCard({ result, t }: { result: CheckResult; t: Translate }) {
  const [open, setOpen] = useState(false);
  const hasDetail = result.detail !== undefined && Object.keys(result.detail).length > 0;

  return (
    <li className="rounded-lg border border-slate-200 bg-white p-4 dark:border-slate-800 dark:bg-slate-900">
      <div className="flex flex-wrap items-baseline gap-3">
        <SeverityChip severity={result.severity} label={t(`severity.${result.severity}`)} />
        <code className="font-mono text-xs text-slate-500 dark:text-slate-400">
          {result.check_id}
        </code>
      </div>

      <p className="mt-2 leading-relaxed">{t(result.summary, result.summary_args)}</p>
      {result.error && (
        <p className="mt-1 text-sm text-slate-500 dark:text-slate-400">{result.error}</p>
      )}

      {hasDetail && (
        <div className="mt-3">
          <button
            type="button"
            onClick={() => setOpen(!open)}
            aria-expanded={open}
            className="text-sm text-sky-700 underline underline-offset-2 hover:no-underline dark:text-sky-400"
          >
            {t('ui.evidence')}
          </button>
          {open && (
            <pre className="mt-2 overflow-x-auto rounded-md border border-slate-200 bg-slate-50 p-3 text-xs dark:border-slate-800 dark:bg-slate-950">
              {JSON.stringify(result.detail, null, 2)}
            </pre>
          )}
        </div>
      )}
    </li>
  );
}

export function Toggle({
  id,
  checked,
  label,
  onChange,
}: {
  id: string;
  checked: boolean;
  label: string;
  onChange: (value: boolean) => void;
}) {
  return (
    <label htmlFor={id} className="flex items-center gap-2 text-sm">
      <input
        id={id}
        type="checkbox"
        checked={checked}
        onChange={(event) => onChange(event.target.checked)}
        className="size-4 rounded border-slate-400 dark:border-slate-600"
      />
      {label}
    </label>
  );
}
