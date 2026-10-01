import { PLANS, type PlanId } from '@anis/types';
import { Badge, Button, Card, Logo } from '@anis/ui';
import { useCallback, useEffect, useState } from 'react';

import { formatNumber, type StringKey } from '../i18n.js';
import { useLanguage } from '../lib/LanguageContext.js';
import { pb } from '../lib/pocketbase.js';
import { useSignOut } from '../lib/useAuth.js';
import { AnalyticsScreen } from './AnalyticsScreen.js';
import { BillingScreen } from './BillingScreen.js';
import { ClientsScreen, type Roster } from './ClientsScreen.js';
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
        // PocketBase's client wraps a non-2xx body; its `message` holds our
        // handler's error (e.g. the plan ceiling), which is safe to show.
        const msg =
          err && typeof err === 'object' && 'message' in err
            ? String((err as { message: unknown }).message)
            : String(err);
        setCreateError(msg);
        return false;
      } finally {
        setCreating(false);
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

      {workspace && tab === 'inbox' && <InboxScreen workspaceId={workspace.id} plan={plan} />}

      {workspace && tab === 'analytics' && <AnalyticsScreen workspaceId={workspace.id} />}

      {tab === 'billing' && <BillingScreen />}

      {workspace && tab === 'workspace' && (
        <>
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
        </>
      )}
    </main>
  );
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
