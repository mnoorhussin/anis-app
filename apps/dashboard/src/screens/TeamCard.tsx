import { Badge, Button, Card } from '@anis/ui';
import { type FormEvent, useCallback, useEffect, useState } from 'react';

import { formatNumber, type StringKey } from '../i18n.js';
import { describeError } from '../lib/apiError.js';
import { useLanguage } from '../lib/LanguageContext.js';
import { pb } from '../lib/pocketbase.js';
import { assignableRoles, canRemoveMember, type Role } from '../lib/roles.js';

interface Member {
  user_id: string;
  name: string;
  email: string;
  role: string;
  you: boolean;
}

interface Invitation {
  id: string;
  email: string;
  role: Role;
  expires: string;
  expired: boolean;
}

interface Team {
  your_role: string;
  /** False below Growth: the plan does not sell inviting people. */
  can_invite: boolean;
  members: Member[];
  invitations: Invitation[];
  seats: { used: number; limit: number };
}

interface Sent {
  email: string;
  link: string;
  emailed: boolean;
}

const roleKey: Record<string, StringKey> = {
  owner: 'roleOwner',
  admin: 'roleAdmin',
  agent: 'roleAgent',
};

/**
 * Who is in this workspace, who has been invited, and a form to invite more.
 * Shown to owners and admins only; the backend refuses the team list to agents
 * anyway.
 */
export function TeamCard({ workspaceId }: { workspaceId: string }) {
  const { t, lang } = useLanguage();
  const [team, setTeam] = useState<Team | null>(null);
  const [email, setEmail] = useState('');
  const [role, setRole] = useState<Role>('agent');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [sent, setSent] = useState<Sent | null>(null);
  const [copied, setCopied] = useState(false);

  const load = useCallback(() => {
    pb.send(`/api/anis/workspaces/${encodeURIComponent(workspaceId)}/team`, { method: 'GET' })
      .then((r) => setTeam(r as Team))
      .catch((err: unknown) => setError(describeError(err)));
  }, [workspaceId]);

  useEffect(load, [load]);

  async function invite(address: string, as: Role) {
    setBusy(true);
    setError(null);
    setSent(null);
    setCopied(false);
    try {
      const r = (await pb.send(
        `/api/anis/workspaces/${encodeURIComponent(workspaceId)}/invitations`,
        {
          method: 'POST',
          body: { email: address, role: as },
        },
      )) as { invitation: Invitation; link: string; emailed: boolean };
      setSent({ email: r.invitation.email, link: r.link, emailed: r.emailed });
      setEmail('');
      load();
    } catch (err) {
      setError(describeError(err));
    } finally {
      setBusy(false);
    }
  }

  async function act(path: string) {
    setError(null);
    try {
      await pb.send(path, { method: 'DELETE' });
      load();
    } catch (err) {
      setError(describeError(err));
    }
  }

  function submit(e: FormEvent) {
    e.preventDefault();
    if (email.trim()) void invite(email.trim(), role);
  }

  async function copy(link: string) {
    try {
      await navigator.clipboard.writeText(link);
      setCopied(true);
    } catch {
      // The link is shown and selectable; failing quietly is fine.
    }
  }

  if (!team) {
    return (
      <Card>
        <h2 className="text-lg font-semibold">{t('teamTitle')}</h2>
        {error ? (
          <p role="alert" className="mt-2 text-sm text-status-danger">
            {error}
          </p>
        ) : (
          <p className="mt-2 text-sm text-muted">…</p>
        )}
      </Card>
    );
  }

  const roles = assignableRoles(team.your_role);
  const full = team.seats.used >= team.seats.limit;
  const date = (s: string) =>
    // Western digits in both languages, matching formatNumber.
    new Date(s.replace(' ', 'T')).toLocaleDateString(lang === 'ar' ? 'ar-u-nu-latn' : 'en-GB', {
      day: 'numeric',
      month: 'short',
    });

  return (
    <Card>
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 className="text-lg font-semibold">{t('teamTitle')}</h2>
          <p className="mt-1 max-w-prose text-sm text-muted">{t('teamLead')}</p>
        </div>
        <div className="text-end">
          <p className="text-sm text-muted">{t('seatsLabel')}</p>
          <p className="font-display text-lg font-semibold tabular-nums">
            {formatNumber(team.seats.used)} {t('ofLimit')} {formatNumber(team.seats.limit)}
          </p>
        </div>
      </div>

      <ul className="mt-4 flex flex-col gap-2">
        {team.members.map((m) => (
          <li
            key={m.user_id}
            className="flex flex-wrap items-center gap-3 rounded-xl bg-surface-2 px-4 py-3"
          >
            <span className="grow">
              {m.name && (
                <span dir="auto" className="block font-medium">
                  {m.name}
                </span>
              )}
              <span dir="ltr" className="block text-sm text-muted">
                {m.email}
              </span>
            </span>
            {m.you && <Badge tone="success">{t('you')}</Badge>}
            <Badge tone={m.role === 'owner' ? 'brand' : 'neutral'}>
              {t(roleKey[m.role] ?? 'roleAgent')}
            </Badge>
            {!m.you && canRemoveMember(team.your_role, m.role, false) && (
              <Button
                size="sm"
                variant="ghost"
                onClick={() =>
                  void act(
                    `/api/anis/workspaces/${encodeURIComponent(workspaceId)}/members/${encodeURIComponent(m.user_id)}`,
                  )
                }
              >
                {t('removeMember')}
              </Button>
            )}
          </li>
        ))}
      </ul>

      {team.invitations.length > 0 && (
        <>
          <h3 className="mt-5 text-sm font-semibold">{t('pendingInvites')}</h3>
          <ul className="mt-2 flex flex-col gap-2">
            {team.invitations.map((inv) => (
              <li
                key={inv.id}
                className="flex flex-wrap items-center gap-3 rounded-xl border border-border-soft px-4 py-2.5"
              >
                <span dir="ltr" className="grow text-sm">
                  {inv.email}
                </span>
                <Badge tone="neutral">{t(roleKey[inv.role] ?? 'roleAgent')}</Badge>
                {inv.expired ? (
                  <Badge tone="warning">{t('expired')}</Badge>
                ) : (
                  <span className="text-xs text-muted">
                    {t('expires')} {date(inv.expires)}
                  </span>
                )}
                {roles.includes(inv.role) && (
                  <Button
                    size="sm"
                    variant="ghost"
                    disabled={busy}
                    onClick={() => void invite(inv.email, inv.role)}
                  >
                    {t('resend')}
                  </Button>
                )}
                <Button
                  size="sm"
                  variant="ghost"
                  onClick={() => void act(`/api/anis/invitations/${encodeURIComponent(inv.id)}`)}
                >
                  {t('revoke')}
                </Button>
              </li>
            ))}
          </ul>
        </>
      )}

      {roles.length > 0 && !team.can_invite && (
        // Said up front, rather than a form whose submit would only come back
        // as a plan error.
        <p className="mt-5 flex items-start gap-2 text-sm text-muted">
          <Badge tone="brand">Growth</Badge>
          <span>{t('invitesNeedGrowth')}</span>
        </p>
      )}

      {roles.length > 0 &&
        team.can_invite &&
        (full ? (
          <p className="mt-5 flex items-start gap-2 text-sm text-muted">
            <Badge tone="warning">{formatNumber(team.seats.limit)}</Badge>
            <span>{t('seatsFull')}</span>
          </p>
        ) : (
          <form onSubmit={submit} className="mt-5 flex flex-col gap-3 sm:flex-row sm:items-end">
            <label className="flex-1">
              <span className="text-sm text-muted">{t('email')}</span>
              <input
                type="email"
                required
                dir="ltr"
                autoComplete="off"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                className="mt-1 w-full rounded-xl border border-border-soft bg-surface-2 px-3 py-2 text-sm text-foreground outline-none focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
              />
            </label>
            <label>
              <span className="text-sm text-muted">{t('roleLabel')}</span>
              <select
                value={role}
                onChange={(e) => setRole(e.target.value as Role)}
                className="mt-1 block w-full rounded-xl border border-border-soft bg-surface-2 px-3 py-2 text-sm text-foreground outline-none focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
              >
                {roles.map((r) => (
                  <option key={r} value={r}>
                    {t(roleKey[r] ?? 'roleAgent')}
                  </option>
                ))}
              </select>
            </label>
            <Button type="submit" variant="primary" disabled={busy || !email.trim()}>
              {busy ? t('sendingInvite') : t('sendInvite')}
            </Button>
          </form>
        ))}

      {sent && (
        <div role="status" className="mt-4 rounded-xl bg-surface-2 px-4 py-3 text-sm">
          <p>
            {t('inviteSentTo')} <bdi dir="ltr">{sent.email}</bdi>
          </p>
          {!sent.emailed && (
            // Mail failing is reported, not hidden: the inviter can still get
            // the link to the person another way, and it only works for them.
            <>
              <p className="mt-1 text-muted">{t('inviteNotEmailed')}</p>
              <div className="mt-2 flex flex-wrap items-center gap-2">
                <code
                  dir="ltr"
                  className="grow overflow-x-auto rounded-lg bg-surface-3 px-2 py-1 font-mono text-xs"
                >
                  {sent.link}
                </code>
                <Button size="sm" variant="secondary" onClick={() => void copy(sent.link)}>
                  {copied ? t('copied') : t('copy')}
                </Button>
              </div>
            </>
          )}
        </div>
      )}

      {error && (
        <p role="alert" dir="auto" className="mt-3 text-sm text-status-danger">
          {error}
        </p>
      )}
    </Card>
  );
}
