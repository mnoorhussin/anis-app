import { detectLanguage, type SupportedLanguage } from '@anis/types';
import { useCallback, useRef, useState } from 'preact/hooks';

import { sendMessage } from './api.js';
import { conversationId, setConversationId, visitorId } from './storage.js';

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
  /**
   * True when the assistant could not answer and said so. Rendered
   * differently, because a refusal is a distinct outcome and not an error.
   */
  refused?: boolean;
}

export interface ConversationState {
  messages: Message[];
  /** True from send until the reply completes. */
  busy: boolean;
  /** Set when the request failed outright — a network problem, not a refusal. */
  failed: boolean;
  send: (text: string) => void;
  retry: () => void;
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
        { conversationId: convo.current, text: question, visitor: visitorId(widgetKey) },
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
            setConversationId(widgetKey, meta.conversationId);
            setMessages((prev) =>
              prev.map((m) =>
                m.id === replyId
                  ? { ...m, streaming: false, outcome: meta.outcome, refused: meta.offerHandoff }
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
    [apiUrl, widgetKey, fallbackLanguage, busy],
  );

  const retry = useCallback(() => {
    if (lastSent.current) send(lastSent.current);
  }, [send]);

  return { messages, busy, failed, send, retry };
}
