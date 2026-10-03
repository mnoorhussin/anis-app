import { PLANS, type PlanId } from '@anis/types';
import { Badge, Button, Card, Logo } from '@anis/ui';
import { Fragment, useCallback, useEffect, useState } from 'react';

import { formatNumber, type StringKey } from '../i18n.js';
import { useLanguage } from '../lib/LanguageContext.js';
import { pb } from '../lib/pocketbase.js';
import { useSignOut } from '../lib/useAuth.js';
import { AnalyticsScreen } from './AnalyticsScreen.js';
import { BillingScreen } from './BillingScreen.js';
import { ClientsScreen, type Roster } from './ClientsScreen.js';
import { DeleteWorkspaceCard } from './DeleteWorkspaceCard.js';
import { InboxScreen } from './InboxScreen.js';
import { SourcesCard } from './SourcesCard.js';

interface Workspace {
  id: string;
  name: string;
  widget_key: string;
  allowed_domains: string[];
  expand?: { account?: { plan: PlanId; name: string } };
}

const CDN_URL = import.meta.env['VITE_CDN_URL'] ?? 'https://cdn.anis.chat';

type Tab = 'clients' | 'workspace' | 'inbox' | 'analytics' | 'billing';

const tabLabel: Record<Tab, StringKey> = {
  clients: 'tabClients',
  workspace: 'tabWorkspace',
  inbox: 'tabInbox',
  analytics: 'tabAnalytics',
  billing: 'tabBilling',
};

export function WorkspaceScreen() {
  const { t, lang, setLang } = useLanguage();
  const signOut = useSignOut();

  const [roster, setRoster] = useState<Roster | null>(null);
  const [activeId, setActiveId] = useState<string | null>(null);
  const [workspace, setWorkspace] = useState<Workspace | null>(null);
  const [tab, setTab] = useState<Tab>('workspace');
  const [error, setError] = useState<string | null>(null);
  const [creating, setCreating] = useState(false);
  const [createError, setCreateError] = useState<string | null>(null);
  const [deleting, setDeleting] = useState(false);
  const [deleteError, setDeleteError] = useState<string | null>(null);

  // The roster is the account's workspaces plus their shared-allowance usage.
  // It is the one source of truth for the switcher and the Clients screen, so
  // they can never disagree about which workspaces exist.
  const loadRoster = useCallback(async (): Promise<Roster | null> => {
    try {
      const r = (await pb.send('/api/anis/workspaces', { method: 'GET' })) as Roster;
      setRoster(r);
      setActiveId((prev) => {
        if (prev && r.workspaces.some((w) => w.id === prev)) return prev;
        // Prefer a workspace the caller owns; fall back to the first.
        return r.workspaces.find((w) => w.role === 'owner')?.id ?? r.workspaces[0]?.id ?? null;
      });
      return r;
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
      return null;
    }
  }, []);

  useEffect(() => {
    void loadRoster();
  }, [loadRoster]);

  // Load the full record for the active workspace. The roster endpoint carries
  // only what the list needs; the detail tab needs the widget key, the domain
  // allow-list and the plan, so those are fetched per selection.
  useEffect(() => {
    // A failed delete's message belongs to the workspace it was about.
    setDeleteError(null);
    if (!activeId) {
      setWorkspace(null);
      return;
    }
    let cancelled = false;
    pb.collection('workspaces')
      .getOne<Workspace>(activeId, { expand: 'account' })
      .then((w) => {
        if (!cancelled) setWorkspace(w);
      })
      .catch((err: unknown) => {
        if (!cancelled) setError(err instanceof Error ? err.message : String(err));
      });
    return () => {
      cancelled = true;
    };
  }, [activeId]);

  const createWorkspace = useCallback(
    async (name: string): Promise<boolean> => {
      setCreating(true);
      setCreateError(null);
      try {
        const created = (await pb.send('/api/anis/workspaces', {
          method: 'POST',
          body: { name },
        })) as { id: string };
        await loadRoster();
        setActiveId(created.id);
        return true;
      } catch (err) {
        setCreateError(describe(err));
        return false;
      } finally {
        setCreating(false);
      }
    },
    [loadRoster],
  );

  const deleteWorkspace = useCallback(
    async (id: string): Promise<boolean> => {
      setDeleting(true);
      setDeleteError(null);
      try {
        await pb.send(`/api/anis/workspaces/${encodeURIComponent(id)}`, { method: 'DELETE' });
        // The deleted id is no longer in the roster, so loadRoster moves the
        // selection to another workspace the caller owns. The Clients tab then
        // shows the roster without it — the confirmation that it is gone. It is
        // only in the nav on a multi-workspace plan; a downgraded account still
        // tidying up its extra workspaces lands back on the workspace tab.
        const r = await loadRoster();
        setTab((r?.account.max_workspaces ?? 1) > 1 ? 'clients' : 'workspace');
        return true;
      } catch (err) {
        setDeleteError(describe(err));
        return false;
      } finally {
        setDeleting(false);
      }
    },
    [loadRoster],
  );

  // Plan comes from the account (the single source of truth for entitlements),
  // falling back to the active workspace's expanded account, then free.
  const plan: PlanId =
    (roster?.account.plan as PlanId) ?? workspace?.expand?.account?.plan ?? 'free';
  const domains = workspace?.allowed_domains ?? [];
  const multi = (roster?.account.max_workspaces ?? 1) > 1;
  const switchable = (roster?.workspaces.length ?? 0) > 1;
  // Mirrors the server's guard: only an owner may delete, and never the last
  // workspace they own. Hiding the card in those cases spares a 409 the owner
  // could do nothing about; the backend still enforces it either way.
  const owned = roster?.workspaces.filter((w) => w.role === 'owner') ?? [];
  const canDelete = !!workspace && owned.length > 1 && owned.some((w) => w.id === workspace.id);

  const tabs: Tab[] = multi
    ? ['clients', 'workspace', 'inbox', 'analytics', 'billing']
    : ['workspace', 'inbox', 'analytics', 'billing'];

  return (
    <main className="mx-auto flex min-h-dvh w-full max-w-3xl flex-col gap-6 px-6 py-10">
      <header className="flex flex-wrap items-center justify-between gap-4">
        <Logo lang={lang} />
        <div className="flex items-center gap-2">
          {switchable && roster && (
            <label className="flex items-center gap-1.5">
              <span className="sr-only">{t('switchWorkspace')}</span>
              <select
                value={activeId ?? ''}
                onChange={(e) => setActiveId(e.target.value)}
                dir="auto"
                className="rounded-lg border border-border-soft bg-surface-2 px-3 py-1.5 text-sm text-foreground outline-none focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
                aria-label={t('switchWorkspace')}
              >
                {roster.workspaces.map((w) => (
                  <option key={w.id} value={w.id}>
                    {w.name}
                  </option>
                ))}
              </select>
            </label>
          )}
          <Button
            size="sm"
            variant="ghost"
            onClick={() => setLang(lang === 'ar' ? 'en' : 'ar')}
            lang={lang === 'ar' ? 'en' : 'ar'}
          >
            {lang === 'ar' ? 'English' : 'العربية'}
          </Button>
          <Button size="sm" variant="secondary" onClick={signOut}>
            {t('signOut')}
          </Button>
        </div>
      </header>

      {(workspace || roster) && (
        <nav className="flex gap-1 rounded-xl bg-surface-2 p-1" aria-label="Sections">
          {tabs.map((id) => (
            <button
              key={id}
              type="button"
              onClick={() => setTab(id)}
              aria-current={tab === id ? 'page' : undefined}
              className={`grow rounded-lg px-3 py-1.5 text-sm font-medium transition-colors ${
                tab === id
                  ? 'bg-surface text-foreground shadow-soft'
                  : 'text-muted hover:text-foreground'
              }`}
            >
              {t(tabLabel[id])}
            </button>
          ))}
        </nav>
      )}

      {error && (
        <Card>
          <p role="alert" className="text-sm text-status-danger">
            {error}
          </p>
        </Card>
      )}

      {tab === 'clients' && roster && (
        <ClientsScreen
          roster={roster}
          activeId={activeId}
          onOpen={(id) => {
            setActiveId(id);
            setTab('workspace');
          }}
          onCreate={createWorkspace}
          creating={creating}
          createError={createError}
        />
      )}

      {/* Keyed by workspace so switching remounts rather than reusing state: an
          open conversation, a half-written reply or a source draft belongs to
          the workspace it was started in, and must not carry over to — or be
          submitted into — the next one. Each key is prefixed because these are
          siblings under <main>; two children sharing a bare workspace id is a
          duplicate key, which React resolves by leaving stale elements on
          screen. */}
      {workspace && tab === 'inbox' && (
        <InboxScreen key={`inbox-${workspace.id}`} workspaceId={workspace.id} plan={plan} />
      )}

      {workspace && tab === 'analytics' && (
        <AnalyticsScreen key={`analytics-${workspace.id}`} workspaceId={workspace.id} />
      )}

      {tab === 'billing' && <BillingScreen />}

      {/* One key for the whole tab: the sources draft and the half-typed delete
          confirmation reset together when the workspace changes. */}
      {workspace && tab === 'workspace' && (
        <Fragment key={`workspace-${workspace.id}`}>
          <Card>
            <div className="flex flex-wrap items-center justify-between gap-3">
              <div>
                <p className="text-sm text-muted">{t('workspace')}</p>
                {/* The name comes from the user's own email or profile, so its
                    direction is theirs to decide, not the page's. */}
                <h1 dir="auto" className="font-display text-2xl font-semibold">
                  {workspace.name}
                </h1>
              </div>
              <div className="text-end">
                <p className="text-sm text-muted">{t('plan')}</p>
                <Badge tone={plan === 'free' ? 'neutral' : 'brand'}>
                  {PLANS[plan].name} · {formatNumber(PLANS[plan].limits.aiRepliesPerMonth)}
                </Badge>
              </div>
            </div>
          </Card>

          <SourcesCard workspaceId={workspace.id} />

          <Card>
            <h2 className="text-lg font-semibold">{t('installTitle')}</h2>
            <p className="mt-1 text-sm text-muted">{t('installLead')}</p>
            <InstallSnippet widgetKey={workspace.widget_key} />
          </Card>

          <Card>
            <h2 className="text-lg font-semibold">{t('domainsTitle')}</h2>
            {domains.length === 0 ? (
              // Not an error state — it is the safe default, and saying so
              // plainly is better than an empty box the owner has to
              // interpret. The widget genuinely will not load until a domain
              // is added.
              <p className="mt-2 flex items-start gap-2 text-sm text-muted">
                <Badge tone="warning">{t('notBuiltYet')}</Badge>
                <span>{t('domainsEmpty')}</span>
              </p>
            ) : (
              <ul className="mt-3 flex flex-wrap gap-2">
                {domains.map((d) => (
                  <li key={d}>
                    <Badge tone="success">{d}</Badge>
                  </li>
                ))}
              </ul>
            )}
          </Card>

          {canDelete && (
            <DeleteWorkspaceCard
              name={workspace.name}
              onDelete={() => deleteWorkspace(workspace.id)}
              deleting={deleting}
              error={deleteError}
            />
          )}
        </Fragment>
      )}
    </main>
  );
}

/**
 * Turn a PocketBase error into something readable.
 *
 * The workspace routes put the cases an owner can act on — the plan ceiling,
 * "you can't delete your only workspace" — in a plain `error` field, which the
 * SDK does not copy into `message`. Reading only `message` showed its generic
 * fallback instead of the reason. Same approach as BillingScreen.
 */
function describe(err: unknown): string {
  const res = (err as { response?: { error?: string; message?: string } })?.response;
  return res?.error ?? res?.message ?? (err instanceof Error ? err.message : String(err));
}

function InstallSnippet({ widgetKey }: { widgetKey: string }) {
  const { t } = useLanguage();
  const [copied, setCopied] = useState(false);

  const snippet = `<script src="${CDN_URL}/widget.js" data-anis-key="${widgetKey}" defer></script>`;

  async function copy() {
    try {
      await navigator.clipboard.writeText(snippet);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      // Clipboard access is denied outside a secure context and in some
      // embedded browsers. The snippet is selectable, so failing quietly is
      // fine — an error toast for something the user can just select is noise.
    }
  }

  return (
    <div className="mt-4 flex flex-col gap-2">
      {/* Always LTR and never mirrored: it is code, and an RTL run would
          reorder the angle brackets into something that no longer parses. */}
      <pre
        dir="ltr"
        className="overflow-x-auto rounded-xl bg-surface-3 p-3 text-start font-mono text-xs leading-relaxed"
      >
        <code>{snippet}</code>
      </pre>
      <div>
        <Button size="sm" variant="secondary" onClick={copy}>
          {copied ? t('copied') : t('copy')}
        </Button>
      </div>
    </div>
  );
}
