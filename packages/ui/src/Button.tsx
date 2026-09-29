import type { ButtonHTMLAttributes, ReactNode } from 'react';

export interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  /**
   * `primary` uses solid oasis with cream text. Use at most one per view — it is
   * the brand's loudest element and stops meaning "the main action" as soon as
   * there are two of them.
   */
  variant?: 'primary' | 'secondary' | 'ghost' | 'danger';
  size?: 'sm' | 'md';
  children: ReactNode;
}

const base =
  'relative inline-flex items-center justify-center gap-2 rounded-xl font-medium ' +
  'transition-[transform,box-shadow,background-color,opacity] duration-200 ease-out-quint ' +
  'focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring ' +
  'disabled:pointer-events-none disabled:opacity-50 active:translate-y-px ' +
  // Logical padding, so RTL mirrors without a second rule.
  'overflow-hidden';

const variants = {
  primary: 'bg-oasis text-cream hover:bg-oasis-deep',
  secondary: 'bg-surface-2 text-foreground hover:bg-surface-3 border border-border-soft',
  ghost: 'text-muted hover:bg-surface-2 hover:text-foreground',
  danger: 'bg-danger text-white hover:brightness-110',
} as const;

const sizes = {
  sm: 'text-sm ps-3 pe-3 py-1.5',
  md: 'text-sm ps-5 pe-5 py-2.5',
} as const;

export function Button({
  variant = 'secondary',
  size = 'md',
  className = '',
  type = 'button',
  children,
  ...rest
}: ButtonProps) {
  return (
    <button
      // Defaulting to "button" rather than the HTML default "submit" — a
      // submit-by-default button inside a settings form fires the form on
      // every stray click.
      type={type}
      className={`${base} ${variants[variant]} ${sizes[size]} ${className}`}
      {...rest}
    >
      {children}
    </button>
  );
}
