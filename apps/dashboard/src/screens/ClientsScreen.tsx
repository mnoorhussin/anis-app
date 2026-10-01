import { Badge, Button, Card } from '@anis/ui';
import { type FormEvent, useState } from 'react';

import { formatNumber } from '../i18n.js';
import { useLanguage } from '../lib/LanguageContext.js';

/** Mirrors the overview shape returned by GET /api/anis/workspaces. */
export interface WorkspaceStat {
  id: string;
  name: string;
  widget_key: string;
  role: string;
  sources: number;
  replies: number;
  conversations: number;
}

export interface AccountSummary {
  id: string;
  name: string;
  kind: string;
  plan: string;
  max_workspaces: number;
  slots_left: number;
  replies_used: number;
  replies_limit: number;
  client_workspaces: boolean;
}

export interface Roster {
  account: AccountSummary;
  workspaces: WorkspaceStat[];
}

const roleTone: Record<string, 'brand' | 'neutral'> = { owner: 'brand' };

/**
 * The account's workspace roster: one screen that lists every workspace, shows
 * how each is using the account's shared allowance, and lets the owner add
 * another where the plan allows it.
 *
 * Presentational by design — the roster, the active selection and the create
 * action all live in WorkspaceScreen, so the header switcher and this screen
 * can never disagree about which workspaces exist or which one is open.
 */
export function ClientsScreen({
  roster,
  activeId,
  onOpen,
  onCreate,
  creating,
  createError,
}: {
  roster: Roster;
  activeId: string | null;
  onOpen: (id: string) => void;
  onCreate: (name: string) => Promise<boolean>;
  creating: boolean;
  createError: string | null;
}) {
  const { t } = useLanguage();
  const [name, setName] = useState('');
  const { account, workspaces } = roster;
  const canCreate = account.slots_left > 0;

  async function submit(e: FormEvent) {
    e.preventDefault();
    const n = name.trim();
    if (!n || creating) return;
    if (await onCreate(n)) setName('');
  }

  const usedPct =
    account.replies_limit > 0
      ? Math.min(100, Math.round((account.replies_used / account.replies_limit) * 100))
      : 0;

  function roleLabel(role: string): string {
    if (role === 'owner') return t('roleOwner');
    if (role === 'admin') return t('roleAdmin');
    if (role === 'agent') return t('roleAgent');
    return role;
  }

  return (
    <div className="flex flex-col gap-6">
      <Card>
        <div className="flex flex-wrap items-start justify-between gap-4">
          <div>
            <h1 className="font-display text-xl font-semibold">{t('clientsTitle')}</h1>
            <p className="mt-1 max-w-prose text-sm text-muted">{t('clientsLead')}</p>
          </div>
          <div className="text-end">
            <p className="text-sm text-muted">{t('workspacesUsed')}</p>
            <p className="font-display text-lg font-semibold">
              {formatNumber(workspaces.length)} {t('ofLimit')}{' '}
              {formatNumber(account.max_workspaces)}
            </p>
          </div>
        </div>

        {/* Shared allowance — the whole point of the account model: these
            replies are pooled across every workspace below, not per-workspace. */}
        <div className="mt-5">
          <div className="flex items-center justify-between text-sm">
            <span className="text-muted">{t('sharedUsage')}</span>
            <span className="tabular-nums">
              {formatNumber(account.replies_used)} {t('ofLimit')}{' '}
              {formatNumber(account.replies_limit)}
            </span>
          </div>
          <div
            className="mt-2 h-2 overflow-hidden rounded-full bg-surface-3"
            role="progressbar"
            aria-valuenow={usedPct}
            aria-valuemin={0}
            aria-valuemax={100}
          >
            <div
              className={`h-full rounded-full ${usedPct >= 100 ? 'bg-danger' : 'bg-iris'}`}
              style={{ width: `${usedPct}%` }}
            />
          </div>
        </div>
      </Card>

      <ul className="flex flex-col gap-3">
        {workspaces.map((w) => {
          const isActive = w.id === activeId;
          return (
            <li key={w.id}>
              <Card>
                <div className="flex flex-wrap items-center justify-between gap-3">
                  <div className="min-w-0">
                    <div className="flex items-center gap-2">
                      {/* The name is the customer's, so its direction is theirs. */}
                      <h2 dir="auto" className="truncate font-display text-lg font-semibold">
                        {w.name}
                      </h2>
                      {w.role && (
                        <Badge tone={roleTone[w.role] ?? 'neutral'}>{roleLabel(w.role)}</Badge>
                      )}
                      {isActive && <Badge tone="success">{t('current')}</Badge>}
                    </div>
                    <p className="mt-1 text-sm text-muted tabular-nums">
                      {formatNumber(w.replies)} {t('repliesLabel')} ·{' '}
                      {formatNumber(w.conversations)} {t('convosLabel')} · {formatNumber(w.sources)}{' '}
                      {t('sourcesLabel')}
                    </p>
                  </div>
                  <Button
                    size="sm"
                    variant={isActive ? 'ghost' : 'secondary'}
                    onClick={() => onOpen(w.id)}
                    disabled={isActive}
                  >
                    {t('open')}
                  </Button>
                </div>
              </Card>
            </li>
          );
        })}
      </ul>

      <Card>
        <h2 className="font-display text-lg font-semibold">{t('newWorkspace')}</h2>
        {canCreate ? (
          <form onSubmit={submit} className="mt-3 flex flex-col gap-3 sm:flex-row sm:items-end">
            <label className="flex-1">
              <span className="text-sm text-muted">{t('workspaceName')}</span>
              <input
                dir="auto"
                value={name}
                onChange={(e) => setName(e.target.value)}
                maxLength={120}
                className="mt-1 w-full rounded-xl border border-border-soft bg-surface-2 px-3 py-2 text-sm text-foreground outline-none focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
              />
            </label>
            <Button type="submit" variant="primary" disabled={creating || name.trim() === ''}>
              {creating ? t('creating') : t('create')}
            </Button>
          </form>
        ) : (
          // Not an error: the plan's workspace allowance is used up. Say so
          // plainly and point at the upgrade, rather than showing a form that
          // would only be rejected by the backend.
          <p className="mt-2 flex items-start gap-2 text-sm text-muted">
            <Badge tone="warning">{formatNumber(account.max_workspaces)}</Badge>
            <span>{t('noSlotsLeft')}</span>
          </p>
        )}
        {createError && (
          <p role="alert" className="mt-3 text-sm text-danger">
            {createError}
          </p>
        )}
      </Card>
    </div>
  );
}
