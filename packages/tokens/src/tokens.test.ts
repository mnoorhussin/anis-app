/**
 * Guards against the two halves of the token system drifting apart.
 *
 * `theme.css` is what the apps actually render with; `tokens.ts` is what JS
 * reads when it needs a colour it cannot get from a custom property (charts,
 * the widget's shadow root, OG images). Nothing enforces that they agree
 * except this file. A silent drift means the widget renders in one brand and
 * the dashboard in another, which is the kind of bug nobody reports and
 * everybody notices.
 */

import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { describe, expect, it } from 'vitest';

import { brand, dark, gradients, gray, light, logoMarkSvg, status } from './tokens.js';

const themeCss = readFileSync(fileURLToPath(new URL('./theme.css', import.meta.url)), 'utf8');

/**
 * Reads a custom property out of theme.css.
 *
 * `scope` narrows the search to one rule block, because the same property name
 * is declared several times with different values — `--muted` exists in
 * `:root`, in `:root[data-theme='dark']`, and again in the
 * prefers-color-scheme block. Matching the first occurrence would silently
 * compare the light value against the dark expectation.
 */
function cssVar(prop: string, scope?: string): string | undefined {
  const source = scope ? (themeCss.split(scope)[1] ?? '') : themeCss;
  const block = scope ? (source.split('}')[0] ?? '') : source;
  const match = new RegExp(`${prop}\\s*:\\s*([^;]+);`).exec(block);
  return match?.[1]?.trim();
}

describe('brand colours', () => {
  it.each(Object.entries(brand))('%s matches theme.css', (name, value) => {
    // brand.irisBright → --color-iris-bright
    const prop = `--color-${name.replace(/[A-Z]/g, (c) => `-${c.toLowerCase()}`)}`;
    expect(cssVar(prop)).toBe(value);
  });

  it('the signature gradient is identical in both files', () => {
    expect(cssVar('--gradient-brand', ':root {')).toBe(gradients.brand);
  });
});

describe('neutral ramp', () => {
  it.each(Object.entries(gray))('gray-%s matches theme.css', (step, value) => {
    expect(cssVar(`--color-gray-${step}`)).toBe(value);
  });

  it('runs light to dark without a repeated or inverted step', () => {
    const steps = Object.values(gray).map((hex) => {
      const n = parseInt(hex.slice(1), 16);
      // Rough perceptual weighting is enough to catch an ordering mistake.
      return 0.299 * ((n >> 16) & 255) + 0.587 * ((n >> 8) & 255) + 0.114 * (n & 255);
    });
    for (let i = 1; i < steps.length; i++) {
      expect(steps[i]!).toBeLessThan(steps[i - 1]!);
    }
  });
});

describe('status colours', () => {
  it.each(Object.entries(status))('%s matches theme.css', (name, value) => {
    const prop = `--color-${name.replace(/[A-Z]/g, (c) => `-${c.toLowerCase()}`)}`;
    // The dark theme overrides the `soft` fills, so only check the light
    // declarations inside the top-level @theme block.
    expect(cssVar(prop, '@theme {')).toBe(value);
  });
});

describe('semantic themes', () => {
  it('light surfaces match :root', () => {
    expect(cssVar('--background', ':root {')).toBe(light.background);
    expect(cssVar('--foreground', ':root {')).toBe(light.foreground);
    expect(cssVar('--muted', ':root {')).toBe(light.muted);
    expect(cssVar('--ring', ':root {')).toBe(light.ring);
  });

  it('dark surfaces match :root[data-theme=dark]', () => {
    const scope = ":root[data-theme='dark'] {";
    expect(cssVar('--background', scope)).toBe(dark.background);
    expect(cssVar('--foreground', scope)).toBe(dark.foreground);
    expect(cssVar('--muted', scope)).toBe(dark.muted);
    expect(cssVar('--ring', scope)).toBe(dark.ring);
  });

  it('the system-preference block matches the explicit dark block', () => {
    // These are duplicated in theme.css by necessity — a user who has never
    // touched the theme toggle gets the media-query block, and a user who
    // chose dark gets the attribute block. If they diverge, toggling the
    // theme to the value it already had visibly changes the page.
    const explicit = ":root[data-theme='dark'] {";
    const system = ":root:not([data-theme='light']) {";
    for (const prop of ['--background', '--surface', '--foreground', '--muted', '--ring']) {
      expect(cssVar(prop, system)).toBe(cssVar(prop, explicit));
    }
  });
});

describe('logo mark', () => {
  it('namespaces its gradient id so two marks on a page do not collide', () => {
    const a = logoMarkSvg('one');
    const b = logoMarkSvg('two');
    expect(a).toContain('id="one-grad"');
    expect(a).toContain('url(#one-grad)');
    expect(b).not.toContain('one-grad');
  });

  it('uses the brand gradient stops', () => {
    const svg = logoMarkSvg();
    expect(svg).toContain(brand.iris);
    expect(svg).toContain(brand.aqua);
  });
});
