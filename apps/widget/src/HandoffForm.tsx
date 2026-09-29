import type { SupportedLanguage } from '@anis/types';
import { useState } from 'preact/hooks';

import { requestHandoff } from './api.js';

const COPY = {
  ar: {
    intro: 'يسعدنا إيصال سؤالك إلى الفريق. اترك وسيلة تواصل ليتمكّن أحد الموظفين من الرد عليك.',
    name: 'الاسم (اختياري)',
    contact: 'البريد الإلكتروني أو رقم الجوال',
    submit: 'أرسل',
    sending: 'جارٍ الإرسال…',
    cancel: 'لا، شكراً',
    // Says a person will follow up, NOT that one is waiting now. We cannot
    // detect who is online, and implying a live agent that does not exist is
    // the kind of promise this product is built to avoid.
    sent: 'شكرًا لك. وصل طلبك إلى الفريق، ويمكنهم التواصل معك عبر البيانات التي أرسلتها.',
    failed: 'تعذّر الإرسال. حاول مرة أخرى.',
    needContact: 'نحتاج بريدك أو رقمك للتواصل معك.',
  },
  en: {
    intro: 'We can pass your question to the team. Leave your contact details so they can reply.',
    name: 'Name (optional)',
    contact: 'Email or phone number',
    submit: 'Send',
    sending: 'Sending…',
    cancel: 'No thanks',
    sent: 'Thank you. We’ve received your request. The team can reach you using the details you shared.',
    failed: "That didn't send. Please try again.",
    needContact: 'We need an email or phone number to reach you.',
  },
} as const;

export function HandoffForm({
  apiUrl,
  widgetKey,
  conversationId,
  question,
  lang,
  onDismiss,
}: {
  apiUrl: string;
  widgetKey: string;
  conversationId: string;
  question: string;
  lang: SupportedLanguage;
  onDismiss: () => void;
}) {
  const t = COPY[lang];
  const [name, setName] = useState('');
  const [contact, setContact] = useState('');
  const [state, setState] = useState<'idle' | 'sending' | 'sent' | 'failed'>('idle');
  const [error, setError] = useState<string | null>(null);

  async function submit(e: Event) {
    e.preventDefault();
    if (!contact.trim()) {
      setError(t.needContact);
      return;
    }
    setError(null);
    setState('sending');

    const ok = await requestHandoff(apiUrl, widgetKey, {
      conversationId,
      name: name.trim(),
      contact: contact.trim(),
      question,
    });
    setState(ok ? 'sent' : 'failed');
  }

  if (state === 'sent') {
    return (
      <p
        dir="auto"
        role="status"
        class="max-w-[85%] self-start rounded-2xl bg-surface-2 px-3 py-2 text-sm text-foreground"
      >
        {t.sent}
      </p>
    );
  }

  return (
    <form
      onSubmit={submit}
      dir="auto"
      class="flex flex-col gap-2 rounded-2xl border border-border-soft bg-surface-2 p-3"
    >
      <p class="text-xs text-muted">{t.intro}</p>

      <input
        value={name}
        onInput={(e) => setName((e.target as HTMLInputElement).value)}
        placeholder={t.name}
        aria-label={t.name}
        // A person's own name carries its own direction.
        dir="auto"
        autocomplete="name"
        class="rounded-lg bg-surface px-3 py-2 text-sm text-foreground outline-none placeholder:text-muted focus-visible:outline-2 focus-visible:outline-offset-2"
      />

      <input
        value={contact}
        onInput={(e) => setContact((e.target as HTMLInputElement).value)}
        placeholder={t.contact}
        aria-label={t.contact}
        // An email address or a phone number is always LTR, even on an Arabic
        // page: in an RTL run a "+966…" number renders with the plus on the
        // wrong end and reads as a different number.
        dir="ltr"
        required
        class="rounded-lg bg-surface px-3 py-2 text-start text-sm text-foreground outline-none placeholder:text-muted focus-visible:outline-2 focus-visible:outline-offset-2"
      />

      {(error || state === 'failed') && (
        <p role="alert" class="text-xs text-danger">
          {error ?? t.failed}
        </p>
      )}

      <div class="flex gap-2">
        <button
          type="submit"
          disabled={state === 'sending'}
          class="rounded-full bg-accent px-3 py-1.5 text-xs font-medium text-accent-contrast disabled:opacity-50 focus-visible:outline-2 focus-visible:outline-offset-2"
        >
          {state === 'sending' ? t.sending : t.submit}
        </button>
        <button
          type="button"
          onClick={onDismiss}
          class="rounded-full px-3 py-1.5 text-xs text-muted hover:text-foreground focus-visible:outline-2 focus-visible:outline-offset-2"
        >
          {t.cancel}
        </button>
      </div>
    </form>
  );
}
