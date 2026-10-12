import type { Health } from '../../lib/api'
import { connectionPresentation } from '../../lib/headscaleConnection'
import { cn } from '../../lib/cn'

const TONE = {
  connected: 'bg-ok',
  stale: 'bg-warn',
  unavailable: 'bg-danger',
  checking: 'bg-muted-foreground/40',
}

/** Headscale's state as a dot plus words; `compact` keeps one word for the phone header. */
export function HealthWord({ health, unavailable, compact, onRail, onSide }: { health?: Health; unavailable: boolean; compact?: boolean; onRail?: boolean; onSide?: boolean }) {
  const p = connectionPresentation(unavailable ? { headscaleState: 'unavailable' } : health)
  const word = { connected: 'connected', stale: 'reconnecting', unavailable: 'unavailable', checking: 'checking' }[p.tone]

  return (
    <span className="inline-flex items-center gap-1.5" title={p.title}>
      <span aria-hidden className={cn('size-2 shrink-0 rounded-full', TONE[p.tone])} />
      <span className={cn(onRail ? 'text-rail-muted' : onSide ? 'font-medium text-side-fg' : 'text-muted-foreground', compact ? 'text-xs' : 'text-sm')}>{compact ? word : p.label}</span>
    </span>
  )
}
