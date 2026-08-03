import { useId } from 'react';

/**
 * The Anis mark: gradient speech bubble with an orbiting companion dot.
 *
 * The gradient's SVG id comes from `useId()` because SVG ids are
 * document-global — two logos on one page with a hard-coded id make the second
 * render with the first one's (possibly removed) gradient, which shows up as a
 * black or invisible mark only once a second logo appears somewhere.
 */
export function Logo({
  wordmark = true,
  lang = 'en',
  className = '',
}: {
  wordmark?: boolean;
  lang?: 'en' | 'ar';
  className?: string;
}) {
  const uid = useId();
  const gradientId = `anis-grad-${uid}`;

  return (
    <span className={`inline-flex items-center gap-2.5 ${className}`}>
      <svg
        className="h-8 w-8 shrink-0"
        viewBox="0 0 40 40"
        fill="none"
        xmlns="http://www.w3.org/2000/svg"
        aria-hidden="true"
      >
        <defs>
          <linearGradient
            id={gradientId}
            x1="4"
            y1="6"
            x2="32"
            y2="32"
            gradientUnits="userSpaceOnUse"
          >
            <stop stopColor="#5A5AF0" />
            <stop offset="1" stopColor="#37E0C8" />
          </linearGradient>
        </defs>
        <rect x="3" y="6" width="27" height="24" rx="9.5" fill={`url(#${gradientId})`} />
        <path d="M11 26 L8 34 L20 29 Z" fill={`url(#${gradientId})`} />
        <circle cx="35" cy="7" r="3.9" fill="#5A5AF0" />
      </svg>
      {wordmark ? (
        <span
          className={`font-display text-xl font-bold ${lang === 'ar' ? '' : 'lowercase tracking-tight'}`}
          lang={lang}
        >
          {lang === 'ar' ? 'أنيس' : 'anis'}
        </span>
      ) : (
        <span className="sr-only">Anis</span>
      )}
    </span>
  );
}
