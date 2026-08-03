/**
 * Domain vocabulary — the enums and shapes that both the Go backend and the
 * TypeScript frontends must agree on.
 *
 * These are hand-written and authoritative. Record types generated from the
 * live PocketBase schema live in `./pocketbase.generated.ts`; where the two
 * describe the same field, THIS file defines what the values mean.
 *
 * Tenancy: every row below except `accounts`, `users` and `memberships` carries
 * a `workspace` relation. A workspace is one assistant and one tenant. Nothing
 * is ever queried without a workspace scope — see the collection API rules in
 * `pocketbase/migrations/` for the enforcement, which is server-side and does
 * not depend on the client passing the right filter.
 */

/* -------------------------------------------------------------------------
 * People and tenancy
 * ---------------------------------------------------------------------- */

/**
 * Roles are per-membership, not per-user: one person can be an owner of their
 * own account and an agent in an agency's workspace.
 *
 *  - `owner`  billing, plan changes, deleting the account. Exactly one.
 *  - `admin`  everything except billing and deletion.
 *  - `agent`  the inbox: read conversations, take over, resolve. No settings.
 */
export const MEMBERSHIP_ROLES = ['owner', 'admin', 'agent'] as const;
export type MembershipRole = (typeof MEMBERSHIP_ROLES)[number];

/**
 * `agency` accounts may hold many workspaces and invite client users into a
 * single workspace each. `direct` accounts are one business, normally one
 * workspace.
 */
export const ACCOUNT_KINDS = ['direct', 'agency'] as const;
export type AccountKind = (typeof ACCOUNT_KINDS)[number];

/* -------------------------------------------------------------------------
 * Knowledge sources
 * ---------------------------------------------------------------------- */

export const SOURCE_TYPES = ['website', 'pdf', 'text', 'faq'] as const;
export type SourceType = (typeof SOURCE_TYPES)[number];

/**
 * Source lifecycle. Shown verbatim in the dashboard, so a source is never in a
 * state the customer cannot see or explain.
 *
 *  - `queued`      accepted, waiting for a worker
 *  - `fetching`    crawling / downloading
 *  - `processing`  extracting text, chunking, embedding
 *  - `ready`       searchable
 *  - `failed`      see `error`; the customer can retry
 *  - `stale`       previously ready, content changed upstream, needs refresh
 */
export const SOURCE_STATUSES = [
  'queued',
  'fetching',
  'processing',
  'ready',
  'failed',
  'stale',
] as const;
export type SourceStatus = (typeof SOURCE_STATUSES)[number];

/* -------------------------------------------------------------------------
 * Conversations
 * ---------------------------------------------------------------------- */

/**
 * Channels. Only `website` is shipped. WhatsApp and Messenger are listed here
 * because the data model must not need migrating when they land — but they are
 * marked "Soon" on the marketing site and the product must refuse to enable
 * them until they genuinely work.
 */
export const CHANNELS = ['website', 'whatsapp', 'messenger'] as const;
export type Channel = (typeof CHANNELS)[number];

/** Channels a customer can actually connect today. */
export const SHIPPED_CHANNELS: readonly Channel[] = ['website'] as const;

/**
 * Conversation state.
 *
 *  - `active`         open, the AI is answering
 *  - `auto_resolved`  closed by the AI, with a defensible signal — see
 *                     `RESOLUTION_SIGNALS`. Never inferred from silence alone.
 *  - `escalated`      a human has been asked for; nobody has picked it up
 *  - `human`          a human is handling it; the AI does not generate replies
 *  - `closed`         ended without resolution and without escalation
 */
export const CONVERSATION_STATUSES = [
  'active',
  'auto_resolved',
  'escalated',
  'human',
  'closed',
] as const;
export type ConversationStatus = (typeof CONVERSATION_STATUSES)[number];

/**
 * The only signals that may mark a conversation `auto_resolved`.
 *
 * This is the honesty constraint from the brief, encoded. Analytics must not
 * count an abandoned conversation as a resolution: if a visitor closes the tab
 * after a relevant answer and none of these fired, the conversation is
 * `closed`, not `auto_resolved`.
 *
 *  - `rated_helpful`      the visitor pressed 👍
 *  - `action_completed`   a configured action (booking, order lookup) finished
 *  - `user_confirmed`     the visitor said the question was answered
 *  - `ended_answered`     the conversation ended after a grounded, high-
 *                         confidence answer AND the visitor never asked for a
 *                         human. The weakest signal — report it separately in
 *                         analytics so the headline number stays defensible.
 */
export const RESOLUTION_SIGNALS = [
  'rated_helpful',
  'action_completed',
  'user_confirmed',
  'ended_answered',
] as const;
export type ResolutionSignal = (typeof RESOLUTION_SIGNALS)[number];

export const MESSAGE_ROLES = ['user', 'assistant', 'human'] as const;
export type MessageRole = (typeof MESSAGE_ROLES)[number];

/**
 * Why an assistant message came out the way it did. Stored per message so the
 * knowledge-gap report and the analytics numbers can both be reconstructed
 * from evidence rather than recomputed guesses.
 *
 *  - `answered`   grounded in retrieved sources, above the confidence floor
 *  - `refused`    retrieval found nothing usable; the assistant said so and
 *                 offered a human. This is a SUCCESS, not an error.
 *  - `clarify`    the question was ambiguous; the assistant asked back
 *  - `escalated`  handed to a human
 *  - `blocked`    refused for policy, rate limit, or a reached spending cap
 */
export const ANSWER_OUTCOMES = ['answered', 'refused', 'clarify', 'escalated', 'blocked'] as const;
export type AnswerOutcome = (typeof ANSWER_OUTCOMES)[number];

/**
 * Below this retrieval score the assistant refuses instead of answering.
 *
 * Tune against a labelled Arabic + English question set before launch; a floor
 * that is too low produces confident wrong answers, which is the one failure
 * this product cannot have. Erring high costs a refusal, which is recoverable.
 */
export const DEFAULT_CONFIDENCE_FLOOR = 0.35;

/**
 * The refusal the product must actually perform, from the brief. Kept here so
 * it cannot drift from what the marketing site promises.
 */
export const REFUSAL_TEMPLATE = {
  ar: 'لم أجد هذه المعلومة في مصادر الشركة. هل ترغب في تحويل سؤالك إلى أحد الموظفين؟',
  en: "I couldn't find that in the company's sources. Would you like me to pass your question to a member of the team?",
} as const;

/* -------------------------------------------------------------------------
 * Escalation
 * ---------------------------------------------------------------------- */

export const ESCALATION_STATUSES = ['pending', 'notified', 'assigned', 'resolved'] as const;
export type EscalationStatus = (typeof ESCALATION_STATUSES)[number];

/* -------------------------------------------------------------------------
 * Language
 * ---------------------------------------------------------------------- */

/**
 * Languages the product commits to. The marketing site deliberately does NOT
 * claim "40+ languages" — it claims Arabic and English, plus more. Add a code
 * here only once replies in it have been reviewed by someone who reads it.
 */
export const SUPPORTED_LANGUAGES = ['ar', 'en'] as const;
export type SupportedLanguage = (typeof SUPPORTED_LANGUAGES)[number];

/** `auto` detects per message, which is what AR/EN code-switching needs. */
export type LanguageSetting = SupportedLanguage | 'auto';

export function dirFor(lang: SupportedLanguage): 'rtl' | 'ltr' {
  return lang === 'ar' ? 'rtl' : 'ltr';
}

/* -------------------------------------------------------------------------
 * Widget configuration
 * ---------------------------------------------------------------------- */

export interface WidgetConfig {
  /** Assistant display name, e.g. "أنيس" or the business's own name. */
  name: string;
  /** Accent colour. Defaults to brand iris. */
  accentColor: string;
  /** URL of the business's logo, or null to show the Anis mark. */
  logoUrl: string | null;
  greeting: { ar: string; en: string };
  /** Prompts shown before the first message. Keep to 3–4. */
  suggestedQuestions: { ar: string[]; en: string[] };
  /**
   * Hostnames permitted to embed this widget. Enforced server-side against the
   * request Origin — an empty list means the widget refuses to load anywhere,
   * which is the safe default for a workspace that has not been configured.
   */
  allowedDomains: string[];
  /** "Powered by Anis". Only removable on plans with `removeBranding`. */
  badgeOn: boolean;
  theme: 'light' | 'dark' | 'auto';
  position: 'right' | 'left';
  language: LanguageSetting;
}

export const DEFAULT_WIDGET_CONFIG: WidgetConfig = {
  name: 'Anis',
  accentColor: '#5a5af0',
  logoUrl: null,
  greeting: {
    ar: 'مرحباً! كيف أقدر أساعدك؟',
    en: 'Hi! How can I help?',
  },
  suggestedQuestions: { ar: [], en: [] },
  allowedDomains: [],
  badgeOn: true,
  theme: 'auto',
  position: 'right',
  language: 'auto',
};
