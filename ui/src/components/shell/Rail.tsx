import { ChevronRight, LogOut, Moon, Sun } from 'lucide-react'
import type { Health, Me } from '../../lib/api'
import { withBase } from '../../lib/basePath'
import { Link } from '../../lib/router'
import { navItems, type RouteId } from '../../lib/routes'
import type { Theme } from '../../lib/theme'
import { cn } from '../../lib/cn'
import { BrandMark } from '../BrandMark'
import { Button } from '../ui'
import { HealthWord } from './HealthWord'
import { ICONS } from './icons'

/** The desktop sidebar: every page the role can open, Headscale's state, then the account. */
export function Rail({
  me,
  active,
  theme,
  setTheme,
  health,
  unavailable,
  needCount,
  showNeeds,
}: {
  me: Me
  active: RouteId | null
  theme: Theme
  setTheme: (t: Theme) => void
  health?: Health
  unavailable: boolean
  needCount: number
  /** Off on Devices, where the needs list is already on the page. */
  showNeeds: boolean
}) {
  const links = navItems(me.capabilities).all.filter((i) => i.id !== 'account')

  return (
    <aside className="fixed inset-y-0 left-0 z-30 flex w-64 flex-col overflow-hidden border-r border-side-border bg-side-wash px-3 py-5">
      <RailArt />
      <Link to="/devices" className="relative mb-8 flex items-center gap-3 px-3">
        <BrandMark className="size-9 text-side-fg" />
        <span>
          <span className="block text-base font-semibold tracking-tight text-side-fg">Headboard</span>
          <span className="block text-[0.6875rem] text-side-muted">Headscale control plane</span>
        </span>
      </Link>
      <nav className="relative space-y-1" aria-label="Primary navigation">
        {links.map((l) => {
          const Icon = ICONS[l.id]

          return (
            <Link
              key={l.id}
              to={l.to}
              className={cn(
                'flex items-center gap-3 rounded-lg px-3 py-2.5 text-sm transition-colors',
                active === l.id ? 'bg-side-active font-semibold text-side-active-fg' : 'text-side-muted hover:bg-side-hover hover:text-side-fg',
              )}
            >
              <Icon aria-hidden className="size-4.5 shrink-0" strokeWidth={1.7} />
              {l.label}
            </Link>
          )
        })}
      </nav>
      <div className="relative mt-auto space-y-3">
        <Link
          to="/devices#needs"
          onClick={() => document.getElementById('needs')?.scrollIntoView({ block: 'start' })}
          className="block rounded-xl border border-side-border bg-surface-1 p-3 shadow-card transition-colors hover:border-accent-500/50"
        >
          <span className="flex items-center justify-between gap-2">
            <HealthWord health={health} unavailable={unavailable} onSide />
            <ChevronRight aria-hidden className="size-4 shrink-0 text-side-muted" strokeWidth={1.7} />
          </span>
          {showNeeds && needCount > 0 && (
            <span className="mt-2 inline-flex rounded-md border border-attention/40 bg-attention/10 px-2 py-0.5 text-xs font-medium text-warn">
              {needCount} need{needCount === 1 ? 's' : ''} you
            </span>
          )}
        </Link>
        <div className="space-y-3 border-t border-side-border px-3 pt-4">
          <Link to="/account" className={cn('block rounded-lg p-1 transition-colors hover:bg-side-hover', active === 'account' && 'bg-side-active')}>
            <span className="block truncate text-sm font-medium text-side-fg">{me.user.email}</span>
            <span className="font-mono text-xs text-side-muted">{me.user.role}</span>
          </Link>
          <div className="flex items-center gap-1">
            <button
              type="button"
              aria-label={theme === 'dark' ? 'Switch to the light theme' : 'Switch to the dark theme'}
              onClick={() => setTheme(theme === 'dark' ? 'light' : 'dark')}
              className="rounded-lg p-2 text-side-muted transition-colors hover:bg-side-hover hover:text-side-fg"
            >
              {theme === 'dark' ? <Sun aria-hidden className="size-4" strokeWidth={1.5} /> : <Moon aria-hidden className="size-4" strokeWidth={1.5} />}
            </button>
            <form method="post" action={withBase('/auth/logout')}>
              <Button type="submit" variant="ghost" icon={LogOut} title="Sign out" className="px-2 text-side-muted hover:bg-side-hover hover:text-side-fg">
                <span className="sr-only">Sign out</span>
              </Button>
            </form>
          </div>
        </div>
      </div>
    </aside>
  )
}

/** A faint tailnet behind the lower sidebar: five devices on a soft wave. Pure decoration. */
function RailArt() {
  return (
    <svg aria-hidden="true" viewBox="0 0 256 420" className="pointer-events-none absolute inset-x-0 bottom-0 h-[26rem] w-full text-side-art">
      <path d="M0 250C70 205 125 290 256 165V420H0Z" fill="currentColor" opacity="0.14" />
      <path d="M0 320C90 275 150 350 256 262V420H0Z" fill="currentColor" opacity="0.12" />
      <g stroke="currentColor" strokeWidth="1" opacity="0.45" fill="none">
        <path d="M40 62 120 100 204 72M120 100 90 168 182 150 204 72" />
      </g>
      <g fill="currentColor" opacity="0.6">
        <circle cx="40" cy="62" r="4.5" />
        <circle cx="120" cy="100" r="5.5" />
        <circle cx="204" cy="72" r="4.5" />
        <circle cx="90" cy="168" r="5.5" />
        <circle cx="182" cy="150" r="4.5" />
      </g>
    </svg>
  )
}
