/**
 * Builds the `:host` custom-property block for the shadow root.
 *
 * Exists because a shadow root inherits nothing from the page's stylesheets:
 * the tokens the dashboard gets from `theme.css`'s `:root` block are simply
 * absent here. And the accent is per-workspace anyway, so the palette has to
 * be assembled at runtime from tokens + config regardless.
 */

import { themes, type ThemeName } from '@anis/tokens';

/**
 * Pick black or white text for a given background.
 *
 * Businesses choose their own accent, and a good number of them will choose
 * something pale. Hard-coding white text on the accent means an aqua or yellow
 * brand ships unreadable buttons. Uses the WCAG relative-luminance formula so
 * the choice is the one an accessibility audit would make.
 */
export function contrastOn(hex: string): '#ffffff' | '#0a0a0f' {
  const n = parseInt(hex.replace('#', ''), 16);
  const channel = (c: number) => {
    const s = c / 255;
    return s <= 0.03928 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4;
  };
  const luminance =
    0.2126 * channel((n >> 16) & 255) +
    0.7152 * channel((n >> 8) & 255) +
    0.0722 * channel(n & 255);
  // Contrast against white vs against black; pick whichever is higher.
  return 1.05 / (luminance + 0.05) >= (luminance + 0.05) / 0.05 ? '#ffffff' : '#0a0a0f';
}

export function hostVars(theme: ThemeName, accent: string): string {
  const t = themes[theme];
  return `:host{
  --background:${t.background};
  --surface:${t.surface};
  --surface-2:${t.surface2};
  --foreground:${t.foreground};
  --muted:${t.muted};
  --border-soft:${t.borderSoft};
  --accent:${accent};
  --accent-contrast:${contrastOn(accent)};
  color-scheme:${theme};
}`;
}

/**
 * Resolve `auto` against the host page.
 *
 * Matches the page the widget sits on rather than the visitor's OS: a business
 * with a light site that a dark-mode visitor is browsing should still get a
 * light widget, or it looks like a hole punched in their page.
 */
export function resolveTheme(setting: 'light' | 'dark' | 'auto'): ThemeName {
  if (setting !== 'auto') return setting;
  try {
    const pageBg = getComputedStyle(document.body).backgroundColor;
    const rgb = pageBg.match(/\d+/g)?.slice(0, 3).map(Number);
    if (rgb && rgb.length === 3) {
      const [r, g, b] = rgb as [number, number, number];
      // Ignore a fully transparent body, which reports as rgba(0,0,0,0) and
      // would otherwise always read as "dark".
      const alpha = pageBg.match(/[\d.]+\)$/)?.[0];
      if (!(pageBg.startsWith('rgba') && alpha === '0)')) {
        return (r * 299 + g * 587 + b * 114) / 1000 < 128 ? 'dark' : 'light';
      }
    }
  } catch {
    /* cross-origin or exotic document; fall through */
  }
  return window.matchMedia?.('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
}
