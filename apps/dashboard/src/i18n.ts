/**
 * Dashboard copy, in both languages.
 *
 * A plain object rather than an i18n library. The string count is small, and
 * the one piece of real message formatting — Arabic's six plural categories —
 * is handled below with the built-in Intl.PluralRules. Reach for a library if
 * that stops being enough, not before.
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
    sourcesTitle: 'مصادر المعرفة',
    sourcesLead:
      'أنيس يجيب من هذه المصادر فقط. إذا لم يجد الإجابة، سيقولها بوضوح ويعرض التحويل إلى موظف.',
    sourcesEmpty: 'لا توجد مصادر بعد. أضف نصاً أو أسئلة شائعة ليبدأ أنيس بالإجابة.',
    addText: 'إضافة نص',
    addFaq: 'إضافة أسئلة شائعة',
    sourceTitleLabel: 'العنوان',
    sourceBodyLabel: 'المحتوى',
    question: 'السؤال',
    answer: 'الإجابة',
    addPair: 'إضافة سؤال آخر',
    save: 'حفظ',
    saving: 'جارٍ الحفظ…',
    cancel: 'إلغاء',
    delete: 'حذف',
    statusQueued: 'في الانتظار',
    statusFetching: 'جارٍ الجلب',
    statusProcessing: 'جارٍ المعالجة',
    statusReady: 'جاهز',
    statusFailed: 'فشل',
    statusStale: 'يحتاج تحديث',
    addWebsite: 'إضافة موقع',
    websiteUrlLabel: 'رابط الموقع',
    websiteHint: 'سنقرأ صفحات موقعك العامة فقط، ونحترم ملف robots.txt.',
    refresh: 'تحديث',
    pdfSoon: 'ملف PDF (قريباً)',
    crawlNote: 'قد يستغرق فحص الموقع بضع دقائق. يمكنك إغلاق الصفحة.',
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
    sourcesTitle: 'Knowledge sources',
    sourcesLead:
      'Anis answers only from these sources. When it cannot find an answer it says so and offers to pass the question to a person.',
    sourcesEmpty: 'No sources yet. Add some text or an FAQ so Anis has something to answer from.',
    addText: 'Add text',
    addFaq: 'Add FAQ',
    sourceTitleLabel: 'Title',
    sourceBodyLabel: 'Content',
    question: 'Question',
    answer: 'Answer',
    addPair: 'Add another question',
    save: 'Save',
    saving: 'Saving…',
    cancel: 'Cancel',
    delete: 'Delete',
    statusQueued: 'Queued',
    statusFetching: 'Fetching',
    statusProcessing: 'Processing',
    statusReady: 'Ready',
    statusFailed: 'Failed',
    statusStale: 'Needs refresh',
    addWebsite: 'Add website',
    websiteUrlLabel: 'Website address',
    websiteHint: 'We read only the public pages of your site, and we respect robots.txt.',
    refresh: 'Refresh',
    pdfSoon: 'PDF (soon)',
    crawlNote: 'Crawling a site can take a few minutes. You can close this page.',
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
 * Counted nouns, per plural category.
 *
 * Arabic has six: zero, one, two, few (3–10), many (11–99) and other (100+),
 * and they take different noun forms — "مقطع" for one, the dual "مقطعان" for
 * two, "مقاطع" for a few. Interpolating a number in front of a single form
 * produces "2 مقطع", which reads to an Arabic speaker exactly the way "2 item"
 * reads in English: like software that was translated rather than written.
 *
 * `Intl.PluralRules` knows the categories, so no library is needed — only the
 * right noun for each.
 */
const plurals = {
  ar: {
    passages: {
      zero: 'لا مقاطع',
      one: 'مقطع واحد',
      two: 'مقطعان',
      few: 'مقاطع',
      many: 'مقطعاً',
      other: 'مقطع',
    },
  },
  en: {
    passages: { one: 'passage', other: 'passages' },
  },
} as const;

export type PluralKey = keyof (typeof plurals)['en'];

/**
 * Render a counted noun, e.g. "3 مقاطع" or "1 passage".
 *
 * Arabic drops the numeral for one and two, because "مقطع واحد" and "مقطعان"
 * already carry the count — writing "1 مقطع واحد" is redundant in a way no
 * Arabic speaker would.
 */
export function plural(lang: SupportedLanguage, key: PluralKey, n: number): string {
  const category = new Intl.PluralRules(lang === 'ar' ? 'ar' : 'en').select(n);
  const forms = plurals[lang][key] as Record<string, string>;
  const noun = forms[category] ?? forms['other'] ?? '';

  if (lang === 'ar' && (category === 'one' || category === 'two' || category === 'zero')) {
    return noun;
  }
  return `${formatNumber(n)} ${noun}`;
}

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
