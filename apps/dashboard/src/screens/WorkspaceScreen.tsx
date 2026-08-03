import { PLANS, type PlanId } from '@anis/types';
import { Badge, Button, Card, Logo } from '@anis/ui';
import { useEffect, useState } from 'react';

import { formatNumber } from '../i18n.js';
import { useLanguage } from '../lib/LanguageContext.js';
import { pb } from '../lib/pocketbase.js';
import { useSignOut } from '../lib/useAuth.js';

interface Workspace {
  id: string;
  name: string;
  widget_key: string;
  allowed_domains: string[];
  expand?: { account?: { plan: PlanId; name: string } };
}

const CDN_URL = import.meta.env['VITE_CDN_URL'] ?? 'https://cdn.anis.chat';

export function WorkspaceScreen() {
  const { t, lang, setLang } = useLanguage();
  const signOut = useSignOut();
  const [workspace, setWorkspace] = useState<Workspace | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    // No filter is passed. The workspace the caller can see is decided by the
    // collection's list rule server-side — a client-supplied workspace filter
    // would be a suggestion, not a boundary.
    pb.collection('workspaces')
      .getFirstListItem<Workspace>('', { expand: 'account' })
      .then((w) => {
        if (!cancelled) setWorkspace(w);
      })
      .catch((err: unknown) => {
        if (!cancelled) setError(err instanceof Error ? err.message : String(err));
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const plan = workspace?.expand?.account?.plan ?? 'free';
  const domains = workspace?.allowed_domains ?? [];

  return (
    <main className="mx-auto flex min-h-dvh w-full max-w-3xl flex-col gap-6 px-6 py-10">
      <header className="flex items-center justify-between gap-4">
        <Logo lang={lang} />
        <div className="flex items-center gap-2">
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

      {error && (
        <Card>
          <p role="alert" className="text-sm text-danger">
            {error}
          </p>
        </Card>
      )}

      {workspace && (
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

          <Card>
            <h2 className="font-display text-lg font-semibold">{t('installTitle')}</h2>
            <p className="mt-1 text-sm text-muted">{t('installLead')}</p>
            <InstallSnippet widgetKey={workspace.widget_key} />
          </Card>

          <Card>
            <h2 className="font-display text-lg font-semibold">{t('domainsTitle')}</h2>
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
