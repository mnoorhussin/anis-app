import { Badge, Button, Card } from '@anis/ui';
import { type FormEvent, useCallback, useEffect, useState } from 'react';

import { formatNumber, plural } from '../i18n.js';
import { useLanguage } from '../lib/LanguageContext.js';
import { pb } from '../lib/pocketbase.js';

interface Summary {
  days: number;
  conversations: number;
  autoResolved: number;
  escalated: number;
  withHuman: number;
  unanswered: number;
  answered: number;
  leads: number;
  resolutionBreakdown: Record<string, number>;
  languages: Record<string, number>;
  ratedHelpful: number;
  ratedUnhelpful: number;
  firstResponseMedianMs: number | null;
  knowledgeGaps: number;
}

interface Gap {
  id: string;
  question: string;
  language: 'ar' | 'en' | '';
  count: number;
  best_similarity: number;
  answered: boolean;
}

const WINDOWS = [7, 30, 90] as const;

export function AnalyticsScreen({
  workspaceId,
  canAnswerGaps = true,
}: {
  workspaceId?: string;
  // False for agents: answering a gap adds knowledge, which the backend
  // reserves for owners and admins. They still see the queue.
  canAnswerGaps?: boolean;
}) {
  const { t, lang } = useLanguage();
  const [days, setDays] = useState<number>(30);
  const [summary, setSummary] = useState<Summary | null>(null);
  const [gaps, setGaps] = useState<Gap[]>([]);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(() => {
    // The analytics route and the knowledge-gap list are both per-workspace. An
    // account with several workspaces must look at one at a time, or an agency
    // would see every client's numbers summed into a meaningless total. When no
    // workspace is given the backend falls back to the caller's own.
    const wq = workspaceId ? `&workspace=${encodeURIComponent(workspaceId)}` : '';
    pb.send(`/api/anis/analytics?days=${days}${wq}`, { method: 'GET' })
      .then((s) => setSummary(s as Summary))
      .catch((err: unknown) => setError(err instanceof Error ? err.message : String(err)));

    // Ordered by frequency: the question asked forty times is worth more than
    // forty questions asked once. Filter params are bound, not interpolated, so
    // a workspace id can never smuggle filter syntax.
    const filter = workspaceId
      ? pb.filter('answered = false && workspace = {:w}', { w: workspaceId })
      : 'answered = false';
    pb.collection('knowledge_gaps')
      .getFullList<Gap>({ filter, sort: '-count' })
      .then(setGaps)
      .catch(() => setGaps([]));
  }, [days, workspaceId]);

  useEffect(load, [load]);

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

  const resolvedPct =
    summary.conversations > 0
      ? Math.round((summary.autoResolved / summary.conversations) * 100)
      : 0;

  return (
    <div className="flex flex-col gap-4">
      <div className="flex gap-1 self-start rounded-xl bg-surface-2 p-1">
        {WINDOWS.map((d) => (
          <button
            key={d}
            type="button"
            onClick={() => setDays(d)}
            aria-current={days === d ? 'true' : undefined}
            className={`rounded-lg px-3 py-1 text-sm font-medium transition-colors ${
              days === d
                ? 'bg-surface text-foreground shadow-soft'
                : 'text-muted hover:text-foreground'
            }`}
          >
            {plural(lang, 'days', d)}
          </button>
        ))}
      </div>

      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        <Metric label={t('mConversations')} value={formatNumber(summary.conversations)} />
        <Metric
          label={t('mAutoResolved')}
          value={`${formatNumber(summary.autoResolved)} · ${resolvedPct}%`}
          tone={summary.autoResolved > 0 ? 'success' : 'neutral'}
        />
        <Metric label={t('mEscalated')} value={formatNumber(summary.escalated)} tone="warning" />
        <Metric label={t('mLeads')} value={formatNumber(summary.leads)} />
        <Metric label={t('mAnswered')} value={formatNumber(summary.answered)} />
        <Metric label={t('mUnanswered')} value={formatNumber(summary.unanswered)} />
        <Metric label={t('mFirstResponse')} value={formatDuration(summary.firstResponseMedianMs)} />
        <Metric
          label={t('mHelpful')}
          value={
            summary.ratedHelpful + summary.ratedUnhelpful === 0
              ? '—'
              : `${formatNumber(summary.ratedHelpful)} / ${formatNumber(summary.ratedUnhelpful)}`
          }
        />
      </div>

      <Card>
        <h3 className="text-lg font-semibold">{t('howResolved')}</h3>
        {/* The breakdown is shown rather than a single percentage because the
            signals are not equal evidence — a thumbs-up is far stronger than a
            conversation simply ending after a good answer, and collapsing them
            would overstate the headline. */}
        <p className="mt-1 max-w-prose text-sm text-muted">{t('howResolvedHelp')}</p>
        {summary.autoResolved === 0 ? (
          <p className="mt-3 text-sm text-muted">{t('noResolutionsYet')}</p>
        ) : (
          <ul className="mt-3 flex flex-wrap gap-2">
            {Object.entries(summary.resolutionBreakdown).map(([signal, n]) => (
              <li key={signal}>
                <Badge tone={signal === 'rated_helpful' ? 'success' : 'neutral'}>
                  {t(signalLabel(signal))} · {formatNumber(n)}
                </Badge>
              </li>
            ))}
          </ul>
        )}
      </Card>

      {Object.keys(summary.languages).length > 0 && (
        <Card>
          <h3 className="text-lg font-semibold">{t('languagesTitle')}</h3>
          <ul className="mt-3 flex flex-wrap gap-2">
            {Object.entries(summary.languages).map(([code, n]) => (
              <li key={code}>
                <Badge tone={code === 'ar' ? 'brand' : 'neutral'}>
                  {code === 'ar' ? 'العربية' : 'English'} · {formatNumber(n)}
                </Badge>
              </li>
            ))}
          </ul>
        </Card>
      )}

      <Card>
        <h3 className="text-lg font-semibold">{t('gapsTitle')}</h3>
        <p className="mt-1 max-w-prose text-sm text-muted">{t('gapsHelp')}</p>
        {gaps.length === 0 ? (
          <p className="mt-3 text-sm text-muted">{t('gapsEmpty')}</p>
        ) : (
          <ul className="mt-4 flex flex-col gap-3">
            {gaps.map((g) => (
              <GapRow key={g.id} gap={g} onAnswered={load} canAnswer={canAnswerGaps} />
            ))}
          </ul>
        )}
      </Card>

      <p className="text-xs text-muted">{t('metricsHonesty')}</p>
    </div>
  );
}

/**
 * Renders a measured duration at a resolution that does not lie about it.
 *
 * Rounding a 34 ms median to "0.0s" reads as "nothing was measured", and
 * rounding a 90-minute one to "5400.0s" is unreadable. Neither number is
 * rounded away from what was actually recorded.
 */
function formatDuration(ms: number | null): string {
  if (ms === null) return '—';
  if (ms < 1000) return `${formatNumber(Math.round(ms))}ms`;
  if (ms < 60_000) return `${(ms / 1000).toFixed(1)}s`;
  if (ms < 3_600_000) return `${Math.round(ms / 60_000)}m`;
  return `${(ms / 3_600_000).toFixed(1)}h`;
}

function signalLabel(signal: string) {
  switch (signal) {
    case 'rated_helpful':
      return 'sigRatedHelpful' as const;
    case 'user_confirmed':
      return 'sigUserConfirmed' as const;
    case 'action_completed':
      return 'sigActionCompleted' as const;
    default:
      return 'sigEndedAnswered' as const;
  }
}

function Metric({
  label,
  value,
  tone,
}: {
  label: string;
  value: string;
  tone?: 'success' | 'warning' | 'neutral';
}) {
  return (
    <div className="rounded-2xl border border-border-soft bg-surface p-4 shadow-soft">
      <p className="text-xs text-muted">{label}</p>
      <p
        // Figures are Western digits in both languages and never mirrored.
        dir="ltr"
        className={`mt-1 text-start font-display text-2xl font-semibold ${
          tone === 'success'
            ? 'text-status-success'
            : tone === 'warning'
              ? 'text-status-warning'
              : ''
        }`}
      >
        {value}
      </p>
    </div>
  );
}

function GapRow({
  gap,
  onAnswered,
  canAnswer,
}: {
  gap: Gap;
  onAnswered: () => void;
  canAnswer: boolean;
}) {
  const { t } = useLanguage();
  const [open, setOpen] = useState(false);
  const [answer, setAnswer] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // The similarity retrieval managed tells the owner WHICH problem this is:
  // a near miss means the answer is probably already there and the assistant
  // was too cautious; a low score means the content genuinely does not exist.
  const nearMiss = gap.best_similarity >= 0.25;

  async function submit(e: FormEvent) {
    e.preventDefault();
    if (!answer.trim()) return;
    setBusy(true);
    setError(null);
    try {
      await pb.send('/api/anis/gaps/answer', {
        method: 'POST',
        body: { gapId: gap.id, answer: answer.trim() },
      });
      onAnswered();
    } catch (err: unknown) {
      const res = (err as { response?: { error?: string } })?.response;
      setError(res?.error ?? (err instanceof Error ? err.message : String(err)));
      setBusy(false);
    }
  }

  return (
    <li className="rounded-xl bg-surface-2 p-3">
      <div className="flex flex-wrap items-start gap-2">
        <span dir="auto" className="grow text-sm font-medium">
          {gap.question}
        </span>
        <Badge tone={gap.count > 1 ? 'warning' : 'neutral'}>×{gap.count}</Badge>
        {nearMiss && <Badge tone="info">{t('gapNearMiss')}</Badge>}
        {!open && canAnswer && (
          <Button size="sm" variant="secondary" onClick={() => setOpen(true)}>
            {t('gapAnswer')}
          </Button>
        )}
      </div>

      {open && (
        <form onSubmit={submit} className="mt-3 flex flex-col gap-2">
          <textarea
            value={answer}
            onChange={(e) => setAnswer(e.target.value)}
            placeholder={t('gapAnswerPlaceholder')}
            dir="auto"
            rows={3}
            className="rounded-lg border border-border-soft bg-surface px-3 py-2 text-sm outline-none focus-visible:outline-2 focus-visible:outline-offset-2"
          />
          {error && (
            <p role="alert" dir="auto" className="text-xs text-status-danger">
              {error}
            </p>
          )}
          <div className="flex gap-2">
            <Button type="submit" size="sm" variant="primary" disabled={busy}>
              {busy ? t('saving') : t('gapSaveToKb')}
            </Button>
            <Button size="sm" variant="ghost" onClick={() => setOpen(false)} disabled={busy}>
              {t('cancel')}
            </Button>
          </div>
        </form>
      )}
    </li>
  );
}
