import { readFileSync } from 'node:fs';
import { runInNewContext } from 'node:vm';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { initialLanguage, persistLanguage } from './i18n.js';

const html = readFileSync(new URL('../index.html', import.meta.url), 'utf8');
const bootScript = html.match(/<script>([\s\S]*?)<\/script>/)![1]!;
afterEach(() => vi.unstubAllGlobals());

describe('Arabic-first dashboard', () => {
  it.each([null, 'invalid', 'ar', 'en'])(
    'respects stored choice %s, never browser language',
    (stored) => {
      const getItem = (key: string) => (key === 'anis-lang' ? stored : null);
      vi.stubGlobal('localStorage', { getItem });
      vi.stubGlobal('navigator', { language: 'en-US' });
      const expected = stored === 'en' ? 'en' : 'ar';
      expect(initialLanguage()).toBe(expected);
      expect(html).toContain('<html lang="ar" dir="rtl">');
      const root = { lang: 'ar', dir: 'rtl', setAttribute: vi.fn() };
      runInNewContext(bootScript, {
        localStorage: { getItem },
        document: { documentElement: root },
      });
      expect(root.lang).toBe(expected);
      expect(root.dir).toBe(expected === 'ar' ? 'rtl' : 'ltr');
    },
  );
  it('remembers English explicitly and tolerates blocked storage', () => {
    const setItem = vi.fn();
    vi.stubGlobal('localStorage', { setItem });
    persistLanguage('en');
    expect(setItem).toHaveBeenCalledWith('anis-lang', 'en');
    vi.stubGlobal('localStorage', {
      getItem() {
        throw new Error('blocked');
      },
      setItem() {
        throw new Error('blocked');
      },
    });
    expect(initialLanguage()).toBe('ar');
    expect(() => persistLanguage('en')).not.toThrow();
  });
});
