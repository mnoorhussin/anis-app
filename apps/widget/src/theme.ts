/**
 * Builds the `:host` custom-property block for the shadow root.
 *
 * Exists because a shadow root inherits nothing from the page's stylesheets:
 * the tokens the dashboard gets from `theme.css`'s `:root` block are simply
 * absent here. And the accent is per-workspace anyway, so the palette has to
 * be assembled at runtime from tokens + config regardless.
 */

import { brand, shadows, statusForeground, themes, type ThemeName } from '@anis/tokens';

/** WCAG 2.x relative luminance for a validated, opaque six-digit hex. */
export function luminance(hex: string): number {
  const n = parseInt(hex.slice(1), 16);
  const channel = (c: number) => {
    const s = c / 255;
    return s <= 0.04045 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4;
  };
  return (
    0.2126 * channel((n >> 16) & 255) + 0.7152 * channel((n >> 8) & 255) + 0.0722 * channel(n & 255)
  );
}

export function contrastRatio(a: string, b: string): number {
  const x = luminance(a),
    y = luminance(b);
  return (Math.max(x, y) + 0.05) / (Math.min(x, y) + 0.05);
}

/** Accept only opaque hex colors, never arbitrary CSS in the shadow stylesheet. */
export function normalizeAccent(value: string): string {
  if (/^#[0-9a-f]{6}$/i.test(value)) return value.toLowerCase();
  if (/^#[0-9a-f]{3}$/i.test(value))
    return (
      '#' +
      [...value.slice(1)]
        .map((c) => c + c)
        .join('')
        .toLowerCase()
    );
  return brand.oasis;
}

/** Prefer the brand's cream/ink. Some midtones cannot reach AA with either:
 * use black/white only for that gap, preserving the customer's chosen fill. */
export function contrastOn(hex: string): string {
  const accent = normalizeAccent(hex);
  const candidates = [brand.cream, brand.ink];
  candidates.sort((a, b) => contrastRatio(accent, b) - contrastRatio(accent, a));
  const best = candidates[0]!;
  if (contrastRatio(accent, best) >= 4.5) return best;
  return contrastRatio(accent, '#ffffff') >= contrastRatio(accent, '#000000')
    ? '#ffffff'
    : '#000000';
}

export function hostVars(theme: ThemeName, accent: string): string {
  const t = themes[theme];
  const safeAccent = normalizeAccent(accent);
  return `:host{
  --background:${t.background};
  --surface:${t.surface};
  --surface-2:${t.surface2};
  --foreground:${t.foreground};
  --muted:${t.muted};
  --border-soft:${t.borderSoft};
  --accent:${safeAccent};
  --accent-contrast:${contrastOn(safeAccent)};
  --saffron:${brand.saffron};
  --danger:${statusForeground[theme].danger};
  --shadow-lifted:${shadows[theme].lifted};
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
