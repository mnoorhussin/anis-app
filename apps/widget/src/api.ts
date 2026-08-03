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

export interface StreamDone {
  outcome: string;
  conversationId: string;
  /**
   * True when the assistant could not answer. The widget offers a person
   * rather than leaving the visitor at a dead end — which is the entire point
   * of refusing well rather than guessing.
   */
  offerHandoff: boolean;
}

export interface StreamCallbacks {
  onToken(text: string): void;
  /** Fired once when the reply is complete. */
  onDone(meta: StreamDone): void;
  onError(): void;
}

/**
 * Send a message and stream the reply.
 *
 * Server-sent events over `fetch`, not `EventSource`. EventSource cannot issue
 * a POST and cannot set headers, and the question has to go in a body — so the
 * SSE framing is parsed by hand here.
 *
 * The parser keeps a buffer across reads because a network chunk boundary
 * falls wherever TCP puts it, not on event boundaries: a single event can
 * arrive split down the middle, and two events can arrive together. Parsing
 * each read in isolation drops text intermittently and only under load, which
 * is the worst way to find out.
 */
export async function sendMessage(
  apiUrl: string,
  key: string,
  body: { conversationId: string | null; text: string; visitor: string },
  callbacks: StreamCallbacks,
): Promise<void> {
  let res: Response;
  try {
    res = await fetch(`${apiUrl}/api/anis/widget/${encodeURIComponent(key)}/message`, {
      method: 'POST',
      credentials: 'omit',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify(body),
    });
  } catch {
    callbacks.onError();
    return;
  }

  if (!res.ok || !res.body) {
    callbacks.onError();
    return;
  }

  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  let buffer = '';

  for (;;) {
    const { done, value } = await reader.read();
    if (done) break;

    // `stream: true` so a multi-byte character split across reads is held
    // back rather than decoded into a replacement character. Arabic is
    // two bytes per letter in UTF-8, so this is not a rare edge case here —
    // it is most letters.
    buffer += decoder.decode(value, { stream: true });

    let split: number;
    while ((split = buffer.indexOf('\n\n')) >= 0) {
      const block = buffer.slice(0, split);
      buffer = buffer.slice(split + 2);

      const event = /^event: (.+)$/m.exec(block)?.[1];
      const data = /^data: (.+)$/m.exec(block)?.[1];
      if (!event || !data) continue;

      try {
        const parsed: unknown = JSON.parse(data);
        if (event === 'token') {
          callbacks.onToken((parsed as { text: string }).text);
        } else if (event === 'done') {
          callbacks.onDone(parsed as StreamDone);
        }
      } catch {
        // A malformed event is skipped rather than aborting the stream: the
        // rest of the reply is still worth showing.
      }
    }
  }
}
