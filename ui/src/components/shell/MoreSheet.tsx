import { useRef } from 'react'
import { createPortal } from 'react-dom'
import { ChevronRight, LogOut, Moon, Sun } from 'lucide-react'
import type { Me } from '../../lib/api'
import { withBase } from '../../lib/basePath'
import { useModal } from '../../lib/modal'
import { Link } from '../../lib/router'
import type { NavItem } from '../../lib/routes'
import type { Theme } from '../../lib/theme'
import { ICONS } from './icons'

const row = 'flex min-h-14 w-full items-center gap-3 px-5 text-left text-sm transition-colors active:bg-surface-2'

/** The pages and preferences that do not fit in the bottom bar. */
export function MoreSheet({
  open,
  onClose,
  items,
  me,
  theme,
  setTheme,
}: {
  open: boolean
  onClose: () => void
  items: NavItem[]
  me: Me
  theme: Theme
  setTheme: (t: Theme) => void
}) {
  const panel = useRef<HTMLDivElement>(null)
  useModal(open, panel, onClose)

  if (!open) return null

  return createPortal(
    <div className="fixed inset-0 z-50 flex items-end">
      <button type="button" aria-label="Close" tabIndex={-1} className="absolute inset-0 bg-black/50 animate-fade-in" onClick={onClose} />
      <div
        ref={panel}
        role="dialog"
        aria-modal="true"
        aria-label="More"
        className="relative z-50 w-full rounded-t-2xl border-t border-border bg-surface-1 pb-[max(1rem,env(safe-area-inset-bottom))] pt-2 shadow-raised outline-none animate-rise"
      >
        <div aria-hidden className="mx-auto mb-2 h-1 w-10 rounded-full bg-border" />
        <ul>
          {items.map((i) => {
            const Icon = ICONS[i.id]

            return (
              <li key={i.id}>
                <Link to={i.to} onClick={onClose} className={row}>
                  <Icon aria-hidden className="size-5 text-muted-foreground" strokeWidth={1.5} />
                  <span className="flex-1">{i.label}</span>
                  <ChevronRight aria-hidden className="size-4 text-muted-foreground" strokeWidth={1.5} />
                </Link>
              </li>
            )
          })}
          <li>
            <button type="button" className={row} onClick={() => setTheme(theme === 'dark' ? 'light' : 'dark')}>
              {theme === 'dark' ? <Sun aria-hidden className="size-5 text-muted-foreground" strokeWidth={1.5} /> : <Moon aria-hidden className="size-5 text-muted-foreground" strokeWidth={1.5} />}
              <span className="flex-1">{theme === 'dark' ? 'Light theme' : 'Dark theme'}</span>
            </button>
          </li>
        </ul>
        <div className="mx-5 mt-2 flex items-center gap-3 border-t border-border pt-3">
          <span className="min-w-0 flex-1">
            <span className="block truncate text-sm font-medium">{me.user.email}</span>
            <span className="font-mono text-xs text-muted-foreground">{me.user.role}</span>
          </span>
          <form method="post" action={withBase('/auth/logout')}>
            <button type="submit" className="inline-flex h-10 items-center gap-1.5 rounded-lg px-3 text-sm text-muted-foreground hover:bg-surface-2 hover:text-foreground">
              <LogOut aria-hidden className="size-4" strokeWidth={1.5} />
              Sign out
            </button>
          </form>
        </div>
      </div>
    </div>,
    document.body,
  )
}
