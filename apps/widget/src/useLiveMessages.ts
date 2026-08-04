import { detectLanguage } from '@anis/types';
import { useEffect } from 'preact/hooks';

import type { Message } from './useConversation.js';

export interface LiveEvent {
  kind: 'message' | 'status';
  id?: string;
  role?: 'user' | 'assistant' | 'human';
  text?: string;
  outcome?: string;
  status?: string;
  created?: string;
}

export interface LiveHandlers {
  onHistory: (messages: Message[]) => void;
  onMessage: (message: Message) => void;
  onStatus: (status: string) => void;
}

/**
 * Subscribe to a conversation so an agent's reply reaches the visitor.
 *
 * `EventSource` rather than the hand-rolled reader used for sending: this is a
 * GET with no body, which is what EventSource is for, and it reconnects on its
 * own after a dropped connection or a laptop waking from sleep. Replaying
 * history on every (re)connect is what makes that automatic reconnect safe —
 * the widget re-syncs rather than silently missing whatever arrived while it
 * was disconnected.
 */
export function useLiveMessages(
  apiUrl: string,
  widgetKey: string,
  conversationId: string | null,
  visitor: string,
  handlers: LiveHandlers,
): void {
  const { onHistory, onMessage, onStatus } = handlers;

  useEffect(() => {
    if (!conversationId) return;

    const url =
      `${apiUrl}/api/anis/widget/${encodeURIComponent(widgetKey)}/stream` +
      `?conversation=${encodeURIComponent(conversationId)}` +
      `&visitor=${encodeURIComponent(visitor)}`;

    let source: EventSource;
    try {
      source = new EventSource(url);
    } catch {
      // No EventSource, or the URL was rejected. Asking questions still works;
      // the visitor just will not see an agent's reply without a reload.
      return;
    }

    source.addEventListener('history', (e) => {
      try {
        const data = JSON.parse((e as MessageEvent<string>).data) as { messages: LiveEvent[] };
        onHistory(data.messages.map(toMessage).filter((m): m is Message => m !== null));
      } catch {
        /* malformed replay; the live stream is still usable */
      }
    });

    source.addEventListener('message', (e) => {
      try {
        const m = toMessage(JSON.parse((e as MessageEvent<string>).data) as LiveEvent);
        if (m) onMessage(m);
      } catch {
        /* skip a malformed event rather than tearing down the stream */
      }
    });

    source.addEventListener('status', (e) => {
      try {
        const ev = JSON.parse((e as MessageEvent<string>).data) as LiveEvent;
        if (ev.status) onStatus(ev.status);
      } catch {
        /* ignore */
      }
    });

    return () => source.close();

    // NOTE: `handlers` is deliberately absent from the dependency list. The
    // callbacks are stable state setters, and including them would close and
    // re-open the connection on every render — replaying history each time,
    // which reads as the conversation flickering.
  }, [apiUrl, widgetKey, conversationId, visitor]);
}

function toMessage(ev: LiveEvent): Message | null {
  if (!ev.id || !ev.text || !ev.role) return null;
  return {
    id: ev.id,
    // A human agent's reply is rendered exactly like the assistant's. From the
    // visitor's side it is one conversation, and labelling one bubble "a real
    // person" would raise the question of whether the others were.
    role: ev.role === 'user' ? 'user' : 'assistant',
    text: ev.text,
    language: detectLanguage(ev.text, 'en').language,
    // Spread rather than assigned: under exactOptionalPropertyTypes an
    // optional field cannot be handed an explicit undefined, so the key is
    // omitted when there is no outcome.
    ...(ev.outcome ? { outcome: ev.outcome } : {}),
    // Carried through the replay so a reconnect does not remove the handoff
    // offer from under a visitor who was about to press it.
    refused: ev.outcome === 'refused',
  };
}
