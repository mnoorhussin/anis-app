import { detectLanguage, type SupportedLanguage } from '@anis/types';
import { useCallback, useRef, useState } from 'preact/hooks';

import { rateMessage, sendMessage } from './api.js';
import { conversationId, setConversationId, visitorId } from './storage.js';
import { useLiveMessages } from './useLiveMessages.js';

export interface Message {
  id: string;
  role: 'user' | 'assistant';
  text: string;
  /** Detected per message: a conversation can hold both languages. */
  language: SupportedLanguage;
  /** True while tokens are still arriving. */
  streaming?: boolean;
  /** Set on the assistant's reply once complete. */
  outcome?: string;
  /** Server id, present only on an answered reply — refusals are not rateable. */
  serverId?: string;
  /** The rating this visitor gave, once they have given one. */
  rating?: 'up' | 'down';
  /**
   * True when the assistant could not answer and said so. Rendered
   * differently, because a refusal is a distinct outcome and not an error.
   */
  refused?: boolean;
}

export interface ConversationState {
  messages: Message[];
  /** The server-side conversation, once one exists. */
  conversationId: string | null;
  /** True once a person has taken the conversation over. */
  withHuman: boolean;
  /** The most recent visitor question, for the handoff form. */
  lastQuestion: string;
  /** True from send until the reply completes. */
  busy: boolean;
  /** Set when the request failed outright — a network problem, not a refusal. */
  failed: boolean;
  send: (text: string) => void;
  retry: () => void;
  rate: (messageId: string, serverId: string, rating: 'up' | 'down') => void;
}

let counter = 0;
const nextId = () => `m${++counter}`;

export function useConversation(
  apiUrl: string,
  widgetKey: string,
  fallbackLanguage: SupportedLanguage,
): ConversationState {
  const [messages, setMessages] = useState<Message[]>([]);
  const [busy, setBusy] = useState(false);
  const [failed, setFailed] = useState(false);
  const [withHuman, setWithHuman] = useState(false);
  const [convoId, setConvoId] = useState<string | null>(conversationId(widgetKey));
  const visitor = visitorId(widgetKey);

  // The server is the authority on what a conversation contains, so a replay
  // replaces the list outright — except for a reply still streaming in, which
  // has no server id yet and would otherwise vanish mid-sentence.
  const onHistory = useCallback((history: Message[]) => {
    setMessages((prev) => [...history, ...prev.filter((m) => m.streaming)]);
  }, []);

  // Deduplicated by id: EventSource reconnects on its own, and a replay after
  // a reconnect can overlap with an event already delivered.
  const onLiveMessage = useCallback((m: Message) => {
    setMessages((prev) => (prev.some((x) => x.id === m.id) ? prev : [...prev, m]));
  }, []);

  const onStatus = useCallback((status: string) => {
    setWithHuman(status === 'human' || status === 'escalated');
  }, []);

  useLiveMessages(apiUrl, widgetKey, convoId, visitor, {
    onHistory,
    onMessage: onLiveMessage,
    onStatus,
  });

  // Refs, not state: these change during a stream and must not each trigger a
  // re-render, and `send` must not be re-created on every token.
  const convo = useRef<string | null>(conversationId(widgetKey));
  const lastSent = useRef<string>('');

  const send = useCallback(
    (text: string) => {
      const question = text.trim();
      if (!question || busy) return;

      lastSent.current = question;
      setFailed(false);
      setBusy(true);

      const userLanguage = detectLanguage(question, fallbackLanguage).language;
      const replyId = nextId();

      setMessages((prev) => [
        ...prev,
        { id: nextId(), role: 'user', text: question, language: userLanguage },
        // The assistant's message is created empty and streaming, so the
        // typing indicator and the reply are the same element. Swapping one
        // for the other would make the reply jump position as it starts.
        { id: replyId, role: 'assistant', text: '', language: userLanguage, streaming: true },
      ]);

      void sendMessage(
        apiUrl,
        widgetKey,
        { conversationId: convo.current, text: question, visitor },
        {
          onToken(chunk) {
            setMessages((prev) =>
              prev.map((m) => {
                if (m.id !== replyId) return m;
                const text = m.text + chunk;
                return {
                  ...m,
                  text,
                  // Re-detect from the reply itself. The assistant answers in
                  // the visitor's language, but a visitor who writes English
                  // to an Arabic-only knowledge base gets an Arabic refusal —
                  // and that bubble has to lay out right-to-left.
                  language: detectLanguage(text, m.language).language,
                };
              }),
            );
          },
          onDone(meta) {
            convo.current = meta.conversationId;
            setConvoId(meta.conversationId);
            setConversationId(widgetKey, meta.conversationId);
            if (meta.withHuman) setWithHuman(true);
            setMessages((prev) =>
              prev
                // A conversation a person has taken over gets no reply at
                // all, so the empty placeholder is removed rather than left
                // as a blank bubble.
                .filter((m) => !(m.id === replyId && meta.withHuman))
                .map((m) =>
                  m.id === replyId
                    ? {
                      ...m,
                      streaming: false,
                      outcome: meta.outcome,
                      refused: meta.offerHandoff,
                      ...(meta.messageId ? { serverId: meta.messageId } : {}),
                    }
                    : m,
                ),
            );
            setBusy(false);
          },
          onError() {
            // Drop the empty placeholder rather than leaving a blank bubble.
            setMessages((prev) => prev.filter((m) => m.id !== replyId));
            setFailed(true);
            setBusy(false);
          },
        },
      );
    },
    [apiUrl, widgetKey, fallbackLanguage, busy, visitor],
  );

  const rate = useCallback(
    (messageId: string, serverId: string, rating: 'up' | 'down') => {
      if (!convo.current) return;
      // Applied optimistically. The rating is not worth a spinner, and a
      // failed call leaves the thumb shown as chosen rather than flickering
      // back — which would read as the tap not registering.
      setMessages((prev) => prev.map((m) => (m.id === messageId ? { ...m, rating } : m)));
      void rateMessage(apiUrl, widgetKey, {
        conversationId: convo.current,
        messageId: serverId,
        rating,
        visitor,
      });
    },
    [apiUrl, widgetKey, visitor],
  );

  const retry = useCallback(() => {
    if (lastSent.current) send(lastSent.current);
  }, [send]);

  return {
    messages,
    conversationId: convoId,
    withHuman,
    lastQuestion: lastSent.current,
    busy,
    failed,
    send,
    retry,
    rate,
  };
}
