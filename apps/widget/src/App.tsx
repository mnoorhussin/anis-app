/**
 * Widget shell — the launcher bubble and the panel it opens.
 *
 * Scaffold state: the panel renders the configured greeting and suggested
 * questions, and the composer is disabled with an honest label. It does not
 * fake a conversation. A demo that answers from nothing is precisely the
 * behaviour this product is being built to avoid.
 */

import { REFUSAL_TEMPLATE, detectLanguage, type SupportedLanguage } from '@anis/types';
import type { ThemeName } from '@anis/tokens';
import { useState } from 'preact/hooks';

import type { PublicWidgetConfig } from './api.js';

interface Props {
  apiUrl: string;
  widgetKey: string;
  config: PublicWidgetConfig;
  theme: ThemeName;
}

function initialLanguage(config: PublicWidgetConfig): SupportedLanguage {
  if (config.language !== 'auto') return config.language;
  // Before the visitor has typed anything, the host page's own language is the
  // best available signal.
  const pageLang = document.documentElement.lang?.slice(0, 2).toLowerCase();
  return pageLang === 'ar' ? 'ar' : 'en';
}

export function App({ config, theme: _theme }: Props) {
  const [open, setOpen] = useState(false);
  const [lang, setLang] = useState<SupportedLanguage>(() => initialLanguage(config));
  const [draft, setDraft] = useState('');

  const dir = lang === 'ar' ? 'rtl' : 'ltr';
  const greeting = config.greeting[lang];
  const suggestions = config.suggestedQuestions[lang];

  // Re-detect from what the visitor is typing, so the panel flips direction as
  // soon as they switch language rather than after they send.
  function onInput(value: string) {
    setDraft(value);
    if (config.language === 'auto' && value.trim().length > 2) {
      setLang(detectLanguage(value, lang).language);
    }
  }

  if (!open) {
    return (
      <button
        type="button"
        onClick={() => setOpen(true)}
        aria-label={lang === 'ar' ? 'افتح المحادثة' : 'Open chat'}
        class="flex h-14 w-14 items-center justify-center rounded-full bg-accent text-accent-contrast shadow-lg transition-transform duration-200 ease-out-quint hover:scale-105 focus-visible:outline-2 focus-visible:outline-offset-2"
      >
        <svg viewBox="0 0 24 24" class="h-6 w-6" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M21 11.5a8.4 8.4 0 0 1-9 8.4 9.9 9.9 0 0 1-4.2-.9L3 21l1.9-4.6A8.4 8.4 0 0 1 12 3a8.4 8.4 0 0 1 9 8.5Z" />
        </svg>
      </button>
    );
  }

  return (
    <div
      dir={dir}
      lang={lang}
      class="flex h-[min(34rem,80vh)] w-[min(24rem,calc(100vw-2rem))] flex-col overflow-hidden rounded-2xl border border-border-soft bg-surface shadow-2xl"
    >
      <header class="flex items-center gap-3 border-b border-border-soft px-4 py-3">
        <span class="grow truncate font-medium text-foreground">{config.name}</span>
        <button
          type="button"
          onClick={() => setOpen(false)}
          aria-label={lang === 'ar' ? 'إغلاق' : 'Close'}
          class="rounded-lg p-1 text-muted hover:bg-surface-2"
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

      <div class="flex grow flex-col gap-3 overflow-y-auto p-4">
        {/* dir="auto" on message content: a single conversation can hold both
            languages, and the browser's bidi resolution beats ours per bubble. */}
        <p
          dir="auto"
          class="max-w-[85%] rounded-2xl bg-surface-2 px-3 py-2 text-sm text-foreground"
        >
          {greeting}
        </p>

        {suggestions.length > 0 && (
          <ul class="flex flex-wrap gap-2">
            {suggestions.map((q) => (
              <li key={q}>
                <button
                  type="button"
                  dir="auto"
                  disabled
                  class="rounded-full border border-border-soft px-3 py-1 text-xs text-muted disabled:opacity-60"
                >
                  {q}
                </button>
              </li>
            ))}
          </ul>
        )}

        {/* The refusal string is shown here so the scaffold demonstrates the
            behaviour the product is required to have, rather than implying an
            answering assistant that does not exist yet. */}
        <p dir="auto" class="max-w-[85%] rounded-2xl bg-surface-2 px-3 py-2 text-sm text-muted">
          {REFUSAL_TEMPLATE[lang]}
        </p>
      </div>

      <div class="border-t border-border-soft p-3">
        <input
          value={draft}
          dir="auto"
          onInput={(e) => onInput((e.target as HTMLInputElement).value)}
          placeholder={lang === 'ar' ? 'غير متاح بعد' : 'Not wired up yet'}
          disabled
          class="w-full rounded-xl bg-surface-2 px-3 py-2 text-sm text-foreground placeholder:text-muted disabled:cursor-not-allowed"
        />
        {config.badgeOn && (
          <p class="mt-2 text-center text-[11px] text-muted">
            {lang === 'ar' ? 'مدعوم بواسطة أنيس' : 'Powered by Anis'}
          </p>
        )}
      </div>
    </div>
  );
}
