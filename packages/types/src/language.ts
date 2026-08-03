/**
 * Language and direction detection.
 *
 * This is the thing the whole "Arabic-first" claim rests on at the UI layer: a
 * visitor types Arabic, the bubble must lay out right-to-left; they type
 * English, it must not. And because Gulf and Levantine customers routinely
 * code-switch mid-sentence, a whole-conversation language setting is not
 * enough — direction is decided per message.
 *
 * The backend mirrors `detectLanguage` in Go for the same input, so a message
 * is not labelled `ar` by the widget and `en` by the retriever. If you change
 * the thresholds here, change `pocketbase/internal/lang/detect.go` too — there
 * is a shared fixture table both sides run against.
 *
 * A note on style: Arabic characters in this file are written as `\u` escapes,
 * not literally. Literal Arabic renders in visual order in most editors and in
 * every diff viewer, which makes a character class impossible to review and
 * easy to corrupt with a stray edit. The escapes are annotated.
 */

import type { SupportedLanguage } from './domain.js';

/**
 * Arabic LETTERS — script and category together.
 *
 * `\p{Script=Arabic}` covers the base block plus the Supplement, Extended-A/B
 * and presentation-form blocks without enumerating any of them.
 *
 * The `(?=\p{L})` lookahead restricts the match to letters, which excludes
 * three things that would otherwise corrupt the ratio: combining marks
 * (tashkeel would inflate the Arabic count on diacritised text), Arabic-Indic
 * digits (a price is not evidence of a language), and Arabic punctuation —
 * "؟" and "،" are routinely used in otherwise-English text by
 * bilingual writers.
 */
const ARABIC_LETTERS_GLOBAL = /(?=\p{L})\p{Script=Arabic}/gu;

/**
 * Latin letters. Script-based rather than `[A-Za-z]` so accented French and
 * transliterated text count — a Lyon shop's page is Latin script whether or
 * not it uses "é".
 */
const LATIN_LETTERS_GLOBAL = /(?=\p{L})\p{Script=Latin}/gu;

/**
 * Share of script-bearing characters that must be Arabic for a message to be
 * treated as Arabic.
 *
 * Set below 0.5 on purpose. Arabic sentences absorb Latin tokens constantly —
 * brand names, SKUs, URLs, "iPhone 15 Pro" — while an English sentence almost
 * never contains Arabic letters. So the asymmetry is real, and the cost of
 * error is asymmetric too: rendering Arabic in a left-to-right bubble is
 * visibly broken, whereas an English fragment inside an RTL bubble is handled
 * correctly by the browser's bidi algorithm on its own.
 */
const ARABIC_THRESHOLD = 0.2;

export interface LanguageDetection {
  language: SupportedLanguage;
  dir: 'rtl' | 'ltr';
  /**
   * Fraction of letters that were Arabic, 0–1. Exposed so the backend can log
   * it and we can tune the threshold against real traffic rather than guesses.
   */
  arabicRatio: number;
  /**
   * True when the text contains a meaningful amount of BOTH scripts. The
   * assistant is told about this so it replies in the same mixed register the
   * customer used instead of "correcting" them into one language.
   */
  mixed: boolean;
}

function countLetters(text: string): { arabic: number; latin: number } {
  return {
    arabic: text.match(ARABIC_LETTERS_GLOBAL)?.length ?? 0,
    latin: text.match(LATIN_LETTERS_GLOBAL)?.length ?? 0,
  };
}

/**
 * Detect the language of a single message.
 *
 * `fallback` is returned for text with no letters at all — an emoji, a bare
 * order number, "؟" — which is common for the first message in a conversation.
 * Pass the workspace's configured default, or the previous message's language,
 * so a lone emoji does not flip the thread's direction.
 */
export function detectLanguage(
  text: string,
  fallback: SupportedLanguage = 'en',
): LanguageDetection {
  const { arabic, latin } = countLetters(text);
  const total = arabic + latin;

  if (total === 0) {
    return {
      language: fallback,
      dir: fallback === 'ar' ? 'rtl' : 'ltr',
      arabicRatio: 0,
      mixed: false,
    };
  }

  const arabicRatio = arabic / total;
  const language: SupportedLanguage = arabicRatio >= ARABIC_THRESHOLD ? 'ar' : 'en';

  return {
    language,
    dir: language === 'ar' ? 'rtl' : 'ltr',
    arabicRatio,
    // Both scripts present in more than a token amount.
    mixed: arabic > 0 && latin > 0 && arabicRatio > 0.1 && arabicRatio < 0.9,
  };
}

/**
 * Decide a conversation's language from its messages so far.
 *
 * Weighted toward recent messages: a visitor who opens in English and switches
 * to Arabic should get Arabic, not a majority vote over the whole history.
 * Only visitor messages count — the assistant's own replies would otherwise
 * lock in whatever it guessed first.
 */
export function detectConversationLanguage(
  visitorMessages: readonly string[],
  fallback: SupportedLanguage = 'en',
): SupportedLanguage {
  if (visitorMessages.length === 0) return fallback;

  const recent = visitorMessages.slice(-5);
  let weighted = 0;
  let weight = 0;

  recent.forEach((text, i) => {
    const { arabic, latin } = countLetters(text);
    const letters = arabic + latin;
    if (letters === 0) return;
    // Later messages count for more; longer messages count for more, but a
    // very long message is capped so one pasted paragraph cannot outvote
    // everything the visitor has said since.
    const w = (i + 1) * Math.min(letters, 200);
    weighted += (arabic / letters) * w;
    weight += w;
  });

  if (weight === 0) return fallback;
  return weighted / weight >= ARABIC_THRESHOLD ? 'ar' : 'en';
}

/* -------------------------------------------------------------------------
 * Normalisation for retrieval
 * ---------------------------------------------------------------------- */

/**
 * Tashkeel and the other Arabic combining marks.
 *
 * Note `Script_Extensions`, not `Script`. Arabic diacritics are
 * `Script=Inherited` — U+064E FATHA is NOT `Script=Arabic` — so the obvious
 * `\p{Script=Arabic}` spelling matches zero marks and silently normalises
 * nothing. Verified: see the codepoint table in `language.test.ts`.
 *
 * Bare `\p{Mn}` would match these, but it also matches the combining acute in
 * a decomposed "café", turning it into "cafe" and breaking retrieval for
 * French-language sources — which we specifically have, being hosted in
 * France. Keep the script constraint.
 */
const ARABIC_MARKS = /(?=\p{Mn})\p{Script_Extensions=Arabic}/gu;

/** U+0640 ARABIC TATWEEL — decorative letter-stretching, never semantic. */
const TATWEEL = /ـ/gu;

/** Alef with madda / hamza above / hamza below / wasla → bare alef U+0627. */
const ALEF_VARIANTS = /[آأإٱ]/gu;

/** U+0649 ALEF MAKSURA → U+064A YEH. */
const ALEF_MAKSURA = /ى/gu;

/** U+0629 TEH MARBUTA → U+0647 HEH. */
const TEH_MARBUTA = /ة/gu;

/** Hamza on waw / on yeh → bare hamza U+0621. */
const HAMZA_SEATS = /[ؤئ]/gu;

/** Arabic-Indic U+0660–0669 and Extended Arabic-Indic U+06F0–06F9. */
const ARABIC_INDIC_DIGITS = /[٠-٩۰-۹]/gu;

function asciiDigit(ch: string): string {
  const cp = ch.codePointAt(0)!;
  const base = cp >= 0x06f0 ? 0x06f0 : 0x0660;
  return String(cp - base);
}

/**
 * Fold the Arabic spelling variants that customers and source documents
 * disagree about, so a question and the page that answers it match.
 *
 * Concretely: a shop writes the word for "returns" with a hamza under the alef
 * and the customer types it with a bare alef. Without folding, those are
 * different strings to a keyword index and measurably different vectors to an
 * embedding model. The variants folded here — alef forms, final yeh / alef
 * maksura, teh marbuta / heh, the hamza seats — are the ones where the
 * distinction is orthographic rather than semantic in ordinary business prose.
 *
 * Apply this to BOTH the indexed text and the query, and to the keyword index
 * unconditionally. For embeddings it is worth measuring first: a model trained
 * on diacritised text may lose signal when you strip tashkeel. Always keep the
 * original text for display — never show a customer normalised text.
 */
export function normalizeArabic(text: string): string {
  return text
    .replace(ARABIC_MARKS, '')
    .replace(TATWEEL, '')
    .replace(ALEF_VARIANTS, 'ا')
    .replace(ALEF_MAKSURA, 'ي')
    .replace(TEH_MARBUTA, 'ه')
    .replace(HAMZA_SEATS, 'ء')
    .replace(ARABIC_INDIC_DIGITS, asciiDigit)
    .replace(/\s+/gu, ' ')
    .trim();
}

/**
 * The value for a `dir` attribute on an element holding user-language content.
 *
 * Prefer `"auto"` and let the browser decide from the first strong character —
 * that is what the brief asks for, and it correctly handles a message we would
 * misjudge. Use `dirFor()` from `./domain.js` only where the direction must be
 * known in advance, such as laying out a container around the message.
 */
export const USER_CONTENT_DIR = 'auto' as const;
