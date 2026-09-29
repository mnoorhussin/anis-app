import { Badge, Button, Card } from '@anis/ui';
import { type FormEvent, useCallback, useEffect, useState } from 'react';

import { type StringKey, plural } from '../i18n.js';
import { useLanguage } from '../lib/LanguageContext.js';
import { pb } from '../lib/pocketbase.js';

type SourceStatus = 'queued' | 'fetching' | 'processing' | 'ready' | 'failed' | 'stale';

interface Source {
  id: string;
  title: string;
  type: 'website' | 'pdf' | 'text' | 'faq';
  status: SourceStatus;
  error: string;
  pages: number;
}

/** Every status is shown to the customer — a source is never in a state they cannot see. */
const STATUS_LABEL: Record<SourceStatus, StringKey> = {
  queued: 'statusQueued',
  fetching: 'statusFetching',
  processing: 'statusProcessing',
  ready: 'statusReady',
  failed: 'statusFailed',
  stale: 'statusStale',
};

const STATUS_TONE = {
  queued: 'neutral',
  fetching: 'info',
  processing: 'info',
  ready: 'success',
  failed: 'danger',
  stale: 'warning',
} as const;

type Draft = { kind: 'text' | 'faq' | 'website' } | null;

export function SourcesCard({ workspaceId }: { workspaceId: string }) {
  const { t, lang } = useLanguage();
  const [sources, setSources] = useState<Source[]>([]);
  const [draft, setDraft] = useState<Draft>(null);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(() => {
    // No workspace filter: the collection's list rule already scopes this to
    // workspaces the caller belongs to. Filtering client-side would be a
    // suggestion, not a boundary.
    pb.collection('sources')
      .getFullList<Source>({ sort: '-created' })
      .then(setSources)
      .catch((err: unknown) => setError(err instanceof Error ? err.message : String(err)));
  }, []);

  useEffect(load, [load]);

  async function refresh(id: string) {
    try {
      await pb.send(`/api/anis/sources/${encodeURIComponent(id)}/refresh`, { method: 'POST' });
      load();
    } catch (err: unknown) {
      const res = (err as { response?: { message?: string } })?.response;
      setError(res?.message ?? (err instanceof Error ? err.message : String(err)));
    }
  }

  async function remove(id: string) {
    try {
      await pb.collection('sources').delete(id);
      load();
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : String(err));
    }
  }

  return (
    <Card>
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 className="text-lg font-semibold">{t('sourcesTitle')}</h2>
          <p className="mt-1 max-w-prose text-sm text-muted">{t('sourcesLead')}</p>
        </div>
        {!draft && (
          <div className="flex flex-wrap gap-2">
            <Button size="sm" variant="secondary" onClick={() => setDraft({ kind: 'text' })}>
              {t('addText')}
            </Button>
            <Button size="sm" variant="secondary" onClick={() => setDraft({ kind: 'faq' })}>
              {t('addFaq')}
            </Button>
            <Button size="sm" variant="secondary" onClick={() => setDraft({ kind: 'website' })}>
              {t('addWebsite')}
            </Button>
            {/* PDF is a badge, not a button. The backend refuses the type, and
                a control that looks available but is not is worse than one
                that says plainly it is coming. */}
            <Badge tone="neutral">{t('pdfSoon')}</Badge>
          </div>
        )}
      </div>

      {error && (
        <p role="alert" className="mt-3 text-sm text-status-danger">
          {error}
        </p>
      )}

      {draft && (
        <SourceForm
          kind={draft.kind}
          workspaceId={workspaceId}
          onDone={() => {
            setDraft(null);
            load();
          }}
          onCancel={() => setDraft(null)}
        />
      )}

      {sources.length === 0 && !draft ? (
        <p className="mt-4 text-sm text-muted">{t('sourcesEmpty')}</p>
      ) : (
        <ul className="mt-4 flex flex-col gap-2">
          {sources.map((s) => (
            <li
              key={s.id}
              className="flex flex-wrap items-center gap-3 rounded-xl bg-surface-2 px-4 py-3"
            >
              {/* Titles hold customer content in either language. */}
              <span dir="auto" className="grow font-medium">
                {s.title}
              </span>
              {s.status === 'ready' && s.pages > 0 && (
                <span className="text-xs text-muted">{plural(lang, 'passages', s.pages)}</span>
              )}
              <Badge tone={STATUS_TONE[s.status]}>{t(STATUS_LABEL[s.status])}</Badge>
              {s.type === 'website' && (
                <Button size="sm" variant="ghost" onClick={() => refresh(s.id)}>
                  {t('refresh')}
                </Button>
              )}
              <Button size="sm" variant="ghost" onClick={() => remove(s.id)}>
                {t('delete')}
              </Button>
              {s.status === 'failed' && s.error && (
                // The reason, verbatim, next to the failure. A status with no
                // explanation leaves the customer with nothing to act on.
                <p dir="auto" className="w-full text-xs text-status-danger">
                  {s.error}
                </p>
              )}
            </li>
          ))}
        </ul>
      )}
    </Card>
  );
}

function SourceForm({
  kind,
  workspaceId,
  onDone,
  onCancel,
}: {
  kind: 'text' | 'faq' | 'website';
  workspaceId: string;
  onDone: () => void;
  onCancel: () => void;
}) {
  const { t } = useLanguage();
  const [title, setTitle] = useState('');
  const [body, setBody] = useState('');
  const [url, setUrl] = useState('');
  const [pairs, setPairs] = useState([{ question: '', answer: '' }]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      // A custom route, not the collections API: ingestion spends money on
      // embeddings and has to respect the plan ceiling, so it cannot be a
      // plain record create.
      await pb.send('/api/anis/sources', {
        method: 'POST',
        body: {
          workspace: workspaceId,
          type: kind,
          title,
          ...(kind === 'website' ? { url } : kind === 'text' ? { body } : { pairs }),
        },
      });
      onDone();
    } catch (err: unknown) {
      const res = (err as { response?: { error?: string } })?.response;
      setError(res?.error ?? (err instanceof Error ? err.message : String(err)));
      setBusy(false);
    }
  }

  return (
    <form onSubmit={submit} className="mt-4 flex flex-col gap-3 rounded-xl bg-surface-2 p-4">
      {kind !== 'website' && (
        <label className="flex flex-col gap-1.5">
          <span className="text-sm font-medium text-muted">{t('sourceTitleLabel')}</span>
          <input
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            dir="auto"
            required
            className="rounded-lg border border-border-soft bg-surface px-3 py-2 outline-none focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
          />
        </label>
      )}

      {kind === 'website' ? (
        <label className="flex flex-col gap-1.5">
          <span className="text-sm font-medium text-muted">{t('websiteUrlLabel')}</span>
          <input
            value={url}
            onChange={(e) => setUrl(e.target.value)}
            type="url"
            placeholder="https://example.com"
            /* A URL is never RTL, even on an Arabic page. */
            dir="ltr"
            required
            className="rounded-lg border border-border-soft bg-surface px-3 py-2 outline-none focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
          />
          <span className="text-xs text-muted">{t('websiteHint')}</span>
          <span className="text-xs text-muted">{t('crawlNote')}</span>
        </label>
      ) : kind === 'text' ? (
        <label className="flex flex-col gap-1.5">
          <span className="text-sm font-medium text-muted">{t('sourceBodyLabel')}</span>
          <textarea
            value={body}
            onChange={(e) => setBody(e.target.value)}
            dir="auto"
            rows={8}
            required
            className="rounded-lg border border-border-soft bg-surface px-3 py-2 outline-none focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
          />
        </label>
      ) : (
        <>
          {pairs.map((p, i) => (
            <div key={i} className="flex flex-col gap-2 rounded-lg bg-surface p-3">
              <input
                value={p.question}
                onChange={(e) =>
                  setPairs(pairs.map((x, j) => (i === j ? { ...x, question: e.target.value } : x)))
                }
                placeholder={t('question')}
                dir="auto"
                className="rounded-lg border border-border-soft bg-surface-2 px-3 py-2 outline-none focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
              />
              <textarea
                value={p.answer}
                onChange={(e) =>
                  setPairs(pairs.map((x, j) => (i === j ? { ...x, answer: e.target.value } : x)))
                }
                placeholder={t('answer')}
                dir="auto"
                rows={3}
                className="rounded-lg border border-border-soft bg-surface-2 px-3 py-2 outline-none focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
              />
            </div>
          ))}
          <div>
            <Button
              size="sm"
              variant="ghost"
              onClick={() => setPairs([...pairs, { question: '', answer: '' }])}
            >
              {t('addPair')}
            </Button>
          </div>
        </>
      )}

      {error && (
        <p role="alert" dir="auto" className="text-sm text-status-danger">
          {error}
        </p>
      )}

      <div className="flex gap-2">
        <Button type="submit" variant="primary" size="sm" disabled={busy}>
          {busy ? t('saving') : t('save')}
        </Button>
        <Button variant="ghost" size="sm" onClick={onCancel} disabled={busy}>
          {t('cancel')}
        </Button>
      </div>
    </form>
  );
}
