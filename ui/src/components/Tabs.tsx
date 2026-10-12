import { useEffect, useRef, useState } from 'react'
import { cn } from '../lib/cn'

/** Scrolls sideways when it does not fit, and keeps the active tab in view. */
export function Tabs<T extends string>({
  items,
  value,
  onChange,
  label,
}: {
  items: readonly { id: T; label: string }[]
  value: T
  onChange: (id: T) => void
  label: string
}) {
  const active = useRef<HTMLButtonElement>(null)
  const strip = useRef<HTMLDivElement>(null)
  // Fade the right edge only while more tabs sit past it.
  const [more, setMore] = useState(false)

  useEffect(() => {
    const el = strip.current
    if (!el) return

    const measure = () => setMore(el.scrollLeft + el.clientWidth < el.scrollWidth - 1)
    measure()
    el.addEventListener('scroll', measure, { passive: true })
    const ro = new ResizeObserver(measure)
    ro.observe(el)

    return () => {
      el.removeEventListener('scroll', measure)
      ro.disconnect()
    }
  }, [])

  useEffect(() => {
    active.current?.scrollIntoView({ block: 'nearest', inline: 'nearest' })
  }, [value])

  return (
    <div
      ref={strip}
      role="tablist"
      aria-label={label}
      className={cn(
        'flex gap-1 overflow-x-auto border-b border-border [scrollbar-width:none] [&::-webkit-scrollbar]:hidden',
        more && '[mask-image:linear-gradient(to_right,black_calc(100%-1.5rem),transparent)]',
      )}
    >
      {items.map((t) => (
        <button
          key={t.id}
          ref={t.id === value ? active : undefined}
          type="button"
          role="tab"
          aria-selected={t.id === value}
          onClick={() => onChange(t.id)}
          className={cn(
            'min-h-11 shrink-0 whitespace-nowrap border-b-2 px-3 text-sm transition-colors md:min-h-0 md:py-2',
            t.id === value ? 'border-accent-500 font-semibold text-foreground' : 'border-transparent text-muted-foreground hover:text-foreground',
          )}
        >
          {t.label}
        </button>
      ))}
    </div>
  )
}
