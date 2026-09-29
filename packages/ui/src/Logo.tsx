import { brand, fonts, ijamDots } from '@anis/tokens';

/** Arabic leads in both locales. The mark shares its geometry with the typing dots. */
export function Logo({
  wordmark = true,
  lang = 'ar',
  className = '',
}: {
  wordmark?: boolean;
  lang?: 'en' | 'ar';
  className?: string;
}) {
  return (
    <span className={`inline-flex items-center gap-2.5 ${className}`}>
      <svg className="h-8 w-8 shrink-0" viewBox="0 0 40 40" aria-hidden="true">
        <rect x="1" y="1" width="38" height="38" rx="10.5" fill={brand.ink} />
        {ijamDots.map((dot, i) => (
          <circle key={i} {...dot} fill={brand.saffron} />
        ))}
      </svg>
      {wordmark ? (
        <span
          className="inline-flex flex-col leading-none"
          aria-label={lang === 'ar' ? 'أنيس' : 'Anis'}
        >
          <span lang="ar" className="text-xl font-bold" style={{ fontFamily: fonts.displayArabic }}>
            أنيس
          </span>
          <span lang="en" className="text-xs" style={{ fontFamily: fonts.display }}>
            Anis
          </span>
        </span>
      ) : (
        <span className="sr-only">أنيس</span>
      )}
    </span>
  );
}
