import { Button, Card } from '@anis/ui';
import { type FormEvent, useState } from 'react';

import { useLanguage } from '../lib/LanguageContext.js';

/**
 * Deleting a workspace, behind a typed confirmation.
 *
 * Typing the name rather than clicking "are you sure" is deliberate: for an
 * agency the workspaces are clients, often with similar names, and the switcher
 * makes it easy to be looking at a different one than intended. Typing the name
 * proves which one is about to go.
 *
 * Presentational like ClientsScreen — the request and the roster refresh live in
 * WorkspaceScreen. The parent remounts the whole workspace tab per workspace, so
 * switching resets a half-typed confirmation instead of carrying it over to
 * another one.
 */
export function DeleteWorkspaceCard({
  name,
  onDelete,
  deleting,
  error,
}: {
  name: string;
  onDelete: () => Promise<boolean>;
  deleting: boolean;
  error: string | null;
}) {
  const { t } = useLanguage();
  const [open, setOpen] = useState(false);
  const [typed, setTyped] = useState('');
  const confirmed = typed.trim() === name.trim();

  async function submit(e: FormEvent) {
    e.preventDefault();
    if (!confirmed || deleting) return;
    await onDelete();
  }

  return (
    <Card>
      <h2 className="text-lg font-semibold text-status-danger">{t('deleteWorkspaceTitle')}</h2>
      <p className="mt-1 max-w-prose text-sm text-muted">{t('deleteWorkspaceLead')}</p>
      <p className="mt-1 max-w-prose text-sm text-muted">{t('deleteKeepsUsage')}</p>

      {open ? (
        <form onSubmit={submit} className="mt-4 flex flex-col gap-3">
          <label>
            <span className="text-sm text-muted">{t('deleteConfirmLabel')}</span>{' '}
            {/* The name is the customer's, so its direction is theirs. */}
            <bdi className="text-sm font-semibold">{name}</bdi>
            <input
              dir="auto"
              value={typed}
              onChange={(e) => setTyped(e.target.value)}
              autoComplete="off"
              className="mt-1 w-full rounded-xl border border-border-soft bg-surface-2 px-3 py-2 text-sm text-foreground outline-none focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
            />
          </label>
          <div className="flex gap-2">
            <Button type="submit" variant="danger" disabled={!confirmed || deleting}>
              {deleting ? t('deleting') : t('deletePermanently')}
            </Button>
            <Button
              variant="ghost"
              onClick={() => {
                setOpen(false);
                setTyped('');
              }}
              disabled={deleting}
            >
              {t('cancel')}
            </Button>
          </div>
        </form>
      ) : (
        <div className="mt-4">
          <Button variant="secondary" onClick={() => setOpen(true)}>
            {t('deleteWorkspaceTitle')}
          </Button>
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
