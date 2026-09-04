import type { Translate } from './i18n';
import type { Advice } from './types';

/**
 * AdviceBlock is the offline explanation shown beneath a finding: what it
 * means, and what to do about it, in the order worth trying.
 *
 * It sits inside the result it explains rather than in a separate panel,
 * because the answer to "what does that mean" belongs next to the thing it
 * explains, not somewhere the reader has to cross-reference.
 */
export function AdviceBlock({
  advice,
  t,
  onFix,
  onWizard,
}: {
  advice: Advice;
  t: Translate;
  onFix?: (id: string) => void;
  onWizard?: (id: string) => void;
}) {
  return (
    <div className="mt-3 border-l-2 border-slate-300 pl-3 dark:border-slate-700">
      <p className="text-sm leading-relaxed">{t(advice.cause)}</p>

      {advice.steps && advice.steps.length > 0 && (
        <>
          <p className="mt-2 text-sm font-medium">{t('ui.advice.what_to_do')}</p>
          <ol className="mt-1 list-decimal space-y-1 pl-5 text-sm">
            {advice.steps.map((step) => (
              <li key={step}>{t(step)}</li>
            ))}
          </ol>
        </>
      )}

      {((advice.fixes && advice.fixes.length > 0) ||
        (advice.wizards && advice.wizards.length > 0)) && (
        <div className="mt-3 flex flex-wrap gap-2">
          {advice.wizards?.map((id) => (
            <button
              key={id}
              type="button"
              onClick={() => onWizard?.(id)}
              className="rounded-md bg-sky-700 px-2.5 py-1 text-xs font-medium text-white hover:bg-sky-800 dark:bg-sky-600 dark:hover:bg-sky-500"
            >
              {t('ui.advice.walk_me_through')}
            </button>
          ))}
          {advice.fixes?.map((id) => (
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
      )}
    </div>
  );
}
