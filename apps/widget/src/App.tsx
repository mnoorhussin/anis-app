/**
 * Widget shell — the launcher bubble and the conversation panel.
 */

import { detectLanguage, type SupportedLanguage } from '@anis/types';
import { ijamDots, type ThemeName } from '@anis/tokens';
import { useEffect, useRef, useState } from 'preact/hooks';

import type { PublicWidgetConfig } from './api.js';
import { HandoffForm } from './HandoffForm.js';
import { useConversation, type Message } from './useConversation.js';

interface Props {
  apiUrl: string;
  widgetKey: string;
  config: PublicWidgetConfig;
  theme: ThemeName;
}

const UI = {
  ar: {
    open: 'افتح المحادثة',
    close: 'إغلاق',
    send: 'إرسال',
    placeholder: 'اكتب سؤالك…',
    failed: 'تعذّر الإرسال. تحقق من اتصالك.',
    retry: 'إعادة المحاولة',
    poweredBy: 'مدعوم بواسطة أنيس',
    helpful: 'كانت مفيدة',
    notHelpful: 'لم تكن مفيدة',
    thanks: 'شكراً لملاحظتك',
    conversation: 'المحادثة',
    talkToPerson: 'تحدّث مع موظف',
    withHuman: 'أحد الموظفين يتابع محادثتك الآن.',
  },
  en: {
    open: 'Open chat',
    close: 'Close',
    send: 'Send',
    placeholder: 'Type your question…',
    failed: "That didn't send. Check your connection.",
    retry: 'Try again',
    poweredBy: 'Powered by Anis',
    helpful: 'This helped',
    notHelpful: 'This did not help',
    thanks: 'Thanks for the feedback',
    conversation: 'Conversation',
    talkToPerson: 'Talk to a person',
    withHuman: 'A member of the team is looking after this conversation.',
  },
} as const;

function initialLanguage(config: PublicWidgetConfig): SupportedLanguage {
  if (config.language !== 'auto') return config.language;
  // Before the visitor has typed, the host page's own language is the best
  // signal available.
  const pageLang = document.documentElement.lang?.slice(0, 2).toLowerCase();
  return pageLang === 'ar' ? 'ar' : 'en';
}

export function App({ apiUrl, widgetKey, config }: Props) {
  const [open, setOpen] = useState(false);
  const [uiLang, setUiLang] = useState<SupportedLanguage>(() => initialLanguage(config));
  const [draft, setDraft] = useState('');

  const { messages, conversationId, withHuman, lastQuestion, busy, failed, send, retry, rate } =
    useConversation(apiUrl, widgetKey, uiLang);
  const [handoff, setHandoff] = useState(false);
  const t = UI[uiLang];
  const dir = uiLang === 'ar' ? 'rtl' : 'ltr';

  const listRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  // Follow the conversation as it grows, including while a reply streams in.
  useEffect(() => {
    const el = listRef.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [messages]);

  // Focus the composer when the panel opens — a chat that needs a second click
  // before you can type is a chat people close.
  useEffect(() => {
    if (open) inputRef.current?.focus();
  }, [open]);

  function onInput(value: string) {
    setDraft(value);
    // Flip the interface as soon as the visitor's language is clear, not after
    // they send. `config.language` pins it when the business chose one.
    if (config.language === 'auto' && value.trim().length > 2) {
      setUiLang(detectLanguage(value, uiLang).language);
    }
  }

  function submit(e: Event) {
    e.preventDefault();
    if (!draft.trim() || busy) return;
    send(draft);
    setDraft('');
  }

  if (!open) {
    return (
      <button
        type="button"
        onClick={() => setOpen(true)}
        aria-label={t.open}
        class="flex h-14 w-14 items-center justify-center rounded-full bg-accent text-accent-contrast shadow-lg transition-transform duration-200 ease-out-quint hover:scale-105 focus-visible:outline-2 focus-visible:outline-offset-2"
      >
        <svg viewBox="0 0 24 24" class="h-6 w-6" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M21 11.5a8.4 8.4 0 0 1-9 8.4 9.9 9.9 0 0 1-4.2-.9L3 21l1.9-4.6A8.4 8.4 0 0 1 12 3a8.4 8.4 0 0 1 9 8.5Z" />
        </svg>
      </button>
    );
  }

  const showSuggestions = messages.length === 0 && config.suggestedQuestions[uiLang].length > 0;

  return (
    <div
      dir={dir}
      lang={uiLang}
      role="dialog"
      aria-label={config.name}
      class="flex h-[min(34rem,80vh)] w-[min(24rem,calc(100vw-2rem))] flex-col overflow-hidden rounded-2xl border border-border-soft bg-surface shadow-lifted"
    >
      <header class="flex items-center gap-3 bg-accent text-accent-contrast px-4 py-3">
        {config.logoUrl && <img src={config.logoUrl} alt="" class="h-6 w-6 rounded" />}
        <span dir="auto" class="grow truncate font-medium">
          {config.name}
        </span>
        <button
          type="button"
          onClick={() => setOpen(false)}
          aria-label={t.close}
          class="rounded-lg p-1 hover:bg-black/10 focus-visible:outline-2 focus-visible:outline-offset-2"
        >
          <svg
            viewBox="0 0 24 24"
            class="h-5 w-5"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
          >
            <path d="M18 6 6 18M6 6l12 12" />
          </svg>
        </button>
      </header>

      <div
        ref={listRef}
        // A log rather than an alert: new messages are announced without
        // stealing focus from the composer mid-conversation.
        role="log"
        aria-live="polite"
        aria-label={t.conversation}
        class="flex grow flex-col gap-3 overflow-y-auto p-4"
      >
        <Bubble
          message={{
            id: 'greeting',
            role: 'assistant',
            text: config.greeting[uiLang],
            language: uiLang,
          }}
        />

        {showSuggestions && (
          <ul class="flex flex-wrap gap-2">
            {config.suggestedQuestions[uiLang].map((q) => (
              <li key={q}>
                <button
                  type="button"
                  dir="auto"
                  disabled={busy}
                  onClick={() => send(q)}
                  class="rounded-full border border-border-soft px-3 py-1 text-xs text-muted transition-colors hover:bg-surface-2 hover:text-foreground disabled:opacity-50 focus-visible:outline-2 focus-visible:outline-offset-2"
                >
                  {q}
                </button>
              </li>
            ))}
          </ul>
        )}

        {messages.map((m) => (
          <Bubble key={m.id} message={m} lang={uiLang} onRate={rate} />
        ))}

        {/* The offer appears only after a genuine refusal, only when the
            plan allows it, and only once a conversation exists to attach it
            to. A person cannot be offered before there is anything to hand
            over. */}
        {!withHuman &&
          config.handoffEnabled &&
          conversationId &&
          messages.at(-1)?.refused &&
          (handoff ? (
            <HandoffForm
              apiUrl={apiUrl}
              widgetKey={widgetKey}
              conversationId={conversationId}
              question={lastQuestion}
              lang={uiLang}
              onDismiss={() => setHandoff(false)}
            />
          ) : (
            <div>
              <button
                type="button"
                onClick={() => setHandoff(true)}
                class="rounded-full border border-border-soft px-3 py-1.5 text-xs font-medium text-foreground hover:bg-surface-2 focus-visible:outline-2 focus-visible:outline-offset-2"
              >
                {t.talkToPerson}
              </button>
            </div>
          ))}

        {withHuman && (
          <p dir="auto" role="status" class="text-center text-xs text-muted">
            {t.withHuman}
          </p>
        )}

        {failed && (
          <div dir="auto" class="flex flex-col items-start gap-2">
            <p role="alert" class="text-xs text-muted">
              {t.failed}
            </p>
            <button
              type="button"
              onClick={retry}
              class="rounded-full border border-border-soft px-3 py-1 text-xs text-foreground hover:bg-surface-2 focus-visible:outline-2 focus-visible:outline-offset-2"
            >
              {t.retry}
            </button>
          </div>
        )}
      </div>

      <form onSubmit={submit} class="border-t border-border-soft p-3">
        <div class="flex items-end gap-2">
          <input
            ref={inputRef}
            value={draft}
            // The visitor's own text decides its direction, independently of
            // the panel: someone can type Arabic into an English interface.
            dir="auto"
            enterkeyhint="send"
            onInput={(e) => onInput((e.target as HTMLInputElement).value)}
            placeholder={t.placeholder}
            aria-label={t.placeholder}
            disabled={busy}
            class="w-full grow rounded-xl bg-surface-2 px-3 py-2 text-sm text-foreground outline-none placeholder:text-muted disabled:opacity-60 focus-visible:outline-2 focus-visible:outline-offset-2"
          />
          <button
            type="submit"
            disabled={busy || !draft.trim()}
            aria-label={t.send}
            class="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-accent text-accent-contrast transition-opacity disabled:opacity-40 focus-visible:outline-2 focus-visible:outline-offset-2"
          >
            {/* Mirrored in RTL: a send arrow must point the way the text runs. */}
            <svg
              viewBox="0 0 24 24"
              class="h-4 w-4 rtl:-scale-x-100"
              fill="none"
              stroke="currentColor"
              stroke-width="2"
            >
              <path d="M5 12h14M13 6l6 6-6 6" />
            </svg>
          </button>
        </div>

        {config.badgeOn && <p class="mt-2 text-center text-[11px] text-muted">{t.poweredBy}</p>}
      </form>
    </div>
  );
}

function Bubble({
  message,
  lang,
  onRate,
}: {
  message: Message;
  lang?: SupportedLanguage;
  onRate?: (messageId: string, serverId: string, rating: 'up' | 'down') => void;
}) {
  const mine = message.role === 'user';

  // A refusal is styled as an ordinary reply, not as an error. It is a correct
  // and expected outcome — the assistant declining to guess — and dressing it
  // in red would teach visitors that the product is broken when it is doing
  // precisely what it promises.
  const tone = mine
    ? 'bg-accent text-accent-contrast self-end'
    : 'bg-surface-2 text-foreground self-start';

  return (
    <div class={`flex flex-col ${mine ? 'items-end' : 'items-start'}`}>
      <p
        // dir="auto" per bubble, not per panel: one conversation routinely holds
        // an Arabic question and an English answer, and the browser resolves each
        // from its own first strong character better than we can guess.
        dir="auto"
        lang={message.language}
        class={`max-w-[85%] whitespace-pre-wrap rounded-2xl px-3 py-2 text-sm ${tone}`}
      >
        {message.text}
        {message.streaming && message.text === '' && <TypingDots />}
      </p>

      {/* Only an answered reply carries a server id, so only an answered reply
          is rateable. A refusal has nothing to call helpful, and rating a
          visitor's own message would be meaningless. */}
      {!mine && !message.streaming && message.serverId && onRate && lang && (
        <Rating message={message} lang={lang} onRate={onRate} />
      )}
    </div>
  );
}

function Rating({
  message,
  lang,
  onRate,
}: {
  message: Message;
  lang: SupportedLanguage;
  onRate: (messageId: string, serverId: string, rating: 'up' | 'down') => void;
}) {
  const t = UI[lang];

  if (message.rating) {
    return <span class="mt-1 text-[11px] text-muted">{t.thanks}</span>;
  }

  return (
    <span class="mt-1 flex gap-1">
      {(['up', 'down'] as const).map((r) => (
        <button
          key={r}
          type="button"
          // Labelled rather than relying on the icon alone: a thumb rotated
          // 180 degrees is not self-evident to a screen reader.
          aria-label={r === 'up' ? t.helpful : t.notHelpful}
          title={r === 'up' ? t.helpful : t.notHelpful}
          onClick={() => message.serverId && onRate(message.id, message.serverId, r)}
          class="rounded-full p-1 text-muted transition-colors hover:bg-surface-2 hover:text-foreground focus-visible:outline-2 focus-visible:outline-offset-2"
        >
          <svg
            viewBox="0 0 24 24"
            class={`h-3.5 w-3.5 ${r === 'down' ? 'rotate-180' : ''}`}
            fill="none"
            stroke="currentColor"
            stroke-width="2"
          >
            <path d="M7 10v11H4a1 1 0 0 1-1-1v-9a1 1 0 0 1 1-1h3Zm0 0 4.5-7a2 2 0 0 1 3.4 2l-1.2 5h5.1a2 2 0 0 1 2 2.4l-1.4 7A2 2 0 0 1 17.4 21H7" />
          </svg>
        </button>
      ))}
    </span>
  );
}

function TypingDots() {
  return (
    <svg viewBox="0 0 40 40" class="h-7 w-7 text-saffron" aria-hidden="true">
      {ijamDots.map((dot, i) => (
        <circle
          key={i}
          {...dot}
          fill="currentColor"
          class="animate-typing-dot"
          style={{ animationDelay: `${i * 0.15}s` }}
        />
      ))}
    </svg>
  );
}
