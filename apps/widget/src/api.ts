/**
 * The widget's API client.
 *
 * The widget runs on domains we do not control and holds no user credentials.
 * Its `data-anis-key` is a PUBLIC key — it is visible in the page source of
 * every site that installs it, so it must not be treated as a secret. What
 * makes it safe is server-side enforcement:
 *
 *   - the backend checks the request `Origin` against the workspace's
 *     allow-list and refuses anything else;
 *   - rate limits are per key and per visitor;
 *   - the key grants exactly two things — read the widget's own public config,
 *     and post a message to its own workspace. Nothing else.
 *
 * Enforcement lives in the custom Go routes, not in PocketBase collection
 * rules, because custom routes bypass those rules entirely.
 */

import type { WidgetConfig } from '@anis/types';

/** Config safe to expose publicly. Never includes the knowledge base itself. */
export type PublicWidgetConfig = Pick<
  WidgetConfig,
  | 'name'
  | 'accentColor'
  | 'logoUrl'
  | 'greeting'
  | 'suggestedQuestions'
  | 'badgeOn'
  | 'theme'
  | 'position'
  | 'language'
>;

/**
 * Fetch the widget's configuration.
 *
 * Returns null on any failure — unknown key, disallowed origin, network error,
 * API down. The caller renders nothing. A customer's site must never show a
 * broken widget or an error message from us.
 */
export async function loadConfig(apiUrl: string, key: string): Promise<PublicWidgetConfig | null> {
  try {
    const res = await fetch(`${apiUrl}/api/anis/widget/${encodeURIComponent(key)}/config`, {
      method: 'GET',
      // No cookies. The widget is anonymous by design, and sending credentials
      // cross-site would need SameSite=None, which we do not want for an
      // embed on arbitrary third-party domains.
      credentials: 'omit',
      headers: { accept: 'application/json' },
    });
    if (!res.ok) return null;
    return (await res.json()) as PublicWidgetConfig;
  } catch {
    return null;
  }
}

export interface StreamCallbacks {
  onToken(text: string): void;
  /** Fired once when the reply is complete. */
  onDone(meta: { outcome: string; conversationId: string }): void;
  onError(): void;
}

/**
 * Send a message and stream the reply.
 *
 * Server-sent events rather than a WebSocket: the reply is one-directional and
 * short-lived, SSE survives proxies that mangle upgrades, and it reconnects on
 * its own. Note that Caddy must have response buffering disabled on this route
 * or the whole reply arrives at once — see deploy/Caddyfile.
 *
 * NOT IMPLEMENTED. The endpoint does not exist yet; this is the shape the
 * backend must provide, kept here so the two are designed together.
 */
export async function sendMessage(
  _apiUrl: string,
  _key: string,
  _body: { conversationId: string | null; text: string },
  callbacks: StreamCallbacks,
): Promise<void> {
  callbacks.onError();
  throw new Error('not implemented: POST /api/anis/widget/:key/message');
}
