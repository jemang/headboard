import type { ReactNode } from 'react'
import { ChevronRight } from 'lucide-react'

/**
 * A phone row. The open control and anything in `aside` (a copy button) are
 * siblings: nesting a button inside the row button would let copying open the
 * sheet.
 */
export function ListRow({
  title,
  meta,
  trailing,
  onOpen,
  openLabel,
  aside,
}: {
  title: ReactNode
  meta?: ReactNode
  trailing?: ReactNode
  onOpen?: () => void
  openLabel?: string
  aside?: ReactNode
}) {
  const body = (
    <>
      <span className="min-w-0 flex-1">
        <span className="flex items-start justify-between gap-3">
          <span className="min-w-0 truncate font-medium">{title}</span>
          {trailing && <span className="shrink-0">{trailing}</span>}
        </span>
        {meta && <span className="mt-0.5 block truncate text-sm text-muted-foreground">{meta}</span>}
      </span>
      {onOpen && <ChevronRight aria-hidden className="size-4 shrink-0 text-muted-foreground" strokeWidth={1.5} />}
    </>
  )

  return (
    <li className="flex items-center border-t border-border first:border-t-0">
      {onOpen ? (
        <button
          type="button"
          onClick={onOpen}
          aria-label={openLabel}
          className="flex min-h-14 min-w-0 flex-1 items-center gap-3 px-4 py-2.5 text-left transition-colors active:bg-surface-2"
        >
          {body}
        </button>
      ) : (
        <div className="flex min-h-14 min-w-0 flex-1 items-center gap-3 px-4 py-2.5">{body}</div>
      )}
      {aside && <div className="shrink-0 pr-2">{aside}</div>}
    </li>
  )
}

export function CardList({ children, label }: { children: ReactNode; label: string }) {
  return <ul aria-label={label} className="overflow-hidden rounded-xl border border-border bg-surface-1 shadow-card">{children}</ul>
}
