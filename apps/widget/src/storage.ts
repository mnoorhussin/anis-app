/**
 * Per-widget browser storage.
 *
 * Every key is namespaced by the widget key. A page can carry two Anis widgets
 * (an agency demoing a client's assistant beside their own), and a developer
 * routinely has a local widget and a production one on the same origin —
 * sharing a conversation id between them would splice two businesses'
 * conversations together.
 *
 * All access is wrapped: Safari in private mode throws on localStorage rather
 * than returning null, and an exception here would take down a widget embedded
 * on a customer's storefront.
 */

const PREFIX = 'anis';

function read(key: string): string | null {
  try {
    return localStorage.getItem(key);
  } catch {
    return null;
  }
}

function write(key: string, value: string): void {
  try {
    localStorage.setItem(key, value);
  } catch {
    /* private mode; the value simply does not survive a reload */
  }
}

/**
 * A stable, opaque id for this browser.
 *
 * Deliberately random and meaningless: it groups a returning visitor's
 * conversations so an agent can see the history, and it is what the rate
 * limiter counts against. It is not a personal identifier, is never derived
 * from anything about the person, and carries nothing that could identify them
 * if the database were read.
 */
export function visitorId(widgetKey: string): string {
  const key = `${PREFIX}:visitor:${widgetKey}`;
  const existing = read(key);
  if (existing) return existing;

  const fresh =
    typeof crypto !== 'undefined' && 'randomUUID' in crypto
      ? crypto.randomUUID()
      : `v${Date.now().toString(36)}${Math.random().toString(36).slice(2, 10)}`;
  write(key, fresh);
  return fresh;
}

/** The conversation to continue, so a page reload does not start a new one. */
export function conversationId(widgetKey: string): string | null {
  return read(`${PREFIX}:conversation:${widgetKey}`);
}

export function setConversationId(widgetKey: string, id: string): void {
  write(`${PREFIX}:conversation:${widgetKey}`, id);
}
