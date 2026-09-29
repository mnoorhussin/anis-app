import type { ReactNode } from 'react';

export type BadgeTone = 'neutral' | 'brand' | 'success' | 'warning' | 'danger' | 'info';

const tones: Record<BadgeTone, string> = {
  neutral: 'bg-surface-3 text-muted',
  brand: 'bg-accent/12 text-accent',
  success: 'bg-success-soft text-status-success',
  warning: 'bg-warning-soft text-status-warning',
  danger: 'bg-danger-soft text-status-danger',
  info: 'bg-info-soft text-status-info',
};

export function Badge({ tone = 'neutral', children }: { tone?: BadgeTone; children: ReactNode }) {
  return (
    <span
      className={`inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-xs font-medium ${tones[tone]}`}
    >
      {children}
    </span>
  );
}
