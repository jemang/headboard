import { cn } from '../lib/cn'

/**
 * A placeholder shaped like the thing that is coming.
 *
 * The word "Loading…" tells someone to wait without saying what for, and the
 * page jumps when the real content replaces it. A skeleton reserves the space
 * and shows the shape, so arrival is a fill rather than a re-layout.
 */
export function Skeleton({ className }: { className?: string }) {
  return (
    <div
      aria-hidden
      className={cn('animate-pulse-soft rounded bg-surface-2', className)}
    />
  )
}

/** Rows shaped like the table that is loading. */
export function SkeletonRows({ rows = 5, cols = 4 }: { rows?: number; cols?: number }) {
  return (
    <div className="divide-y divide-border" aria-hidden>
      {Array.from({ length: rows }, (_, r) => (
        <div key={r} className="flex items-center gap-4 px-3 py-3">
          {Array.from({ length: cols }, (_, c) => (
            <Skeleton
              key={c}
              className={cn('h-4', c === 0 ? 'w-40' : c === cols - 1 ? 'w-16' : 'w-28')}
            />
          ))}
        </div>
      ))}
    </div>
  )
}

/** A labelled loading region for screen readers, paired with visual skeletons. */
export function Loading({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div role="status" aria-busy="true" aria-label={label}>
      <span className="sr-only">{label}</span>
      {children}
    </div>
  )
}
