import { useState } from 'react'
import { useInfiniteQuery } from '@tanstack/react-query'
import { History, KeyRound, KeySquare, Laptop, LogIn, ShieldCheck, Users, X } from 'lucide-react'
import { api, type AuditCategory, type AuditEntry } from '../lib/api'
import { changedFields, dayLabel, describeEntry, type Described } from '../lib/activity'
import { cn } from '../lib/cn'
import { Button, Empty, ErrorNote } from '../components/ui'

const CHIPS: { id: AuditCategory; label: string }[] = [
  { id: '', label: 'All' },
  { id: 'devices', label: 'Devices' },
  { id: 'policy', label: 'Policy' },
  { id: 'people', label: 'People' },
  { id: 'keys', label: 'Keys' },
  { id: 'ssh', label: 'SSH' },
  { id: 'signin', label: 'Sign-ins' },
]

const ICON: Record<Described['category'], typeof Laptop> = {
  devices: Laptop,
  policy: ShieldCheck,
  people: Users,
  keys: KeyRound,
  ssh: KeySquare,
  signin: LogIn,
}

const TONE: Record<Described['tone'], string> = {
  default: 'bg-accent-500/12 text-accent-700 dark:text-accent-400',
  warn: 'bg-attention/12 text-warn',
  danger: 'bg-danger/10 text-danger',
}

/** The /activity route: every change made through Headboard, newest first. */
export function Activity() {
  const [category, setCategory] = useState<AuditCategory>('')
  const [actor, setActor] = useState('')
  const log = useInfiniteQuery({
    queryKey: ['audit', category, actor],
    queryFn: ({ pageParam }) => api.audit({ category, actor, before: pageParam || undefined }),
    initialPageParam: 0,
    getNextPageParam: (last) => last.next || undefined,
    refetchInterval: 30_000,
  })
  const entries = log.data?.pages.flatMap((p) => p.entries) ?? []

  return (
    <div className="space-y-6">
      <header>
        <h1 className="text-display font-semibold lg:text-page">Activity</h1>
        <p className="max-w-3xl text-sm text-muted-foreground">
          Every change made through Headboard, and every sign-in, newest first. Headscale keeps no history of its own, so
          changes made with the headscale command line do not appear here.
        </p>
      </header>

      <div className="flex flex-wrap items-center gap-2">
        <div role="radiogroup" aria-label="Show" className="-mx-4 flex gap-2 overflow-x-auto px-4 [scrollbar-width:none] md:mx-0 md:flex-wrap md:overflow-visible md:px-0 [&::-webkit-scrollbar]:hidden">
          {CHIPS.map((c) => (
            <button
              key={c.id || 'all'}
              type="button"
              role="radio"
              aria-checked={category === c.id}
              onClick={() => setCategory(c.id)}
              className={cn(
                'h-10 shrink-0 whitespace-nowrap rounded-full border px-3 text-sm transition-colors md:h-9',
                category === c.id
                  ? 'border-accent-500/50 bg-accent-500/15 font-medium text-accent-700 dark:text-accent-400'
                  : 'border-border text-muted-foreground hover:text-foreground',
              )}
            >
              {c.label}
            </button>
          ))}
        </div>
        {actor && (
          <button
            type="button"
            onClick={() => setActor('')}
            aria-label={`Show everyone, not only ${actor}`}
            className="inline-flex h-10 max-w-full items-center gap-1.5 rounded-full border border-accent-500/50 bg-accent-500/15 pl-3 pr-2 text-sm font-medium text-accent-700 md:h-9 dark:text-accent-400"
          >
            <span className="truncate">by {actor}</span>
            <X aria-hidden className="size-4 shrink-0" strokeWidth={1.75} />
          </button>
        )}
      </div>

      {log.error && !log.data ? (
        <ErrorNote error={log.error} />
      ) : log.isPending ? (
        <p className="text-sm text-muted-foreground">Loading…</p>
      ) : entries.length === 0 ? (
        <Empty
          icon={History}
          title={category || actor ? 'Nothing matches' : 'No activity yet'}
          hint={category || actor ? 'Try another category, or show everyone.' : 'Changes and sign-ins appear here as they happen.'}
        />
      ) : (
        <ActivityList
          entries={entries}
          hasMore={log.hasNextPage}
          loadingMore={log.isFetchingNextPage}
          onMore={() => void log.fetchNextPage()}
          onActor={setActor}
        />
      )}
    </div>
  )
}

export function ActivityList({
  entries,
  hasMore,
  loadingMore,
  onMore,
  onActor,
  now,
}: {
  entries: AuditEntry[]
  hasMore: boolean
  loadingMore: boolean
  onMore: () => void
  onActor: (label: string) => void
  now?: Date
}) {
  const days: { label: string; entries: AuditEntry[] }[] = []
  for (const e of entries) {
    const label = dayLabel(e.createdAt, now)
    const last = days[days.length - 1]
    if (last?.label === label) last.entries.push(e)
    else days.push({ label, entries: [e] })
  }

  return (
    <div className="space-y-6">
      {days.map((day) => (
        <section key={day.label} aria-label={day.label} className="space-y-2">
          <h2 className="text-eyebrow font-semibold uppercase text-muted-foreground">{day.label}</h2>
          <ul className="divide-y divide-border overflow-hidden rounded-xl border border-border bg-surface-1 shadow-card">
            {day.entries.map((e) => (
              <Row key={e.id} entry={e} onActor={onActor} />
            ))}
          </ul>
        </section>
      ))}
      {hasMore && (
        <div className="flex justify-center">
          <Button onClick={onMore} disabled={loadingMore}>{loadingMore ? 'Loading…' : 'Load more'}</Button>
        </div>
      )}
    </div>
  )
}

function Row({ entry, onActor }: { entry: AuditEntry; onActor: (label: string) => void }) {
  const [open, setOpen] = useState(false)
  const d = describeEntry(entry)
  const Icon = ICON[d.category]
  const at = new Date(entry.createdAt)
  // A masked or empty label names nobody, so there is no one to filter by.
  const filterable = entry.actorLabel !== '' && !entry.actorLabel.includes('***')
  const hasChange = entry.before !== undefined || entry.after !== undefined
  const changes = changedFields(entry.before, entry.after)

  return (
    <li>
      <button
        type="button"
        aria-expanded={open}
        onClick={() => setOpen(!open)}
        className="flex w-full items-start gap-3 px-4 py-3 text-left transition-colors hover:bg-surface-2/60"
      >
        <span aria-hidden className={cn('mt-0.5 grid size-8 shrink-0 place-items-center rounded-full', TONE[d.tone])}>
          <Icon className="size-4" strokeWidth={1.6} />
        </span>
        <span className="min-w-0 flex-1">
          <span className="block break-words text-sm">
            <span className="font-medium">{d.actor}</span> {d.text}
          </span>
          {d.detail && <span className="mt-0.5 block break-words text-xs text-muted-foreground">{d.detail}</span>}
        </span>
        <span className="shrink-0 text-right text-xs text-muted-foreground">
          <time dateTime={entry.createdAt} title={at.toLocaleString()}>
            {at.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' })}
          </time>
          {entry.ip && <span className="mt-0.5 block font-mono">{entry.ip}</span>}
        </span>
      </button>
      {open && (
        <div className="space-y-3 border-t border-border bg-surface-2/40 px-4 py-3 text-xs">
          <dl className="grid gap-x-4 gap-y-1 sm:grid-cols-[8rem_1fr]">
            <dt className="text-muted-foreground">When</dt>
            <dd>{at.toLocaleString()}</dd>
            <dt className="text-muted-foreground">Action</dt>
            <dd className="font-mono">{entry.action}</dd>
            {(entry.targetLabel || entry.targetId) && (
              <>
                <dt className="text-muted-foreground">Target</dt>
                <dd className="break-all">
                  {entry.targetLabel || entry.targetType}
                  {entry.targetId && entry.targetId !== '0' && <span className="font-mono text-muted-foreground"> · {entry.targetType} #{entry.targetId}</span>}
                </dd>
              </>
            )}
          </dl>
          {changes.length > 0 && (
            <div>
              <p className="mb-1 text-muted-foreground">What changed</p>
              <ul className="space-y-1">
                {changes.map((c) => (
                  <li key={c.key} className="break-words">
                    <span className="font-mono text-muted-foreground">{c.key}</span>{' '}
                    <span className="line-through decoration-danger/60">{c.from || 'none'}</span> → <span className="font-medium">{c.to || 'none'}</span>
                  </li>
                ))}
              </ul>
            </div>
          )}
          {hasChange && (changes.length > 0 ? (
            <details>
              <summary className="cursor-pointer text-muted-foreground hover:text-foreground">Full record</summary>
              <div className="mt-2 grid gap-3 md:grid-cols-2">
                <Snapshot label="Before" value={entry.before} />
                <Snapshot label="After" value={entry.after} />
              </div>
            </details>
          ) : (
            <div className="grid gap-3 md:grid-cols-2">
              <Snapshot label="Before" value={entry.before} />
              <Snapshot label="After" value={entry.after} />
            </div>
          ))}
          {filterable && (
            <Button variant="ghost" onClick={() => onActor(entry.actorLabel)}>Show only {d.actor}</Button>
          )}
        </div>
      )}
    </li>
  )
}

function Snapshot({ label, value }: { label: string; value: unknown }) {
  return (
    <div className="min-w-0">
      <p className="mb-1 text-muted-foreground">{label}</p>
      <pre className="max-h-64 overflow-auto rounded-lg border border-border bg-surface-1 p-2 font-mono text-[0.6875rem] leading-relaxed">
        {value === undefined ? '—' : JSON.stringify(value, null, 2)}
      </pre>
    </div>
  )
}
