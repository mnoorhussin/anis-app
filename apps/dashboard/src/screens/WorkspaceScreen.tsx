import { PLANS, type PlanId } from '@anis/types';
import { Badge, Button, Card, Logo } from '@anis/ui';
import { Fragment, useCallback, useEffect, useState } from 'react';

import { formatNumber, type StringKey } from '../i18n.js';
import { describeError } from '../lib/apiError.js';
import { type PendingInvite } from '../lib/invite.js';
import { useLanguage } from '../lib/LanguageContext.js';
import { pb } from '../lib/pocketbase.js';
import { isManager } from '../lib/roles.js';
import { useSignOut } from '../lib/useAuth.js';
import { AcceptInviteCard } from './AcceptInviteCard.js';
import { AnalyticsScreen } from './AnalyticsScreen.js';
import { BillingScreen } from './BillingScreen.js';
import { ClientsScreen, type Roster } from './ClientsScreen.js';
import { DeleteWorkspaceCard } from './DeleteWorkspaceCard.js';
import { InboxScreen } from './InboxScreen.js';
import { SourcesCard } from './SourcesCard.js';
import { TeamCard } from './TeamCard.js';

/**
 * The last workspace opened, per browser. Without it everyone lands on a
 * workspace they own — and every signup provisions one, so a client or agent
 * invited into an agency's workspace would open their own empty one each time.
 * A convenience only: if it is missing or stale the default applies.
 */
const ACTIVE_KEY = 'anis.activeWorkspace';
function storedActive(): string | null {
  try {
    return localStorage.getItem(ACTIVE_KEY);
  } catch {
    return null;
  }
}
function storeActive(id: string): void {
  try {
    localStorage.setItem(ACTIVE_KEY, id);
  } catch {
    // Blocked storage: the default selection still works.
  }
}

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

export function WorkspaceScreen({
  invite,
  onInviteDone,
}: {
  invite: PendingInvite | null;
  onInviteDone: () => void;
}) {
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
      r.shared ??= [];
      setRoster(r);
      const reachable = (id: string | null) =>
        !!id && (r.workspaces.some((w) => w.id === id) || r.shared.some((w) => w.id === id));
      setActiveId((prev) => {
        if (reachable(prev)) return prev;
        const remembered = storedActive();
        if (reachable(remembered)) return remembered;
        // Prefer a workspace the caller owns; then one shared with them.
        return (
          r.workspaces.find((w) => w.role === 'owner')?.id ??
          r.workspaces[0]?.id ??
          r.shared[0]?.id ??
          null
        );
      });
      return r;
    } catch (err) {
      setError(describeError(err));
      return null;
    }
  }, []);

  useEffect(() => {
    void loadRoster();
  }, [loadRoster]);

  useEffect(() => {
    if (activeId) storeActive(activeId);
  }, [activeId]);

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
        setCreateError(describeError(err));
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
        setDeleteError(describeError(err));
        return false;
      } finally {
        setDeleting(false);
      }
    },
    [loadRoster],
  );

  // Accepting an invitation lands the person in the workspace they joined.
  const acceptedInvite = useCallback(
    async (workspaceId: string) => {
      await loadRoster();
      setActiveId(workspaceId);
      setTab('workspace');
      onInviteDone();
    },
    [loadRoster, onInviteDone],
  );

  // Leaving a workspace someone else owns. The selection then falls back to
  // another workspace, as it does after a delete.
  const [leaveError, setLeaveError] = useState<string | null>(null);
  const leave = useCallback(
    async (workspaceId: string) => {
      const me = pb.authStore.record?.id;
      if (!me) return;
      setLeaveError(null);
      try {
        await pb.send(
          `/api/anis/workspaces/${encodeURIComponent(workspaceId)}/members/${encodeURIComponent(me)}`,
          { method: 'DELETE' },
        );
        await loadRoster();
        setTab('workspace');
      } catch (err) {
        setLeaveError(describeError(err));
      }
    },
    [loadRoster],
  );

  // Plan comes from the ACTIVE workspace's account, because that is the plan
  // whose features apply here. Preferring the caller's own account would be
  // wrong for a workspace they were invited into: an agency's agent, whose own
  // account is free, would see the agency's inbox without human takeover.
  // The roster's account plan is the fallback while the record loads.
  const plan: PlanId =
    workspace?.expand?.account?.plan ?? (roster?.account.plan as PlanId | undefined) ?? 'free';
  const domains = workspace?.allowed_domains ?? [];
  const multi = (roster?.account.max_workspaces ?? 1) > 1;
  const shared = roster?.shared ?? [];
  const switchable = (roster?.workspaces.length ?? 0) + shared.length > 1;
  // The caller's role here decides which controls are offered. The backend
  // enforces the same rules; this only avoids showing buttons it would refuse.
  const activeRole =
    roster?.workspaces.find((w) => w.id === activeId)?.role ??
    shared.find((w) => w.id === activeId)?.role;
  const canManage = isManager(activeRole);
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
                {shared.length === 0 ? (
                  roster.workspaces.map((w) => (
                    <option key={w.id} value={w.id}>
                      {w.name}
                    </option>
                  ))
                ) : (
                  <>
                    <optgroup label={t('yourWorkspaces')}>
                      {roster.workspaces.map((w) => (
                        <option key={w.id} value={w.id}>
                          {w.name}
                        </option>
                      ))}
                    </optgroup>
                    <optgroup label={t('sharedWithYou')}>
                      {shared.map((w) => (
                        <option key={w.id} value={w.id}>
                          {w.account_name ? `${w.name} · ${w.account_name}` : w.name}
                        </option>
                      ))}
                    </optgroup>
                  </>
                )}
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

      {invite && (
        <AcceptInviteCard invite={invite} onAccepted={acceptedInvite} onDismiss={onInviteDone} />
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
        <AnalyticsScreen
          key={`analytics-${workspace.id}`}
          workspaceId={workspace.id}
          canAnswerGaps={canManage}
        />
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
            {activeRole && activeRole !== 'owner' && (
              // Someone invited in can always leave. The owner cannot: their
              // membership is how the account and its billing are resolved.
              <div className="mt-4 flex flex-wrap items-center gap-3 border-t border-border-soft pt-4">
                {activeRole === 'agent' && (
                  <p className="grow text-sm text-muted">{t('agentNote')}</p>
                )}
                <Button
                  size="sm"
                  variant="ghost"
                  className="ms-auto"
                  onClick={() => void leave(workspace.id)}
                >
                  {t('leaveWorkspace')}
                </Button>
                {leaveError && (
                  <p role="alert" dir="auto" className="w-full text-sm text-status-danger">
                    {leaveError}
                  </p>
                )}
              </div>
            )}
          </Card>

          <SourcesCard workspaceId={workspace.id} canManage={canManage} />

          {canManage && <TeamCard workspaceId={workspace.id} />}

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
