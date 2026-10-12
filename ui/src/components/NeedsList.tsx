import { ChevronRight } from 'lucide-react'
import type { NeedItem } from '../lib/needs'
import { Link } from '../lib/router'

/** Everything waiting on a person. Renders nothing at all when nothing waits. */
export function NeedsList({ items }: { items: NeedItem[] }) {
  if (items.length === 0) return null

  return (
    <section id="needs" aria-label="Needs you" className="scroll-mt-24 overflow-hidden rounded-xl border border-attention/35 bg-attention/5">
      <h2 className="px-4 pb-1 pt-3 text-sm font-semibold text-warn">Waiting on you</h2>
      <ul>
        {items.map((i) => {
          const body = (
            <>
              <span aria-hidden className="mt-1.5 size-2 shrink-0 rounded-full bg-attention" />
              <span className="min-w-0 flex-1">
                <span className="block break-words font-medium">{i.label}</span>
                <span className="block truncate text-sm text-muted-foreground">{i.detail}</span>
              </span>
            </>
          )

          return (
            <li key={i.id} className="border-t border-attention/20">
              {i.to ? (
                <Link to={i.to} className="flex min-h-14 items-start gap-3 px-4 py-2.5 transition-colors hover:bg-attention/10">
                  {body}
                  <ChevronRight aria-hidden className="mt-0.5 size-4 shrink-0 self-center text-muted-foreground" strokeWidth={1.5} />
                </Link>
              ) : (
                <div className="flex items-start gap-3 px-4 py-2.5">{body}</div>
              )}
            </li>
          )
        })}
      </ul>
    </section>
  )
}
