/** Dar tokens. CSS and TS are maintained together; tokens.test.ts guards all shared values.
 * Brand source: anis-chat/src/styles/global.css; product extensions: DESIGN-DIRECTION.md §5.
 */
export const brand = {
  ink: '#231a10',
  paper: '#faf4e8',
  card: '#fffcf5',
  oasis: '#0f6b5c',
  oasisDeep: '#0c594d',
  mint: '#4fc5b0',
  saffron: '#e3a84e',
  saffronInk: '#8a5a12',
  saffronTint: '#f3e4c8',
  terracotta: '#b4441f',
  espresso: '#1c1610',
  cream: '#f4ecdd',
} as const;

export const gray = {
  '50': '#f5efe3',
  '100': '#ebe3d3',
  '200': '#dcd2be',
  '300': '#bfb29a',
  '400': '#968970',
  '500': '#6b5d4a',
  '600': '#524636',
  '700': '#3d3428',
  '800': '#2e271d',
  '900': '#231a10',
  '950': '#1c1610',
} as const;

export const status = {
  success: '#1f7a4d',
  warning: '#9a6a00',
  danger: '#b3382d',
  info: '#2f5fa8',
  successSoft: '#e0e5d5',
  warningSoft: '#eee3cc',
  dangerSoft: '#f1ddd2',
  infoSoft: '#e2e2e0',
} as const;

/** PRODUCT: text-safe status colors. Keep §3.2 hues for fills, but use these
 * for small text (including on tinted badges) and for espresso surfaces. */
export const statusForeground = {
  light: {
    success: '#1b6b43',
    warning: '#805800',
    danger: '#b3382d',
    info: '#2f5fa8',
  },
  dark: {
    success: '#78c69a',
    warning: '#e3b65e',
    danger: '#ed9588',
    info: '#95b8e3',
  },
} as const;
export const darkStatusSoft = {
  successSoft: '#23352a',
  warningSoft: '#3d321e',
  dangerSoft: '#3d2822',
  infoSoft: '#27313d',
} as const;

export const fonts = {
  sans: "'Inter', 'IBM Plex Sans Arabic', ui-sans-serif, system-ui, -apple-system, 'Segoe UI', sans-serif",
  display: "'Fraunces', 'Amiri', Georgia, 'Times New Roman', serif",
  arabic: "'IBM Plex Sans Arabic', ui-sans-serif, system-ui, sans-serif",
  mono: "ui-monospace, 'SF Mono', 'JetBrains Mono', 'Fira Code', monospace",
  displayArabic: "'Amiri', 'IBM Plex Sans Arabic', serif",
  system:
    "system-ui, -apple-system, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, 'Noto Sans Arabic', sans-serif",
} as const;

export const radii = {
  lg: '0.5rem',
  xl: '0.625rem',
  '2xl': '0.875rem',
  '3xl': '1.25rem',
  '4xl': '1.5rem',
} as const;

export const easing = {
  outExpo: 'cubic-bezier(0.16, 1, 0.3, 1)',
  outQuint: 'cubic-bezier(0.22, 1, 0.36, 1)',
  spring: 'cubic-bezier(0.34, 1.4, 0.64, 1)',
} as const;

export const displayType = {
  'display-2xl': 'clamp(2.6rem, 1.6rem + 4.6vw, 4.6rem)',
  'display-2xl--line-height': '1.08',
  'display-2xl--letter-spacing': '-0.01em',
  'display-2xl--font-weight': '600',
  'display-xl': 'clamp(2.3rem, 1.6rem + 3.2vw, 3.7rem)',
  'display-xl--line-height': '1.1',
  'display-xl--letter-spacing': '-0.01em',
  'display-xl--font-weight': '600',
  'display-lg': 'clamp(1.9rem, 1.4rem + 2.3vw, 2.9rem)',
  'display-lg--line-height': '1.12',
  'display-lg--letter-spacing': '-0.005em',
  'display-lg--font-weight': '600',
  'display-md': 'clamp(1.6rem, 1.3rem + 1.4vw, 2.2rem)',
  'display-md--line-height': '1.15',
  'display-md--letter-spacing': '0em',
  'display-md--font-weight': '600',
} as const;

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
  accent: string;
  accentStrong: string;
  accentWarm: string;
}
export const light: SemanticTheme = {
  background: '#faf4e8',
  surface: '#fffcf5',
  surface2: '#f5efe3',
  surface3: '#ebe3d3',
  foreground: '#231a10',
  muted: '#6b5d4a',
  subtle: '#7d6f5a',
  borderSoft: 'rgba(35, 26, 16, 0.1)',
  borderStrong: 'rgba(35, 26, 16, 0.18)',
  ring: '#0f6b5c',
  accent: '#0f6b5c',
  accentStrong: '#0c594d',
  accentWarm: '#8a5a12',
};
export const dark: SemanticTheme = {
  background: '#1c1610',
  surface: '#262018',
  surface2: '#2e271d',
  surface3: '#3d3428',
  foreground: '#f4ecdd',
  muted: '#b3a48d',
  subtle: '#968970',
  borderSoft: 'rgba(244, 236, 221, 0.1)',
  borderStrong: 'rgba(244, 236, 221, 0.2)',
  ring: '#4fc5b0',
  accent: '#4fc5b0',
  accentStrong: '#6ad4c1',
  accentWarm: '#e3a84e',
};
export const themes = { light, dark } as const;
export type ThemeName = keyof typeof themes;
export const shadows = {
  light: {
    soft: '0 1px 0 rgba(35, 26, 16, 0.06), 0 8px 24px -16px rgba(35, 26, 16, 0.25)',
    lifted: '0 1px 0 rgba(35, 26, 16, 0.06), 0 20px 40px -24px rgba(35, 26, 16, 0.35)',
  },
  dark: {
    soft: 'inset 0 1px 0 rgba(244, 236, 221, 0.04), 0 2px 8px -2px rgba(0, 0, 0, 0.5)',
    lifted: 'inset 0 1px 0 rgba(244, 236, 221, 0.05), 0 24px 48px -20px rgba(0, 0, 0, 0.65)',
  },
} as const;

/** Shared geometry from anis-chat's mark. Also used by the animated typing indicator. */
export const ijamDots = [
  { cx: 20, cy: 13.5, r: 4.6 },
  { cx: 12.8, cy: 26, r: 4.6 },
  { cx: 27.2, cy: 26, r: 4.6 },
] as const;

/** No document-global IDs: safe to repeat, including in shadow roots. */
export function logoMarkSvg(): string {
  return `<svg viewBox="0 0 40 40" xmlns="http://www.w3.org/2000/svg" role="img" aria-label="أنيس"><rect x="1" y="1" width="38" height="38" rx="10.5" fill="${brand.ink}"/><g fill="${brand.saffron}">${ijamDots.map((d) => `<circle cx="${d.cx}" cy="${d.cy}" r="${d.r}"/>`).join('')}</g></svg>`;
}
export const wordmark = { ar: 'أنيس', en: 'Anis' } as const;

export function themeCssVars(theme: ThemeName): string {
  return [
    ...Object.entries(themes[theme]).map(
      ([key, value]) =>
        `--${key.replace(/[A-Z]/g, (c) => `-${c.toLowerCase()}`).replace(/([a-z])(\d)/g, '$1-$2')}: ${value};`,
    ),
    ...Object.entries(statusForeground[theme]).map(([key, value]) => `--status-${key}: ${value};`),
    ...(theme === 'dark'
      ? Object.entries(darkStatusSoft).map(
          ([key, value]) =>
            `--color-${key.replace(/[A-Z]/g, (c) => `-${c.toLowerCase()}`)}: ${value};`,
        )
      : []),
    `--shadow-soft: ${shadows[theme].soft};`,
    `--shadow-lifted: ${shadows[theme].lifted};`,
    `color-scheme: ${theme};`,
  ].join('\n');
}
