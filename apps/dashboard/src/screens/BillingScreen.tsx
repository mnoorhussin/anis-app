import { PLANS, PLAN_LIST, type PlanId } from '@anis/types';
import { Button, Card } from '@anis/ui';
import { type FormEvent, useCallback, useEffect, useState } from 'react';

import { formatNumber } from '../i18n.js';
import { useLanguage } from '../lib/LanguageContext.js';
import { pb } from '../lib/pocketbase.js';

interface Summary {
  plan: PlanId;
  repliesUsed: number;
  repliesLimit: number;
  overageUsd: number;
  hardCapUsd: number;
  subscriptionStatus: string;
  cancelAtPeriodEnd: boolean;
  currentPeriodEnd: string;
  billingConfigured: boolean;
  hasCustomer: boolean;
}

/** Matches USAGE_WARNING_THRESHOLDS in packages/types. */
const WARN_AT = 0.8;

export function BillingScreen() {
  const { t, lang } = useLanguage();
  const [summary, setSummary] = useState<Summary | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const load = useCallback(() => {
    pb.send('/api/anis/billing', { method: 'GET' })
      .then((s) => setSummary(s as Summary))
      .catch((err: unknown) => setError(describe(err)));
  }, []);

  useEffect(load, [load]);

  async function go(path: string, body?: unknown) {
    setBusy(true);
    setError(null);
    try {
      const res = (await pb.send(path, { method: 'POST', ...(body ? { body } : {}) })) as {
        url?: string;
      };
      // Checkout and the portal both hand back a Stripe URL to leave for.
      if (res.url) {
        window.location.href = res.url;
        return;
      }
      load();
    } catch (err: unknown) {
      setError(describe(err));
    } finally {
      setBusy(false);
    }
  }

  if (!summary) {
    return (
      <Card>
        {error ? (
          <p role="alert" className="text-sm text-status-danger">
            {error}
          </p>
        ) : (
          <p className="text-sm text-muted">…</p>
        )}
      </Card>
    );
  }

  const plan = PLANS[summary.plan];
  const ratio = summary.repliesLimit > 0 ? summary.repliesUsed / summary.repliesLimit : 0;
  const over = summary.repliesUsed >= summary.repliesLimit;

  return (
    <div className="flex flex-col gap-4">
      <Card>
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <p className="text-sm text-muted">{t('currentPlan')}</p>
            <h2 className="text-2xl font-semibold">{plan.name}</h2>
            {summary.subscriptionStatus === 'past_due' && (
              // Said plainly and without alarm: Stripe is still retrying, and
              // the plan has NOT changed. Telling someone they have been
              // downgraded when they have not is worse than saying nothing.
              <p className="mt-1 max-w-prose text-sm text-status-warning">{t('statusPastDue')}</p>
            )}
            {summary.cancelAtPeriodEnd && summary.currentPeriodEnd && (
              <p className="mt-1 text-sm text-muted">
                {t('cancelsOn')}{' '}
                {new Date(summary.currentPeriodEnd).toLocaleDateString(
                  lang === 'ar' ? 'ar' : 'en-GB',
                )}
              </p>
            )}
          </div>
          {summary.hasCustomer && (
            <Button
              size="sm"
              variant="secondary"
              disabled={busy}
              onClick={() => go('/api/anis/billing/portal')}
            >
              {t('managePlan')}
            </Button>
          )}
        </div>

        <div className="mt-5">
          <div className="flex items-baseline justify-between text-sm">
            <span className="text-muted">{t('repliesUsed')}</span>
            <span>
              {formatNumber(summary.repliesUsed)} {t('ofLimit')}{' '}
              {formatNumber(summary.repliesLimit)}
            </span>
          </div>
          <div
            className="mt-2 h-2 overflow-hidden rounded-full bg-surface-3"
            role="progressbar"
            aria-valuenow={Math.round(ratio * 100)}
            aria-valuemin={0}
            aria-valuemax={100}
          >
            <div
              className={`h-full rounded-full transition-[width] duration-500 ease-out-quint ${
                over ? 'bg-danger' : ratio >= WARN_AT ? 'bg-warning' : 'bg-oasis'
              }`}
              // Capped at 100% so going over does not overflow the track.
              style={{ width: `${Math.min(100, ratio * 100)}%` }}
            />
          </div>
          {over ? (
            <p className="mt-2 text-sm text-status-danger">{t('usageExceeded')}</p>
          ) : ratio >= WARN_AT ? (
            <p className="mt-2 text-sm text-status-warning">{t('usageWarning')}</p>
          ) : null}
          {summary.overageUsd > 0 && (
            <p className="mt-2 text-sm text-muted">
              {t('overageSoFar')}: ${summary.overageUsd.toFixed(2)}
            </p>
          )}
        </div>
      </Card>

      <SpendingCap current={summary.hardCapUsd} onSaved={load} />

      <Card>
        {!summary.billingConfigured ? (
          <p className="text-sm text-muted">{t('billingNotConfigured')}</p>
        ) : (
          <ul className="flex flex-wrap gap-2">
            {PLAN_LIST.filter((p) => p.id !== 'free' && p.id !== summary.plan).map((p) => (
              <li key={p.id}>
                <Button
                  size="sm"
                  variant={p.popular ? 'primary' : 'secondary'}
                  disabled={busy}
                  onClick={() => go('/api/anis/billing/checkout', { plan: p.id })}
                >
                  {t('upgrade')} · {p.name} · ${p.monthlyUsd}
                </Button>
              </li>
            ))}
          </ul>
        )}
        {error && (
          <p role="alert" className="mt-3 text-sm text-status-danger">
            {error}
          </p>
        )}
      </Card>
    </div>
  );
}

function SpendingCap({ current, onSaved }: { current: number; onSaved: () => void }) {
  const { t } = useLanguage();
  const [value, setValue] = useState(String(current));
  const [saved, setSaved] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => setValue(String(current)), [current]);

  async function save(e: FormEvent) {
    e.preventDefault();
    setError(null);
    const n = Number(value);
    if (!Number.isFinite(n) || n < 0) {
      setError(t('spendingCapHelp'));
      return;
    }
    try {
      await pb.send('/api/anis/billing/cap', { method: 'POST', body: { hardCapUsd: n } });
      setSaved(true);
      setTimeout(() => setSaved(false), 2000);
      onSaved();
    } catch (err: unknown) {
      setError(describe(err));
    }
  }

  return (
    <Card>
      <h3 className="text-lg font-semibold">{t('spendingCap')}</h3>
      <p className="mt-1 max-w-prose text-sm text-muted">{t('spendingCapHelp')}</p>
      <form onSubmit={save} className="mt-3 flex flex-wrap items-center gap-2">
        <span className="text-sm text-muted">$</span>
        <input
          type="number"
          min={0}
          step="1"
          value={value}
          onChange={(e) => setValue(e.target.value)}
          // A currency amount is always LTR, even on an Arabic page.
          dir="ltr"
          className="w-28 rounded-xl border border-border-soft bg-surface-2 px-3 py-2 text-sm outline-none focus-visible:outline-2 focus-visible:outline-offset-2"
        />
        <Button type="submit" size="sm" variant="secondary">
          {saved ? t('capSaved') : t('saveCap')}
        </Button>
      </form>
      {error && (
        <p role="alert" className="mt-2 text-sm text-status-danger">
          {error}
        </p>
      )}
    </Card>
  );
}

/**
 * Turn a PocketBase error into something readable.
 *
 * The billing endpoints return a plain `error` string for the cases a customer
 * can act on — "only the owner can change billing", "billing is not configured
 * on this instance" — so those are shown as written rather than replaced with
 * a generic message that hides the actual problem.
 */
function describe(err: unknown): string {
  const res = (err as { response?: { error?: string; message?: string } })?.response;
  return res?.error ?? res?.message ?? (err instanceof Error ? err.message : String(err));
}
