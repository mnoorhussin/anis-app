import { Button, Card, Logo } from '@anis/ui';
import { type FormEvent, useState } from 'react';

import { useLanguage } from '../lib/LanguageContext.js';
import { pb } from '../lib/pocketbase.js';

type Mode = 'signin' | 'signup';

/** PocketBase's minimum. Stated up front rather than after a failed submit. */
const MIN_PASSWORD = 8;

export function AuthScreen() {
  const { t, lang, setLang } = useLanguage();
  const [mode, setMode] = useState<Mode>('signin');
  const [email, setEmail] = useState('');
  const [name, setName] = useState('');
  const [password, setPassword] = useState('');
  const [confirm, setConfirm] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);

    if (mode === 'signup') {
      if (password.length < MIN_PASSWORD) return setError(t('passwordTooShort'));
      if (password !== confirm) return setError(t('passwordsDiffer'));
    }

    setBusy(true);
    try {
      if (mode === 'signup') {
        await pb.collection('users').create({
          email,
          password,
          passwordConfirm: confirm,
          name,
        });
        // The account, workspace and widget key are created server-side by a
        // hook on user creation, atomically with the user itself — so by the
        // time this resolves there is always somewhere to land.
      }
      await pb.collection('users').authWithPassword(email, password);
    } catch (err: unknown) {
      setError(describe(err, mode, t));
      setBusy(false);
    }
    // No setBusy(false) on success: the auth store change unmounts this
    // screen, and setting state on the way out logs a warning for nothing.
  }

  return (
    <main className="mx-auto flex min-h-dvh w-full max-w-md flex-col justify-center gap-6 px-6 py-12">
      <div className="flex items-center justify-between">
        <Logo lang={lang} />
        <Button
          size="sm"
          variant="ghost"
          onClick={() => setLang(lang === 'ar' ? 'en' : 'ar')}
          // The label is always in the language being switched TO, so it reads
          // as an offer rather than a description of the current state.
          lang={lang === 'ar' ? 'en' : 'ar'}
        >
          {lang === 'ar' ? 'English' : 'العربية'}
        </Button>
      </div>

      <Card>
        <h1 className="font-display text-2xl font-semibold">
          {mode === 'signin' ? t('welcomeBack') : t('createAccount')}
        </h1>

        <form onSubmit={onSubmit} className="mt-6 flex flex-col gap-4">
          {mode === 'signup' && (
            <Field
              label={t('nameOptional')}
              value={name}
              onChange={setName}
              autoComplete="name"
              // The one field that holds user-language content, so the browser
              // decides its direction from what is typed. An Arabic name in an
              // LTR box renders with its punctuation in the wrong place.
              dir="auto"
            />
          )}

          <Field
            label={t('email')}
            type="email"
            value={email}
            onChange={setEmail}
            autoComplete="email"
            required
            // Email is always LTR even on an Arabic page — an address in an
            // RTL run displays its dots and @ in a jumbled order.
            dir="ltr"
          />

          <Field
            label={t('password')}
            type="password"
            value={password}
            onChange={setPassword}
            autoComplete={mode === 'signin' ? 'current-password' : 'new-password'}
            required
            dir="ltr"
          />

          {mode === 'signup' && (
            <Field
              label={t('passwordConfirm')}
              type="password"
              value={confirm}
              onChange={setConfirm}
              autoComplete="new-password"
              required
              dir="ltr"
            />
          )}

          {error && (
            <p role="alert" className="text-sm text-status-danger">
              {error}
            </p>
          )}

          <Button type="submit" variant="primary" disabled={busy}>
            {busy
              ? mode === 'signin'
                ? t('signingIn')
                : t('creating')
              : mode === 'signin'
                ? t('signIn')
                : t('signUp')}
          </Button>
        </form>
      </Card>

      <p className="text-center text-sm text-muted">
        {mode === 'signin' ? t('noAccount') : t('haveAccount')}{' '}
        <button
          type="button"
          className="font-medium text-accent underline-offset-4 hover:underline"
          onClick={() => {
            setMode(mode === 'signin' ? 'signup' : 'signin');
            setError(null);
          }}
        >
          {mode === 'signin' ? t('signUp') : t('signIn')}
        </button>
      </p>
    </main>
  );
}

function Field({
  label,
  value,
  onChange,
  type = 'text',
  ...rest
}: {
  label: string;
  value: string;
  onChange: (v: string) => void;
  type?: string;
  required?: boolean;
  autoComplete?: string;
  dir?: 'ltr' | 'rtl' | 'auto';
}) {
  return (
    <label className="flex flex-col gap-1.5">
      <span className="text-sm font-medium text-muted">{label}</span>
      <input
        type={type}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className="rounded-xl border border-border-soft bg-surface-2 px-3 py-2 text-foreground outline-none focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
        {...rest}
      />
    </label>
  );
}

/**
 * Turn a PocketBase error into something a person can act on.
 *
 * Only "email already registered" is singled out, because it is the one signup
 * failure with an obvious next step. Everything else stays generic on purpose:
 * echoing the server's field-level messages leaks which emails exist.
 */
function describe(
  err: unknown,
  mode: Mode,
  t: (k: 'signInFailed' | 'signUpFailed' | 'emailTaken') => string,
): string {
  const data = (err as { response?: { data?: Record<string, { code?: string }> } })?.response?.data;
  if (mode === 'signup' && data?.['email']?.code === 'validation_invalid_email') {
    return t('emailTaken');
  }
  return mode === 'signin' ? t('signInFailed') : t('signUpFailed');
}
