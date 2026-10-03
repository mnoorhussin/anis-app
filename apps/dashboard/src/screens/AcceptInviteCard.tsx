import { Button, Card } from '@anis/ui';
import { useState } from 'react';

import { inviteLine } from '../i18n.js';
import { describeError } from '../lib/apiError.js';
import { type PendingInvite } from '../lib/invite.js';
import { useLanguage } from '../lib/LanguageContext.js';
import { pb } from '../lib/pocketbase.js';

/**
 * The signed-in side of an invitation link: say what it is, and join only when
 * the person presses Accept.
 *
 * Accepting is a deliberate click rather than something that happens on load.
 * Otherwise opening a link — in a preview pane, or one sent by someone else —
 * would be enough to add a person to a workspace they never chose to join.
 */
export function AcceptInviteCard({
  invite,
  onAccepted,
  onDismiss,
}: {
  invite: PendingInvite;
  onAccepted: (workspaceId: string) => Promise<void>;
  onDismiss: () => void;
}) {
  const { t, lang } = useLanguage();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const { preview, problem } = invite;

  async function accept() {
    setBusy(true);
    setError(null);
    try {
      const r = (await pb.send('/api/anis/invitations/accept', {
        method: 'POST',
        body: { token: invite.token },
      })) as { workspace: string };
      await onAccepted(r.workspace);
    } catch (err) {
      // The backend's reason is the useful part: "this invitation is for
      // x@… — sign in with that email", "this team is full", "expired".
      setError(describeError(err));
      setBusy(false);
    }
  }

  return (
    <Card className="ring-1 ring-accent/30">
      <h2 className="text-lg font-semibold">{t('inviteTitle')}</h2>
      {preview ? (
        <p dir="auto" className="mt-1 text-sm">
          {inviteLine(lang, preview.inviter, preview.workspace_name, preview.role)}
        </p>
      ) : (
        <p className="mt-1 text-sm text-muted">
          {problem === 'expired' ? t('inviteExpired') : t('inviteInvalid')}
        </p>
      )}
      {error && (
        <p role="alert" dir="auto" className="mt-2 text-sm text-status-danger">
          {error}
        </p>
      )}
      <div className="mt-4 flex gap-2">
        {preview && (
          <Button variant="primary" onClick={() => void accept()} disabled={busy}>
            {busy ? t('accepting') : t('acceptInvite')}
          </Button>
        )}
        <Button variant="ghost" onClick={onDismiss} disabled={busy}>
          {t('notNow')}
        </Button>
      </div>
    </Card>
  );
}
