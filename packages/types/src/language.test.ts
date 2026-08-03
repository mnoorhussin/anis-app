/**
 * Fixtures for language detection.
 *
 * `pocketbase/internal/lang/detect_test.go` runs the same cases against the Go
 * implementation. Add a case to one, add it to the other — a message the
 * widget renders RTL and the retriever treats as English is a bug that only
 * shows up in production, in a language most of the team cannot read.
 */

import { describe, expect, it } from 'vitest';

import { detectConversationLanguage, detectLanguage, normalizeArabic } from './language.js';
import { PLANS, featureIsAvailable, overageCostUsd, repliesRemainingUnderCap } from './plans.js';

describe('detectLanguage', () => {
  const arabic = [
    'مرحبا، كيف يمكنني تتبع طلبي؟',
    'هل عندكم شحن مجاني؟',
    'أريد إرجاع المنتج',
    'كم سعر التوصيل إلى الرياض؟',
  ];

  it.each(arabic)('reads %s as Arabic', (text) => {
    const d = detectLanguage(text);
    expect(d.language).toBe('ar');
    expect(d.dir).toBe('rtl');
  });

  const english = [
    'Hi, how do I track my order?',
    'Do you ship to France?',
    'I want a refund for order #10482',
    'whats ur return policy',
  ];

  it.each(english)('reads %s as English', (text) => {
    const d = detectLanguage(text);
    expect(d.language).toBe('en');
    expect(d.dir).toBe('ltr');
  });

  it('treats code-switched text as Arabic and flags it as mixed', () => {
    // The case the product exists for: Gulf/Levantine customers mixing scripts
    // freely. Latin brand names must not drag the message to English.
    const d = detectLanguage('عندكم iPhone 15 Pro بالمخزون؟');
    expect(d.language).toBe('ar');
    expect(d.mixed).toBe(true);
  });

  it('keeps an English sentence English when it quotes an Arabic word', () => {
    const d = detectLanguage('Is the product name written as أنيس on the invoice?');
    expect(d.language).toBe('en');
  });

  it('falls back rather than guessing on text with no letters', () => {
    // Very common as an opening message. Flipping direction on "👍" would make
    // the whole thread jump.
    for (const text of ['👍', '2024', '؟؟؟', '   ', '#10482']) {
      expect(detectLanguage(text, 'ar').language).toBe('ar');
      expect(detectLanguage(text, 'en').language).toBe('en');
    }
  });

  it('reports the ratio it decided on', () => {
    expect(detectLanguage('hello').arabicRatio).toBe(0);
    expect(detectLanguage('مرحبا').arabicRatio).toBe(1);
  });
});

describe('detectConversationLanguage', () => {
  it('follows a mid-conversation switch to Arabic', () => {
    expect(
      detectConversationLanguage([
        'Hello',
        'Do you have this in stock?',
        'ممكن تجاوبني بالعربي من فضلك',
        'شكرا، وكم سعر التوصيل؟',
      ]),
    ).toBe('ar');
  });

  it('does not let one short Arabic aside flip a long English thread', () => {
    expect(
      detectConversationLanguage([
        'I ordered a jacket last Tuesday and it has not arrived yet, can you check the status',
        'The order number is 10482 and it was shipped to Lyon, France',
        'شكرا',
      ]),
    ).toBe('en');
  });

  it('uses the fallback for an empty conversation', () => {
    expect(detectConversationLanguage([], 'ar')).toBe('ar');
  });
});

describe('normalizeArabic', () => {
  it('folds the alef variants a customer and a policy page disagree on', () => {
    // "الإرجاع" on the returns page vs "الارجاع" typed by the customer.
    expect(normalizeArabic('الإرجاع')).toBe(normalizeArabic('الارجاع'));
    expect(normalizeArabic('أحمد')).toBe(normalizeArabic('احمد'));
  });

  it('strips tashkeel and tatweel', () => {
    expect(normalizeArabic('مَرْحَبًا')).toBe('مرحبا');
    expect(normalizeArabic('مـــرحبا')).toBe('مرحبا');
  });

  it('does not damage decomposed Latin accents', () => {
    // Regression guard. The obvious way to strip tashkeel is `\p{Mn}`, which
    // also eats the combining acute in a decomposed "café" and quietly breaks
    // retrieval for French sources. Anis is hosted in France; we have those.
    const decomposed = 'café';
    expect(normalizeArabic(decomposed)).toBe(decomposed);
  });

  it('assumes tashkeel is Script=Inherited, not Script=Arabic', () => {
    // Documents the Unicode fact that makes the regex in language.ts look
    // wrong to anyone who has not checked. If a future edit "simplifies"
    // Script_Extensions back to Script, normalisation silently stops working
    // and only this test notices.
    const fatha = 'َ';
    expect(/\p{Script=Arabic}/u.test(fatha)).toBe(false);
    expect(/\p{Script=Inherited}/u.test(fatha)).toBe(true);
    expect(/\p{Script_Extensions=Arabic}/u.test(fatha)).toBe(true);
  });

  it('folds ta marbuta and alef maqsura', () => {
    expect(normalizeArabic('سياسة')).toBe(normalizeArabic('سياسه'));
    expect(normalizeArabic('على')).toBe(normalizeArabic('علي'));
  });

  it('converts Arabic-Indic digits so prices match', () => {
    expect(normalizeArabic('٥٠ ريال')).toBe('50 ريال');
    expect(normalizeArabic('١٢٣٤٥٦٧٨٩٠')).toBe('1234567890');
  });

  it('leaves Latin text alone', () => {
    expect(normalizeArabic('iPhone 15 Pro')).toBe('iPhone 15 Pro');
  });
});

describe('plan enforcement', () => {
  it('has no unlimited plan', () => {
    for (const plan of Object.values(PLANS)) {
      expect(plan.limits.aiRepliesPerMonth).toBeGreaterThan(0);
      expect(Number.isFinite(plan.limits.aiRepliesPerMonth)).toBe(true);
    }
  });

  it('refuses an entitled feature that is not built yet', () => {
    // Pro is sold "advanced actions", but until they genuinely work the
    // product must not switch them on. This is the truthful-claims gate.
    expect(PLANS.pro.features).toContain('advancedActions');
    expect(featureIsAvailable('pro', 'advancedActions')).toBe(false);
    expect(featureIsAvailable('pro', 'apiAccess')).toBe(true);
    expect(featureIsAvailable('starter', 'apiAccess')).toBe(false);
  });

  it('stops generating once the hard cap is reached', () => {
    expect(repliesRemainingUnderCap(0, 50)).toBe(2500);
    expect(repliesRemainingUnderCap(50, 50)).toBe(0);
    expect(repliesRemainingUnderCap(60, 50)).toBe(0);
    // A cap of 0 means "never bill me overage" — it must not mean "unlimited".
    expect(repliesRemainingUnderCap(0, 0)).toBe(0);
  });

  it('charges nothing below the allowance', () => {
    expect(overageCostUsd(0)).toBe(0);
    expect(overageCostUsd(-5)).toBe(0);
    expect(overageCostUsd(100)).toBe(2);
  });

  it('prices annual billing as ten months', () => {
    // The marketing site says "2 months free". If this ratio changes, that
    // copy becomes untrue.
    expect(PLANS.growth.annualMonthlyUsd * 12).toBeCloseTo(79 * 10, 1);
  });
});
