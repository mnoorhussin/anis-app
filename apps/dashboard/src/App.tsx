/**
 * Scaffold placeholder.
 *
 * Deliberately not a fake dashboard: the brief's golden rule is that nothing
 * may display a capability that does not exist, and a mock inbox with invented
 * numbers is exactly the thing that gets screenshotted and shipped. This
 * screen proves the wiring — tokens, fonts, RTL, and a live call to the
 * backend — and says plainly what is not built yet.
 */

import { Badge, Button, Card, Logo } from '@anis/ui';
import { PLAN_LIST, detectLanguage } from '@anis/types';
import { useEffect, useState } from 'react';

import { pb } from './lib/pocketbase.js';

type Health =
  { status: 'checking' } | { status: 'up'; version: string } | { status: 'down'; error: string };

function useBackendHealth(): Health {
  const [health, setHealth] = useState<Health>({ status: 'checking' });

  useEffect(() => {
    let cancelled = false;
    pb.health
      .check()
      .then((res) => {
        if (!cancelled) setHealth({ status: 'up', version: String(res.message ?? 'ok') });
      })
      .catch((err: unknown) => {
        if (!cancelled) {
          setHealth({ status: 'down', error: err instanceof Error ? err.message : String(err) });
        }
      });
    return () => {
      cancelled = true;
    };
  }, []);

  return health;
}

/** Proves the bidi setup end to end, including AR/EN code-switching. */
const SAMPLE_MESSAGES = [
  'كم تستغرق مدة التوصيل إلى الرياض؟',
  'Do you ship to France?',
  'عندكم iPhone 15 Pro بالمخزون؟',
] as const;

export function App() {
  const health = useBackendHealth();

  return (
    <main className="mx-auto flex min-h-dvh max-w-3xl flex-col gap-8 px-6 py-16">
      <header className="flex items-center justify-between">
        <Logo />
        <Badge
          tone={health.status === 'up' ? 'success' : health.status === 'down' ? 'danger' : 'info'}
        >
          {health.status === 'checking' && 'checking backend…'}
          {health.status === 'up' && 'backend up'}
          {health.status === 'down' && 'backend unreachable'}
        </Badge>
      </header>

      <Card>
        <h1 className="text-display-md text-gradient">Scaffold</h1>
        <p className="mt-2 text-muted">
          The monorepo is wired up. No product features are implemented yet — this screen exists to
          prove that tokens, fonts, bidi text and the PocketBase connection all work.
        </p>
        {health.status === 'down' && (
          <p className="mt-4 text-sm text-danger">
            {health.error}. Start the backend with{' '}
            <code className="font-mono">pnpm --filter @anis/pocketbase dev</code>.
          </p>
        )}
      </Card>

      <Card>
        <h2 className="mb-3 font-display text-lg font-semibold">Bidirectional text</h2>
        <ul className="flex flex-col gap-2">
          {SAMPLE_MESSAGES.map((text) => {
            const { language, mixed } = detectLanguage(text);
            return (
              <li
                key={text}
                // `dir="auto"` lets the browser decide from the first strong
                // character, which is more reliable than our own guess for the
                // text itself. The detected language is used for the label and
                // for choosing the font, not for layout.
                dir="auto"
                lang={language}
                className="flex items-center gap-3 rounded-xl bg-surface-2 px-4 py-2"
              >
                <span className="grow">{text}</span>
                <Badge tone="info">
                  {language}
                  {mixed ? ' · mixed' : ''}
                </Badge>
              </li>
            );
          })}
        </ul>
      </Card>

      <Card>
        <h2 className="mb-3 font-display text-lg font-semibold">Plans</h2>
        <p className="mb-3 text-sm text-muted">
          Read from <code className="font-mono">@anis/types</code>, the single source of truth the
          billing code enforces against.
        </p>
        <ul className="flex flex-wrap gap-2">
          {PLAN_LIST.map((plan) => (
            <li key={plan.id}>
              <Badge tone={plan.popular ? 'brand' : 'neutral'}>
                {plan.name} · ${plan.monthlyUsd} · {plan.limits.aiRepliesPerMonth.toLocaleString()}{' '}
                replies
              </Badge>
            </li>
          ))}
        </ul>
      </Card>

      <footer className="flex gap-3">
        <Button
          variant="primary"
          onClick={() => document.documentElement.setAttribute('data-theme', 'dark')}
        >
          Dark
        </Button>
        <Button
          variant="secondary"
          onClick={() => document.documentElement.setAttribute('data-theme', 'light')}
        >
          Light
        </Button>
      </footer>
    </main>
  );
}
