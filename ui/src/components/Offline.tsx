import { useEffect, useRef } from 'react'
import { createPortal } from 'react-dom'
import { WifiOff } from 'lucide-react'
import { Button } from './ui'

/**
 * Nothing from the tailnet is shown offline, so nobody acts on stale access
 * data. As an `overlay` the signed-in app stays mounted underneath (staged
 * policy edits, an open sheet and typed notes survive) but is covered and made
 * inert, portals included.
 */
export function Offline({ onRetry, overlay = false }: { onRetry: () => void; overlay?: boolean }) {
  const self = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!overlay) return

    const others = [...document.body.children].filter((el): el is HTMLElement => el instanceof HTMLElement && el !== self.current)
    const was = others.map((el) => el.inert)
    others.forEach((el) => {
      el.inert = true
    })
    self.current?.querySelector('button')?.focus()

    return () => others.forEach((el, i) => {
      el.inert = was[i]
    })
  }, [overlay])

  const screen = (
    <div ref={self} className={overlay ? 'fixed inset-0 z-[70] bg-surface-0' : undefined}>
      <main className="grid min-h-dvh place-items-center bg-surface-0 p-6">
        <section role="alert" className="w-full max-w-md rounded-xl border border-border bg-surface-1 p-6">
          <h1 className="flex items-center gap-2 text-display font-semibold">
            <WifiOff aria-hidden className="size-5 text-muted-foreground" strokeWidth={1.5} />
            You're offline
          </h1>
          <p className="mt-3 text-sm text-muted-foreground">
            Headboard needs a connection to manage the tailnet. Nothing is shown while offline, so you never act on
            old data. Anything you were editing is kept.
          </p>
          <Button variant="primary" onClick={onRetry} className="mt-5">Retry</Button>
        </section>
      </main>
    </div>
  )

  return overlay ? createPortal(screen, document.body) : screen
}
