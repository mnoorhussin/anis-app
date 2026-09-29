import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import * as tokens from './tokens.js';
import {
  brand,
  dark,
  darkStatusSoft,
  statusForeground,
  displayType,
  easing,
  fonts,
  gray,
  ijamDots,
  light,
  logoMarkSvg,
  radii,
  shadows,
  status,
  themeCssVars,
} from './tokens.js';

const themeCss = readFileSync(new URL('./theme.css', import.meta.url), 'utf8');
const kebab = (s: string) =>
  s.replace(/[A-Z]/g, (c) => `-${c.toLowerCase()}`).replace(/([a-z])(\d)/g, '$1-$2');
const normalize = (s: string) => s.replace(/\s+/g, ' ').trim();
function declarations(scope: string): Record<string, string> {
  const block = themeCss.split(scope)[1]?.split('}')[0];
  if (!block) throw new Error(`Missing scope: ${scope}`);
  return Object.fromEntries(
    [...block.matchAll(/(--[\w-]+)\s*:\s*([^;]+);/g)].map((m) => [m[1]!, normalize(m[2]!)]),
  );
}
const css = declarations('@theme {');

describe('CSS/TS parity (all shared tokens, both directions)', () => {
  const expected = Object.fromEntries([
    ...Object.entries({ ...brand, ...status }).map(([k, v]) => [`--color-${kebab(k)}`, v]),
    ...Object.entries(gray).map(([k, v]) => [`--color-gray-${k}`, v]),
    ...Object.entries(radii).map(([k, v]) => [`--radius-${k}`, v]),
    ...Object.entries(easing).map(([k, v]) => [`--ease-${kebab(k)}`, v]),
    ...Object.entries(displayType).map(([k, v]) => [`--text-${k}`, v]),
    ...Object.entries(fonts)
      .filter(([k]) => !['displayArabic', 'system'].includes(k))
      .map(([k, v]) => [`--font-${k}`, v]),
  ]);
  it('matches every shared theme declaration, including additions', () => {
    const shared = Object.fromEntries(
      Object.entries(css).filter(([k]) => !k.startsWith('--animate-')),
    );
    expect(shared).toEqual(expected);
  });
  it.each([
    ['light', ':root {', light],
    ['dark', ":root[data-theme='dark'] {", dark],
    ['dark', ":root:not([data-theme='light']) {", dark],
  ] as const)('%s semantics and shadows match %s', (name, scope, theme) => {
    const expected = Object.fromEntries(
      Object.entries(theme).map(([k, v]) => [`--${kebab(k)}`, v]),
    );
    for (const [key, value] of Object.entries(statusForeground[name]))
      expected[`--status-${key}`] = value;
    if (name === 'dark')
      for (const [key, value] of Object.entries(darkStatusSoft))
        expected[`--color-${kebab(key)}`] = value;
    expected['--shadow-soft'] = shadows[name].soft;
    expected['--shadow-lifted'] = shadows[name].lifted;
    expect(declarations(scope)).toEqual(expected);
    for (const [key, value] of Object.entries(expected))
      expect(themeCssVars(name)).toContain(`${key}: ${value};`);
  });
  it('promotes the Arabic faces on RTL surfaces', () => {
    const ar = declarations("[dir='rtl'] {");
    expect(ar['--font-sans']).toBe(fonts.arabic);
    expect(ar['--font-display']).toBe(fonts.displayArabic);
  });
});

it('keeps the neutral ramp strictly light to dark', () => {
  const values = Object.values(gray).map((hex) => parseInt(hex.slice(1), 16));
  for (let i = 1; i < values.length; i++) expect(values[i]).toBeLessThan(values[i - 1]!);
});
it('has no legacy palette, gradients, glass or glow', () => {
  expect(tokens).not.toHaveProperty('gradients');
  expect(themeCss).not.toMatch(
    /iris|aqua|linear-gradient|radial-gradient|backdrop-filter|glow|Satoshi/i,
  );
});
it('reuses the source mark geometry without SVG ID collisions', () => {
  expect(ijamDots).toHaveLength(3);
  const svg = logoMarkSvg();
  expect(svg).toContain(brand.ink);
  expect(svg).toContain(brand.saffron);
  expect(svg).not.toMatch(/id=|url\(#/);
  for (const d of ijamDots) expect(svg).toContain(`cx="${d.cx}" cy="${d.cy}" r="${d.r}"`);
});

function luminance(hex: string): number {
  const values = [1, 3, 5]
    .map((i) => parseInt(hex.slice(i, i + 2), 16) / 255)
    .map((s) => (s <= 0.04045 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4));
  return values[0]! * 0.2126 + values[1]! * 0.7152 + values[2]! * 0.0722;
}
it('status labels pass AA on surfaces and tinted badges in both themes', () => {
  for (const mode of ['light', 'dark'] as const) {
    const surfaces = mode === 'light' ? light : dark;
    for (const key of ['success', 'warning', 'danger', 'info'] as const) {
      const foreground = statusForeground[mode][key];
      const tint = (mode === 'light' ? status : darkStatusSoft)[`${key}Soft`];
      for (const bg of [surfaces.background, surfaces.surface, surfaces.surface2, tint]) {
        const a = luminance(foreground),
          b = luminance(bg);
        expect((Math.max(a, b) + 0.05) / (Math.min(a, b) + 0.05)).toBeGreaterThanOrEqual(4.5);
      }
    }
  }
});
