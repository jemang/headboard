import { X } from 'lucide-react'
import { useMemo, useRef, useState } from 'react'
import { clsx } from 'clsx'
import type { Tokens } from '../lib/api'
import { validateToken, type Slot } from '../lib/tokenValidation'

/**
 * The token picker is the difference between a form and a textarea.
 *
 * Every suggestion comes from the live tailnet: real usernames, groups defined
 * above in this document, tags that actually have an owner, hosts, plus the
 * autogroups the policy language defines. Anything typed that is not one of
 * those is flagged before save rather than after — which is the failure mode
 * raw editor has.
 */

export function TokenPicker({
  values,
  onChange,
  tokens,
  slot,
  placeholder,
}: {
  values: string[]
  onChange: (next: string[]) => void
  tokens: Tokens
  slot: Slot
  placeholder?: string
}) {
  const [draft, setDraft] = useState('')
  const [open, setOpen] = useState(false)
  const inputRef = useRef<HTMLInputElement>(null)

  const suggestions = useMemo(() => {
    if (slot === 'ip') return []

    const all = slot === 'via'
      ? tokens.tags
      : [
          ...tokens.users,
          ...tokens.groups,
          ...tokens.tags,
          ...tokens.hosts,
          ...tokens.autogroups,
        ]

    const q = draft.toLowerCase()

    return all.filter((t) => t.toLowerCase().includes(q) && !values.includes(t)).slice(0, 8)
  }, [draft, slot, tokens, values])

  const add = (token: string) => {
    const t = token.trim()

    if (t === '') return

    onChange([...values, t])
    setDraft('')
    setOpen(false)
    inputRef.current?.focus()
  }

  return (
    <div className="relative flex min-h-10 flex-1 flex-wrap md:min-h-9 items-center gap-1 rounded-lg border border-border bg-surface-1 px-1.5 py-1 transition-colors focus-within:border-accent-500">
      {values.map((v, i) => {
        const issue = validateToken(v, tokens, slot)

        return (
          <span
            key={`${v}-${i}`}
            title={issue?.reason}
            className={clsx(
              'inline-flex items-center gap-1 rounded-md px-1.5 py-0.5 font-mono text-xs',
              issue
                ? 'bg-danger/15 text-danger ring-1 ring-danger/40'
                : 'bg-surface-2 text-foreground',
            )}
          >
            {issue && <span aria-hidden>⚠</span>}
            {v}
            <button
              type="button"
              aria-label={`Remove ${v}`}
              className="relative opacity-60 hover:opacity-100 after:absolute after:-inset-4 md:after:hidden"
              onClick={() => onChange(values.filter((_, j) => j !== i))}
            >
              <X aria-hidden className="size-3" strokeWidth={2} />
            </button>
          </span>
        )
      })}

      <input
        ref={inputRef}
        value={draft}
        placeholder={values.length === 0 ? placeholder : ''}
        onChange={(e) => {
          setDraft(e.target.value)
          setOpen(true)
        }}
        onFocus={() => setOpen(true)}
        onBlur={() => setTimeout(() => setOpen(false), 120)}
        onKeyDown={(e) => {
          if (e.key === 'Enter' || e.key === ',') {
            e.preventDefault()
            add(suggestions.length === 1 && draft !== '' ? suggestions[0] : draft)
          }

          if (e.key === 'Backspace' && draft === '' && values.length > 0) {
            onChange(values.slice(0, -1))
          }
        }}
        className="bare min-w-24 flex-1 bg-transparent px-1 py-0.5 font-mono text-xs outline-none placeholder:text-muted-foreground"
      />

      {open && suggestions.length > 0 && (
        <ul className="absolute left-0 top-full z-20 mt-1 max-h-56 w-72 overflow-y-auto rounded-lg border border-border bg-surface-0 py-1 shadow-raised">
          {suggestions.map((s) => (
            <li key={s}>
              <button
                type="button"
                className="block w-full px-2.5 py-1 text-left font-mono text-xs hover:bg-surface-2"
                onMouseDown={(e) => {
                  e.preventDefault()
                  add(slot === 'acl-dst' && !s.startsWith('autogroup:') ? `${s}:` : s)
                }}
              >
                {s}
                <span className="ml-2 text-muted-foreground">{kindOf(s, tokens)}</span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

function kindOf(token: string, tokens: Tokens): string {
  if (tokens.users.includes(token)) return 'user'
  if (tokens.groups.includes(token)) return 'group'
  if (tokens.tags.includes(token)) return 'tag'
  if (tokens.hosts.includes(token)) return 'host'

  return 'autogroup'
}
