import { type SupportedLanguage, dirFor } from '@anis/types';
import { type ReactNode, createContext, useCallback, useContext, useEffect, useState } from 'react';

import { type StringKey, initialLanguage, persistLanguage, strings } from '../i18n.js';

interface LanguageValue {
  lang: SupportedLanguage;
  dir: 'rtl' | 'ltr';
  setLang: (lang: SupportedLanguage) => void;
  /** Look up a UI string in the current language. */
  t: (key: StringKey) => string;
}

const LanguageContext = createContext<LanguageValue | null>(null);

export function LanguageProvider({ children }: { children: ReactNode }) {
  const [lang, setLangState] = useState<SupportedLanguage>(initialLanguage);
  const dir = dirFor(lang);

  // Drive `lang` and `dir` on <html>, not on a wrapper div. Both are what the
  // token stylesheet keys off to swap in the Arabic face, and `dir` on the
  // root is also what makes the browser mirror scrollbars, form controls and
  // the default text alignment.
  useEffect(() => {
    document.documentElement.lang = lang;
    document.documentElement.dir = dir;
  }, [lang, dir]);

  const setLang = useCallback((next: SupportedLanguage) => {
    setLangState(next);
    persistLanguage(next);
  }, []);

  const t = useCallback((key: StringKey) => strings[lang][key], [lang]);

  return (
    <LanguageContext.Provider value={{ lang, dir, setLang, t }}>
      {children}
    </LanguageContext.Provider>
  );
}

export function useLanguage(): LanguageValue {
  const ctx = useContext(LanguageContext);
  if (!ctx) throw new Error('useLanguage must be used inside <LanguageProvider>');
  return ctx;
}
