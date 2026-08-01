import type { ReactNode } from 'react'
import { cn } from '../lib/cn'

/**
 * The table frame Devices, People and Keys all share.
 *
 * They each used to declare the same `overflow-x-auto rounded-lg border` and
 * `thead` markup with slightly different padding. Loading and empty states go
 * *inside* the frame rather than replacing it, so the page does not jump
 * between states.
 */
export function Table({
  columns,
  children,
  className,
}: {
  columns: (string | { label: string; align?: 'left' | 'right' })[]
  children: ReactNode
  className?: string
}) {
  return (
    <div className={cn('overflow-x-auto rounded-xl border border-border bg-surface-1 shadow-sm', className)}>
      <table className="w-full min-w-max text-sm">
        <thead>
          <tr>
            {columns.map((c) => {
              const col = typeof c === 'string' ? { label: c, align: 'left' as const } : c

              return (
                <th
                  key={col.label}
                  scope="col"
                  className={cn(
                    // Sticky so column names survive a long device list.
                    'sticky top-0 z-10 bg-surface-2/95 px-4 py-3 text-eyebrow font-semibold uppercase text-muted-foreground backdrop-blur',
                    col.align === 'right' ? 'text-right' : 'text-left',
                  )}
                >
                  {col.label}
                </th>
              )
            })}
          </tr>
        </thead>
        <tbody>{children}</tbody>
      </table>
    </div>
  )
}

/**
 * A row. `onClick` makes it keyboard-reachable as well as clickable — the
 * device table has always opened a drawer on click and was mouse-only.
 */
export function Row({
  children,
  onClick,
  label,
}: {
  children: ReactNode
  onClick?: () => void
  /** What activating this row does, for screen readers. */
  label?: string
}) {
  if (!onClick) {
    return <tr className="border-t border-border">{children}</tr>
  }

  return (
    <tr
      tabIndex={0}
      role="button"
      aria-label={label}
      onClick={onClick}
      onKeyDown={(e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault()
          onClick()
        }
      }}
      className="cursor-pointer border-t border-border transition-colors hover:bg-accent-500/8 focus-visible:bg-accent-500/10 active:bg-surface-2"
    >
      {children}
    </tr>
  )
}

export function Cell({
  children,
  align,
  muted,
  className,
}: {
  children: ReactNode
  align?: 'left' | 'right'
  muted?: boolean
  className?: string
}) {
  return (
    <td
      className={cn(
        'px-4 py-3 align-middle',
        align === 'right' && 'text-right',
        muted && 'text-muted-foreground',
        className,
      )}
    >
      {children}
    </td>
  )
}

/** A full-width row for the empty and loading states, keeping the frame. */
export function SpanRow({ columns, children }: { columns: number; children: ReactNode }) {
  return (
    <tr className="border-t border-border">
      <td colSpan={columns} className="px-3 py-10 text-center">
        {children}
      </td>
    </tr>
  )
}
