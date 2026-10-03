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
    tabWorkspace: 'مساحة العمل',
    tabInbox: 'صندوق الوارد',
    tabBilling: 'الاشتراك',
    tabAnalytics: 'التحليلات',
    tabClients: 'العملاء',
    clientsTitle: 'مساحات العمل',
    clientsLead:
      'أنشئ مساحة عمل مستقلة لكل عميل، وتابع استهلاك كل واحدة من رصيد حسابك المشترك من مكان واحد.',
    sharedUsage: 'الرصيد المشترك',
    workspacesUsed: 'مساحات العمل',
    newWorkspace: 'مساحة عمل جديدة',
    workspaceName: 'اسم مساحة العمل',
    create: 'إنشاء',
    open: 'فتح',
    current: 'الحالية',
    noSlotsLeft: 'استنفدت عدد مساحات العمل في باقتك. رقِّ الباقة لإضافة المزيد.',
    repliesLabel: 'ردود',
    convosLabel: 'محادثات',
    sourcesLabel: 'مصادر',
    switchWorkspace: 'تبديل مساحة العمل',
    roleOwner: 'مالك',
    roleAdmin: 'مشرف',
    roleAgent: 'وكيل',
    deleteWorkspaceTitle: 'حذف مساحة العمل',
    deleteWorkspaceLead:
      'تُحذف مساحة العمل نهائياً مع كل مصادرها ومحادثاتها وعملائها المحتملين، ويتوقف المساعد عن العمل على كل المواقع التي يظهر فيها. لا يمكن التراجع عن ذلك.',
    deleteKeepsUsage: 'الردود المستهلكة هذا الشهر تبقى محسوبة من رصيدك المشترك.',
    deleteConfirmLabel: 'للتأكيد، اكتب اسم مساحة العمل:',
    deletePermanently: 'احذفها نهائياً',
    deleting: 'جارٍ الحذف…',
    teamTitle: 'الفريق',
    teamLead:
      'ادعُ عميلك أو زملاءك إلى مساحة العمل هذه. المشرف يدير المحتوى والإعدادات، والوكيل يتابع المحادثات ويرد عليها.',
    seatsLabel: 'المقاعد',
    invitesNeedGrowth: 'دعوة أعضاء الفريق متاحة بدءاً من باقة النمو.',
    roleLabel: 'الدور',
    sendInvite: 'إرسال الدعوة',
    sendingInvite: 'جارٍ الإرسال…',
    inviteSentTo: 'أُرسلت الدعوة إلى',
    inviteNotEmailed:
      'تعذّر إرسال البريد. انسخ الرابط وأرسله بنفسك — لا يعمل إلا لهذا البريد، وتنتهي صلاحيته بعد سبعة أيام.',
    pendingInvites: 'دعوات بانتظار القبول',
    expires: 'تنتهي',
    expired: 'منتهية',
    resend: 'إعادة الإرسال',
    revoke: 'إلغاء',
    removeMember: 'إزالة',
    leaveWorkspace: 'مغادرة',
    you: 'أنت',
    seatsFull: 'كل المقاعد في باقتك مشغولة. أزل عضواً أو ألغِ دعوة، أو رقِّ الباقة.',
    inviteTitle: 'دعوة للانضمام',
    acceptInvite: 'قبول الدعوة',
    accepting: 'جارٍ القبول…',
    notNow: 'ليس الآن',
    inviteUseEmail: 'سجّل الدخول أو أنشئ حساباً بهذا البريد:',
    inviteExpired: 'انتهت صلاحية هذه الدعوة. اطلب من مُرسلها دعوة جديدة.',
    inviteInvalid: 'رابط الدعوة غير صالح أو أُلغي.',
    yourWorkspaces: 'مساحاتك',
    sharedWithYou: 'انضممت إليها',
    agentNote:
      'دورك هنا وكيل: تتابع المحادثات وترد عليها، ويدير المالك والمشرفون المحتوى والإعدادات.',
    mConversations: 'المحادثات',
    mAutoResolved: 'أُجيبت تلقائياً',
    mEscalated: 'مُحوَّلة',
    mLeads: 'بيانات تواصل',
    mAnswered: 'إجابات من المصادر',
    mUnanswered: 'أسئلة بلا إجابة',
    mFirstResponse: 'وسيط زمن أول رد',
    mHelpful: 'مفيدة / غير مفيدة',
    howResolved: 'على أي أساس اعتُبرت مُجابة',
    howResolvedHelp:
      'لا تُحتسب المحادثة مُجابة لمجرد أن الزائر أغلق النافذة. هذه هي الإشارات الفعلية.',
    noResolutionsYet: 'لا توجد إشارات بعد. تظهر عندما يقيّم الزوار الإجابات.',
    sigRatedHelpful: 'قيّمها الزائر مفيدة',
    sigUserConfirmed: 'أكّد الزائر',
    sigActionCompleted: 'اكتمل إجراء',
    sigEndedAnswered: 'انتهت بعد إجابة',
    languagesTitle: 'اللغات',
    gapsTitle: 'أسئلة بلا إجابة',
    gapsHelp: 'أسئلة لم تجد لها المصادر إجابة. أضف الإجابة لتصبح جزءاً من المعرفة فوراً.',
    gapsEmpty: 'لا توجد أسئلة بلا إجابة.',
    gapAnswer: 'أضف إجابة',
    gapAnswerPlaceholder: 'اكتب الإجابة كما تودّ أن يقولها أنيس…',
    gapSaveToKb: 'أضف إلى المعرفة',
    gapNearMiss: 'قريبة',
    metricsHonesty:
      'نعرض فقط ما نستطيع قياسه فعلياً. لا نُدرج «أهم المواضيع» لأننا لا نحلّلها بعد.',
    currentPlan: 'باقتك الحالية',
    repliesUsed: 'الردود المستخدمة',
    ofLimit: 'من',
    usageWarning: 'اقتربت من حد باقتك.',
    usageExceeded: 'تجاوزت حد باقتك. الردود الإضافية تُحتسب حتى سقف الإنفاق.',
    spendingCap: 'سقف الإنفاق الإضافي',
    spendingCapHelp:
      'عند بلوغ هذا المبلغ يتوقف أنيس عن الرد ويعرض التحويل إلى موظف. اضبطه على صفر لمنع أي تكلفة إضافية.',
    saveCap: 'حفظ',
    capSaved: 'تم الحفظ',
    upgrade: 'ترقية',
    managePlan: 'إدارة الاشتراك',
    billingNotConfigured: 'الدفع غير مُفعّل على هذا الخادم.',
    ownerOnly: 'إدارة الاشتراك متاحة لمالك الحساب فقط.',
    statusPastDue: 'هناك مشكلة في الدفع. سنعيد المحاولة قبل تغيير باقتك.',
    cancelsOn: 'ينتهي الاشتراك في',
    overageSoFar: 'التكلفة الإضافية حتى الآن',
    inboxEmpty: 'لا توجد محادثات بعد.',
    needsAttention: 'يحتاج انتباه',
    statusActive: 'نشطة',
    statusAutoResolved: 'أُجيبت تلقائياً',
    statusEscalated: 'محوَّلة',
    statusHuman: 'مع موظف',
    statusClosed: 'مغلقة',
    takeOver: 'تولَّ المحادثة',
    returnToAuto: 'أعِد إلى أنيس',
    markResolved: 'إغلاق',
    replyPlaceholder: 'اكتب ردك…',
    sendReply: 'إرسال',
    takeoverNotOnPlan: 'الرد المباشر متاح في باقة النمو وما فوق.',
    leadContact: 'بيانات التواصل',
    selectConversation: 'اختر محادثة لعرضها.',
    aiPaused: 'أنيس متوقف عن الرد في هذه المحادثة.',
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
    tabWorkspace: 'Workspace',
    tabInbox: 'Inbox',
    tabBilling: 'Billing',
    tabAnalytics: 'Analytics',
    tabClients: 'Clients',
    clientsTitle: 'Workspaces',
    clientsLead:
      'Spin up a separate workspace for each client and track how each one uses your shared account allowance — all from one place.',
    sharedUsage: 'Shared allowance',
    workspacesUsed: 'Workspaces',
    newWorkspace: 'New workspace',
    workspaceName: 'Workspace name',
    create: 'Create',
    open: 'Open',
    current: 'Current',
    noSlotsLeft: "You've used every workspace your plan includes. Upgrade to add more.",
    repliesLabel: 'replies',
    convosLabel: 'conversations',
    sourcesLabel: 'sources',
    switchWorkspace: 'Switch workspace',
    roleOwner: 'Owner',
    roleAdmin: 'Admin',
    roleAgent: 'Agent',
    deleteWorkspaceTitle: 'Delete workspace',
    deleteWorkspaceLead:
      "This permanently deletes the workspace with all its sources, conversations and leads, and its assistant stops working on every site it's installed on. This can't be undone.",
    deleteKeepsUsage: 'Replies already used this month still count toward your shared allowance.',
    deleteConfirmLabel: 'To confirm, type the workspace name:',
    deletePermanently: 'Delete permanently',
    deleting: 'Deleting…',
    teamTitle: 'Team',
    teamLead:
      'Invite your client or colleagues into this workspace. Admins manage content and settings; agents follow and answer conversations.',
    seatsLabel: 'Seats',
    invitesNeedGrowth: 'Inviting team members is available from the Growth plan.',
    roleLabel: 'Role',
    sendInvite: 'Send invitation',
    sendingInvite: 'Sending…',
    inviteSentTo: 'Invitation sent to',
    inviteNotEmailed:
      "We couldn't email it. Copy the link and send it yourself — it only works for that address, and expires in seven days.",
    pendingInvites: 'Pending invitations',
    expires: 'Expires',
    expired: 'Expired',
    resend: 'Resend',
    revoke: 'Revoke',
    removeMember: 'Remove',
    leaveWorkspace: 'Leave',
    you: 'You',
    seatsFull:
      'Every seat on your plan is taken. Remove someone or revoke an invitation, or upgrade.',
    inviteTitle: "You've been invited",
    acceptInvite: 'Accept invitation',
    accepting: 'Accepting…',
    notNow: 'Not now',
    inviteUseEmail: 'Sign in or create an account with:',
    inviteExpired: 'This invitation has expired. Ask the person who sent it for a new one.',
    inviteInvalid: 'This invitation link is invalid or was revoked.',
    yourWorkspaces: 'Your workspaces',
    sharedWithYou: 'Shared with you',
    agentNote:
      "You're an agent here: you follow and answer conversations, while the owner and admins manage content and settings.",
    mConversations: 'Conversations',
    mAutoResolved: 'Auto-resolved',
    mEscalated: 'Escalated',
    mLeads: 'Leads captured',
    mAnswered: 'Answered from sources',
    mUnanswered: 'Unanswered',
    mFirstResponse: 'Median first response',
    mHelpful: 'Helpful / unhelpful',
    howResolved: 'What counted as resolved',
    howResolvedHelp:
      'A conversation is never counted as resolved just because the visitor closed the tab. These are the actual signals.',
    noResolutionsYet: 'No signals yet. They appear once visitors rate answers.',
    sigRatedHelpful: 'Visitor said it helped',
    sigUserConfirmed: 'Visitor confirmed',
    sigActionCompleted: 'Action completed',
    sigEndedAnswered: 'Ended after an answer',
    languagesTitle: 'Languages',
    gapsTitle: 'Unanswered questions',
    gapsHelp:
      'Questions your sources could not answer. Add an answer and it becomes part of the knowledge base straight away.',
    gapsEmpty: 'No unanswered questions.',
    gapAnswer: 'Add an answer',
    gapAnswerPlaceholder: 'Write the answer as you would want Anis to say it…',
    gapSaveToKb: 'Add to knowledge',
    gapNearMiss: 'Near miss',
    metricsHonesty:
      'Only figures we can actually measure are shown. Top topics is absent because we do not cluster them yet.',
    currentPlan: 'Your plan',
    repliesUsed: 'AI replies used',
    ofLimit: 'of',
    usageWarning: 'You are close to your plan limit.',
    usageExceeded:
      'You are past your plan limit. Extra replies are billed up to your spending cap.',
    spendingCap: 'Overage spending cap',
    spendingCapHelp:
      'When this is reached Anis stops replying and offers a person instead. Set it to zero to allow no extra cost at all.',
    saveCap: 'Save',
    capSaved: 'Saved',
    upgrade: 'Upgrade',
    managePlan: 'Manage subscription',
    billingNotConfigured: 'Billing is not enabled on this server.',
    ownerOnly: 'Only the account owner can manage the subscription.',
    statusPastDue: 'There is a problem with your payment. We will retry before changing your plan.',
    cancelsOn: 'Subscription ends on',
    overageSoFar: 'Overage so far',
    inboxEmpty: 'No conversations yet.',
    needsAttention: 'Needs attention',
    statusActive: 'Active',
    statusAutoResolved: 'Auto-resolved',
    statusEscalated: 'Escalated',
    statusHuman: 'With a person',
    statusClosed: 'Closed',
    takeOver: 'Take over',
    returnToAuto: 'Return to Anis',
    markResolved: 'Close',
    replyPlaceholder: 'Write your reply…',
    sendReply: 'Send',
    takeoverNotOnPlan: 'Replying live is available on Growth and above.',
    leadContact: 'Contact details',
    selectConversation: 'Choose a conversation to open it.',
    aiPaused: 'Anis has stopped replying in this conversation.',
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
    days: {
      zero: 'لا أيام',
      one: 'يوم واحد',
      two: 'يومان',
      few: 'أيام',
      many: 'يوماً',
      other: 'يوم',
    },
  },
  en: {
    passages: { one: 'passage', other: 'passages' },
    days: { one: 'day', other: 'days' },
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
 * "Sara invited you to join “Noor Store” as an admin."
 *
 * A function rather than fragments, because the two languages order the parts
 * differently and Arabic takes no article before the role. Gluing translated
 * pieces together in English word order is how a sentence ends up reading as
 * machine-assembled.
 */
export function inviteLine(
  lang: SupportedLanguage,
  inviter: string,
  workspace: string,
  role: 'admin' | 'agent',
): string {
  if (lang === 'ar') {
    const r = role === 'admin' ? 'مشرف' : 'وكيل';
    return inviter
      ? `دعاك ${inviter} للانضمام إلى «${workspace}» بصفة ${r}.`
      : `دُعيت للانضمام إلى «${workspace}» بصفة ${r}.`;
  }
  const r = role === 'admin' ? 'an admin' : 'an agent';
  return inviter
    ? `${inviter} invited you to join “${workspace}” as ${r}.`
    : `You've been invited to join “${workspace}” as ${r}.`;
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

/** Remembered choice, else Arabic — the dashboard's primary language. */
export function initialLanguage(): SupportedLanguage {
  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    if (stored === 'ar' || stored === 'en') return stored;
  } catch {
    /* private mode */
  }
  return 'ar';
}

export function persistLanguage(lang: SupportedLanguage): void {
  try {
    localStorage.setItem(STORAGE_KEY, lang);
  } catch {
    /* private mode; the choice just won't survive a reload */
  }
}
