import { useEffect, useState } from 'react'
import type { Health } from '../lib/api'
import { Link } from '../lib/router'
import { HealthWord } from './shell/HealthWord'

function useNow(every: number) {
  const [now, setNow] = useState(() => Date.now())

  useEffect(() => {
    const t = setInterval(() => setNow(Date.now()), every)

    return () => clearInterval(t)
  }, [every])

  return now
}

/** Same-URL links do not navigate, so a repeat click scrolls directly. */
function scrollToNeeds() {
  document.getElementById('needs')?.scrollIntoView({ block: 'start' })
}

/** One line: is Headscale up, how much of the tailnet is online, and is anything waiting. */
export function HealthBand({
  health,
  unavailable,
  online,
  total,
  needCount,
  updatedAt,
}: {
  health?: Health
  unavailable: boolean
  online: number
  total: number
  needCount: number
  /** When the device list last arrived; live updates refresh it. */
  updatedAt: number
}) {
  const now = useNow(10_000)
  const age = Math.max(0, Math.round((now - updatedAt) / 1000))

  return (
    <section aria-label="Tailnet status" className="flex flex-wrap items-center gap-x-2.5 gap-y-1 rounded-xl border border-border bg-surface-1 px-4 py-3 text-sm shadow-card">
      {/* The phone header already says whether Headscale is up. */}
      <span className="hidden items-center gap-2.5 md:inline-flex">
        <HealthWord health={health} unavailable={unavailable} />
        <span aria-hidden className="text-muted-foreground">·</span>
      </span>
      <span>
        <strong className="font-semibold tabular-nums">{online}</strong>
        <span className="text-muted-foreground"> of {total} online</span>
      </span>
      {needCount > 0 && (
        <>
          <span aria-hidden className="text-muted-foreground">·</span>
          <Link to="/devices#needs" onClick={scrollToNeeds} className="inline-flex min-h-11 items-center font-medium text-warn underline-offset-4 hover:underline md:min-h-0">
            {needCount} need{needCount === 1 ? 's' : ''} you
          </Link>
        </>
      )}
      <span className="ml-auto text-xs text-muted-foreground" title={new Date(updatedAt).toLocaleString()}>
        updated {age < 10 ? 'just now' : age < 60 ? `${age}s ago` : `${Math.round(age / 60)}m ago`}
      </span>
    </section>
  )
}
