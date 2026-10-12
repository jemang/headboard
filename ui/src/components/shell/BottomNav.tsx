import { Ellipsis } from 'lucide-react'
import { Link } from '../../lib/router'
import type { NavItem, RouteId } from '../../lib/routes'
import { cn } from '../../lib/cn'
import { ICONS } from './icons'

const cell = 'flex min-h-14 flex-1 flex-col items-center justify-center gap-0.5 text-[0.6875rem] font-medium transition-colors'

/** Phone navigation: up to four pages the role uses most, then More. */
export function BottomNav({ items, active, moreActive, onMore }: { items: NavItem[]; active: RouteId | null; moreActive: boolean; onMore: () => void }) {
  return (
    <nav
      aria-label="Primary navigation"
      className="fixed inset-x-0 bottom-0 z-30 flex border-t border-rail-border bg-rail/95 pb-[env(safe-area-inset-bottom)] backdrop-blur"
    >
      {items.map((i) => {
        const Icon = ICONS[i.id]
        const on = active === i.id

        return (
          <Link key={i.id} to={i.to} className={cn(cell, on ? 'text-accent-400' : 'text-rail-muted')}>
            <Icon aria-hidden className="size-5" strokeWidth={on ? 2 : 1.5} />
            <span>{i.short}</span>
            {on && <span className="sr-only">(current page)</span>}
          </Link>
        )
      })}
      <button type="button" onClick={onMore} className={cn(cell, moreActive ? 'text-accent-400' : 'text-rail-muted')}>
        <Ellipsis aria-hidden className="size-5" strokeWidth={1.5} />
        <span>More</span>
      </button>
    </nav>
  )
}
