/**
 * @anis/tokens — the same design tokens as `theme.css`, as TypeScript values.
 *
 * Use these where CSS custom properties cannot reach: canvas/chart colours,
 * inline styles computed in JS, the widget's shadow-root theme, generated
 * email HTML, and OG images.
 *
 * `theme.css` is the source of truth for CSS. This file must be kept in sync
 * with it by hand — `packages/tokens/src/tokens.test.ts` asserts they agree,
 * so a drift fails the build rather than shipping two different brands.
 */

/* -------------------------------------------------------------------------
 * Brand
 * ---------------------------------------------------------------------- */

export const brand = {
  iris: '#5a5af0',
  irisBright: '#6d6dff',
  aqua: '#37e0c8',
  aquaBright: '#4df0d8',
  ink: '#0a0a0f',
  offwhite: '#fbfbfd',
} as const;

/**
 * The signature gradient. Use it sparingly — primary CTAs and key accents
 * only. It loses its signal the moment it becomes a background.
 */
export const gradients = {
  brand: 'linear-gradient(120deg, #5a5af0 0%, #6d6dff 40%, #37e0c8 100%)',
  brandRev: 'linear-gradient(120deg, #37e0c8 0%, #5a5af0 100%)',
} as const;

/**
 * Cool neutral ramp, ink → offwhite.
 *
 * Note: the product brief lists 500 as `#55555F`. That value is actually the
 * light-theme `--muted` semantic token, not a step on this ramp. The ramp
 * value below (`#5c5c6b`) is what the marketing site's global.css ships, and
 * global.css is the brand source of truth.
 */
export const gray = {
  50: '#f8f8fa',
  100: '#efeff3',
  200: '#dedee6',
  300: '#b9b9c6',
  400: '#8a8a99',
  500: '#5c5c6b',
  600: '#3f3f4d',
  700: '#2a2a35',
  800: '#1c1c24',
  900: '#121218',
  950: '#0a0a0f',
} as const;

/**
 * Status colours. Not part of the marketing brand — added for product surfaces
 * (source processing state, conversation state, usage meters). Deliberately
 * distinct from iris/aqua so brand accents keep their meaning.
 */
export const status = {
  success: '#12a67c',
  successSoft: '#d6f5ec',
  warning: '#b8791a',
  warningSoft: '#fbeeda',
  danger: '#d33d4b',
  dangerSoft: '#fbe0e3',
  info: '#3b6fd4',
  infoSoft: '#dfe9fb',
} as const;

/* -------------------------------------------------------------------------
 * Semantic surfaces (per theme)
 * ---------------------------------------------------------------------- */

export interface SemanticTheme {
  background: string;
  surface: string;
  surface2: string;
  surface3: string;
  foreground: string;
  muted: string;
  subtle: string;
  borderSoft: string;
  borderStrong: string;
  ring: string;
}

export const light: SemanticTheme = {
  background: '#fbfbfd',
  surface: '#ffffff',
  surface2: '#f6f6f9',
  surface3: '#efeff3',
  foreground: '#101018',
  muted: '#55555f',
  subtle: '#6a6a78',
  borderSoft: 'rgba(10, 10, 15, 0.08)',
  borderStrong: 'rgba(10, 10, 15, 0.14)',
  ring: '#5a5af0',
};

export const dark: SemanticTheme = {
  background: '#0a0a0f',
  surface: '#101017',
  surface2: '#16161f',
  surface3: '#1e1e29',
  foreground: '#f4f4f7',
  muted: '#a6a6b5',
  subtle: '#86868f',
  borderSoft: 'rgba(255, 255, 255, 0.08)',
  borderStrong: 'rgba(255, 255, 255, 0.16)',
  ring: '#4df0d8',
};

export const themes = { light, dark } as const;

export type ThemeName = keyof typeof themes;

/* -------------------------------------------------------------------------
 * Type, shape, depth, motion
 * ---------------------------------------------------------------------- */

/**
 * Font stacks. The Arabic face is a fallback in the Latin stacks on purpose:
 * Inter and Satoshi carry no Arabic glyphs, so a mixed "شحن مجاني over $50"
 * string resolves per-glyph to the right face. `arabic` promotes it to primary
 * for surfaces known to be Arabic.
 */
export const fonts = {
  sans: "'Inter', 'IBM Plex Sans Arabic', ui-sans-serif, system-ui, -apple-system, 'Segoe UI', sans-serif",
  display: "'Satoshi', 'Inter', 'IBM Plex Sans Arabic', ui-sans-serif, system-ui, sans-serif",
  arabic: "'IBM Plex Sans Arabic', ui-sans-serif, system-ui, sans-serif",
  mono: "ui-monospace, 'SF Mono', 'JetBrains Mono', 'Fira Code', monospace",
  /** No webfonts. The widget uses this so an embed costs no font bytes. */
  system:
    "system-ui, -apple-system, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, 'Noto Sans Arabic', sans-serif",
} as const;

export const radii = {
  xl: '1rem',
  '2xl': '1.25rem',
  '3xl': '1.75rem',
  '4xl': '2.25rem',
} as const;

export const shadows = {
  light: {
    soft: '0 1px 2px rgba(10, 10, 15, 0.04), 0 6px 20px -8px rgba(10, 10, 15, 0.12)',
    lifted: '0 2px 4px rgba(10, 10, 15, 0.04), 0 24px 48px -18px rgba(10, 10, 15, 0.2)',
  },
  dark: {
    soft: 'inset 0 1px 0 rgba(255, 255, 255, 0.04), 0 2px 8px -2px rgba(0, 0, 0, 0.6)',
    lifted: 'inset 0 1px 0 rgba(255, 255, 255, 0.05), 0 24px 60px -20px rgba(0, 0, 0, 0.8)',
  },
  glowIris: '0 10px 40px -10px rgba(90, 90, 240, 0.5)',
  glowAqua: '0 10px 40px -10px rgba(55, 224, 200, 0.45)',
} as const;

export const easing = {
  outExpo: 'cubic-bezier(0.16, 1, 0.3, 1)',
  outQuint: 'cubic-bezier(0.22, 1, 0.36, 1)',
  spring: 'cubic-bezier(0.34, 1.4, 0.64, 1)',
} as const;

/* -------------------------------------------------------------------------
 * Logo
 * ---------------------------------------------------------------------- */

/**
 * The Anis mark: gradient speech bubble with an orbiting companion dot.
 *
 * `uid` namespaces the gradient's SVG id. SVG ids are document-global, so two
 * marks on one page with the same id make the second one render with the
 * first one's gradient — and inside a shadow root, a `url(#id)` reference
 * cannot see a gradient defined in the outer document at all. Always pass a
 * stable unique id per rendered instance.
 */
export function logoMarkSvg(uid = 'anis'): string {
  return `<svg viewBox="0 0 40 40" fill="none" xmlns="http://www.w3.org/2000/svg" role="img" aria-label="Anis">
  <defs>
    <linearGradient id="${uid}-grad" x1="4" y1="6" x2="32" y2="32" gradientUnits="userSpaceOnUse">
      <stop stop-color="${brand.iris}" />
      <stop offset="1" stop-color="${brand.aqua}" />
    </linearGradient>
  </defs>
  <rect x="3" y="6" width="27" height="24" rx="9.5" fill="url(#${uid}-grad)" />
  <path d="M11 26 L8 34 L20 29 Z" fill="url(#${uid}-grad)" />
  <circle cx="35" cy="7" r="3.9" fill="${brand.iris}" />
</svg>`;
}

/** Wordmark, per locale. Latin is lowercase by design. */
export const wordmark = { en: 'anis', ar: 'أنيس' } as const;

/* -------------------------------------------------------------------------
 * Helpers
 * ---------------------------------------------------------------------- */

/**
 * Emit the semantic tokens as CSS custom-property declarations.
 *
 * The widget needs this: it renders into a shadow root, which does NOT inherit
 * the host page's custom properties from `theme.css` (the host page has never
 * loaded it). So the widget stamps its own `:host` block from these values.
 */
export function themeCssVars(theme: ThemeName): string {
  const t = themes[theme];
  return [
    `--background: ${t.background};`,
    `--surface: ${t.surface};`,
    `--surface-2: ${t.surface2};`,
    `--surface-3: ${t.surface3};`,
    `--foreground: ${t.foreground};`,
    `--muted: ${t.muted};`,
    `--subtle: ${t.subtle};`,
    `--border-soft: ${t.borderSoft};`,
    `--border-strong: ${t.borderStrong};`,
    `--ring: ${t.ring};`,
    `--gradient-brand: ${gradients.brand};`,
    `--shadow-soft: ${shadows[theme].soft};`,
    `--shadow-lifted: ${shadows[theme].lifted};`,
    `color-scheme: ${theme};`,
  ].join('\n');
}
