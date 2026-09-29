import { describe, expect, it } from 'vitest';
import { brand } from '@anis/tokens';
import { DEFAULT_WIDGET_CONFIG } from '@anis/types';
import { contrastOn, contrastRatio, hostVars, normalizeAccent } from './theme.js';

describe('Dar widget', () => {
  it('defaults to the light brand card, retaining automatic language detection', () => {
    expect(DEFAULT_WIDGET_CONFIG.accentColor).toBe(brand.oasis);
    expect(DEFAULT_WIDGET_CONFIG.theme).toBe('light');
    expect(DEFAULT_WIDGET_CONFIG.language).toBe('auto');
    expect(hostVars('light', brand.oasis)).toContain(`--surface:${brand.card}`);
    expect(hostVars('light', brand.oasis)).toContain(`--foreground:${brand.ink}`);
    expect(hostVars('light', brand.oasis)).toContain(`--saffron:${brand.saffron}`);
  });
  it('prefers cream on oasis and ink on pale accents', () => {
    expect(contrastOn(brand.oasis)).toBe(brand.cream);
    expect(contrastOn('#ffffaa')).toBe(brand.ink);
  });
  it('keeps arbitrary customer header colors AA, including the cream/ink contrast gap', () => {
    for (let r = 0; r <= 255; r += 17)
      for (let g = 0; g <= 255; g += 17)
        for (let b = 0; b <= 255; b += 17) {
          const hex = '#' + [r, g, b].map((c) => c.toString(16).padStart(2, '0')).join('');
          expect(contrastRatio(hex, contrastOn(hex))).toBeGreaterThanOrEqual(4.5);
        }
  });
  it('normalizes shorthand and rejects CSS injection', () => {
    expect(normalizeAccent('#abc')).toBe('#aabbcc');
    expect(normalizeAccent('red;}body{display:none')).toBe(brand.oasis);
    expect(hostVars('dark', '#AABBCC')).toContain('--accent:#aabbcc');
  });
});
