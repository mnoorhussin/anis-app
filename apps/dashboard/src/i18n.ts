/**
 * Dashboard copy, in both languages.
 *
 * A plain object rather than an i18n library. The dashboard's string count is
 * small and the interesting problem here is not pluralisation or
 * interpolation — it is direction, font and tone, which the token system
 * already handles. Reach for a library when message formatting genuinely needs
 * one, not before.
 *
 * Arabic is the primary market, so the Arabic string is written first and read
 * as the real copy; the English is its counterpart, not its source.
 */

import type { SupportedLanguage } from '@anis/types';

export const strings = {
  ar: {
    signIn: 'تسجيل الدخول',
    signUp: 'إنشاء حساب',
    signOut: 'تسجيل الخروج',
    email: 'البريد الإلكتروني',
    password: 'كلمة المرور',
    passwordConfirm: 'تأكيد كلمة المرور',
    name: 'الاسم',
    nameOptional: 'الاسم (اختياري)',
    createAccount: 'أنشئ حسابك',
    welcomeBack: 'أهلاً بعودتك',
    noAccount: 'ليس لديك حساب؟',
    haveAccount: 'لديك حساب بالفعل؟',
    workspace: 'مساحة العمل',
    plan: 'الباقة',
    installTitle: 'ثبّت أنيس على موقعك',
    installLead: 'انسخ هذا السطر وألصقه قبل إغلاق وسم body في موقعك.',
    copy: 'نسخ',
    copied: 'تم النسخ',
    domainsTitle: 'النطاقات المسموح بها',
    domainsEmpty: 'لم تضف أي نطاق بعد، لذلك لن تظهر أداة المحادثة على أي موقع. أضف نطاقك لتفعيلها.',
    notBuiltYet: 'غير متاح بعد',
    signingIn: 'جارٍ تسجيل الدخول…',
    creating: 'جارٍ الإنشاء…',
    passwordsDiffer: 'كلمتا المرور غير متطابقتين',
    passwordTooShort: 'كلمة المرور يجب أن تكون 8 أحرف على الأقل',
    signInFailed: 'تعذّر تسجيل الدخول. تأكد من البريد وكلمة المرور.',
    signUpFailed: 'تعذّر إنشاء الحساب.',
    emailTaken: 'هذا البريد مسجّل بالفعل.',
  },
  en: {
    signIn: 'Sign in',
    signUp: 'Create account',
    signOut: 'Sign out',
    email: 'Email',
    password: 'Password',
    passwordConfirm: 'Confirm password',
    name: 'Name',
    nameOptional: 'Name (optional)',
    createAccount: 'Create your account',
    welcomeBack: 'Welcome back',
    noAccount: "Don't have an account?",
    haveAccount: 'Already have an account?',
    workspace: 'Workspace',
    plan: 'Plan',
    installTitle: 'Install Anis on your site',
    installLead: 'Copy this line and paste it before the closing body tag on your site.',
    copy: 'Copy',
    copied: 'Copied',
    domainsTitle: 'Allowed domains',
    domainsEmpty:
      "You haven't added a domain yet, so the widget won't load anywhere. Add your domain to switch it on.",
    notBuiltYet: 'Not built yet',
    signingIn: 'Signing in…',
    creating: 'Creating…',
    passwordsDiffer: 'Passwords do not match',
    passwordTooShort: 'Password must be at least 8 characters',
    signInFailed: 'Could not sign in. Check your email and password.',
    signUpFailed: 'Could not create the account.',
    emailTaken: 'That email is already registered.',
  },
} as const satisfies Record<SupportedLanguage, Record<string, string>>;

export type StringKey = keyof (typeof strings)['en'];

/**
 * Format a quantity for display, in Western digits, in both languages.
 *
 * Deliberately locale-independent. `(4000).toLocaleString('ar')` yields
 * "4,000" in some browsers and "٤٬٠٠٠" in others — bare `ar` resolves to a
 * different default numbering system than `ar-EG` or `ar-SA` — so leaving it
 * to the runtime makes the same plan render differently for two customers.
 *
 * Western digits are also the right call on the merits: they dominate business
 * writing across the Gulf and the Levant, and these numbers sit beside USD
 * prices and an invoice that will certainly use them. Arabic-Indic digits
 * remain correct for Egypt and Sudan, so revisit this per-market rather than
 * assuming it is settled.
 */
export function formatNumber(n: number): string {
  return n.toLocaleString('en-US');
}

const STORAGE_KEY = 'anis-lang';

/** Remembered choice, else the browser's preference, else English. */
export function initialLanguage(): SupportedLanguage {
  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    if (stored === 'ar' || stored === 'en') return stored;
  } catch {
    /* private mode */
  }
  return navigator.language?.toLowerCase().startsWith('ar') ? 'ar' : 'en';
}

export function persistLanguage(lang: SupportedLanguage): void {
  try {
    localStorage.setItem(STORAGE_KEY, lang);
  } catch {
    /* private mode; the choice just won't survive a reload */
  }
}
