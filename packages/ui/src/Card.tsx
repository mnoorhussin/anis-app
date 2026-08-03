import type { ReactNode } from 'react';

export function Card({ children, className = '' }: { children: ReactNode; className?: string }) {
  return (
    <section
      className={`rounded-2xl border border-border-soft bg-surface p-6 shadow-soft ${className}`}
    >
      {children}
    </section>
  );
}
