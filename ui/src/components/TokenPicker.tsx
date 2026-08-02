import { useMemo, useRef, useState } from 'react'
import { clsx } from 'clsx'
import type { Tokens } from '../lib/api'

/**
 * The token picker is the difference between a form and a textarea.
 *
 * Every suggestion comes from the live tailnet: real usernames, groups defined
 * above in this document, tags that actually have an owner, hosts, plus the
 * autogroups the policy language defines. Anything typed that is not one of
 * those is flagged before save rather than after — which is the failure mode
 * Headplane's raw editor has.
 */

export type Slot = 'src' | 'acl-dst' | 'grant-dst' | 'via' | 'ip'

export interface TokenIssue {
  token: string
  reason: string
  severity: 'blocking' | 'warning'
}

/** validate reports which tokens will not resolve. */
export function validateTokens(values: string[], tokens: Tokens, slot: Slot): TokenIssue[] {
  return values.flatMap((v) => {
    const issue = validateToken(v, tokens, slot)

    return issue ? [{ token: v, ...issue }] : []
  })
}

function validateToken(
  value: string,
  tokens: Tokens,
  slot: Slot,
): Omit<TokenIssue, 'token'> | undefined {
  if (value === '') return { reason: 'empty', severity: 'blocking' }
  if (value === '*') return undefined

  if (slot === 'ip') {
    return isProtocolPortSpec(value)
      ? undefined
      : { reason: 'use *, a port, a range, or tcp:/udp:/sctp: with one', severity: 'blocking' }
  }

  // An ACL destination carries a port suffix; a grant destination does not.
  let base = value

  if (slot === 'acl-dst') {
    const cut = value.lastIndexOf(':')

    if (cut > 0) {
      base = value.slice(0, cut)

      const ports = value.slice(cut + 1)

      if (!isPortSpec(ports)) return { reason: `"${ports}" is not a port, a range, or *`, severity: 'blocking' }
    } else if (!value.startsWith('autogroup:')) {
      return { reason: 'a destination needs a port, e.g. tag:prod:443', severity: 'blocking' }
    }
  }

  if (slot === 'via' && !base.startsWith('tag:')) {
    return { reason: 'a route constraint must be a tag', severity: 'blocking' }
  }

  if (base.startsWith('autogroup:')) {
    // autogroup:self is written as a bare prefix in destinations.
    const known = tokens.autogroups.some((a) => base === a || base.startsWith(a))

    return known ? undefined : { reason: `unknown autogroup "${base}"`, severity: 'blocking' }
  }

  if (base.startsWith('group:')) {
    return tokens.groups.includes(base)
      ? undefined
      : { reason: `no group named "${base}" is defined`, severity: 'warning' }
  }

  if (base.startsWith('tag:')) {
    return tokens.tags.includes(base)
      ? undefined
      : { reason: `"${base}" has no owner in tagOwners`, severity: 'warning' }
  }

  if (base.endsWith('@')) {
    return tokens.users.includes(base)
      ? undefined
      : { reason: `no user named "${base}"`, severity: 'warning' }
  }

  if (tokens.hosts.includes(base)) return undefined

  if (isCidrOrIP(base)) return undefined

  return { reason: `"${base}" is not a known user, group, tag, host or CIDR`, severity: 'warning' }
}

function isPortSpec(s: string): boolean {
  if (s === '*') return true

  return s.split(',').every((part) => {
    const [a, b] = part.split('-')

    return isPort(a) && (b === undefined || isPort(b))
  })
}

function isProtocolPortSpec(s: string): boolean {
  return s.split(',').every((part) => {
    const [, ports = part] = part.match(/^(?:tcp|udp|sctp):(.*)$/) ?? []

    return isPortSpec(ports)
  })
}

function isPort(s: string): boolean {
  const n = Number(s)

  return /^\d+$/.test(s) && n >= 0 && n <= 65535
}

function isCidrOrIP(s: string): boolean {
  const [addr, bits] = s.split('/')

  if (bits !== undefined && !/^\d{1,3}$/.test(bits)) return false

  return /^[0-9.]+$/.test(addr) || /^[0-9a-fA-F:]+$/.test(addr)
}

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
  }, [draft, tokens, values])

  const add = (token: string) => {
    const t = token.trim()

    if (t === '') return

    onChange([...values, t])
    setDraft('')
    setOpen(false)
    inputRef.current?.focus()
  }

  return (
    <div className="relative flex min-h-10 flex-1 flex-wrap items-center gap-1 rounded-lg border border-border bg-surface-1 px-1.5 py-1 shadow-sm transition-colors focus-within:border-accent-500">
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
              className="opacity-60 hover:opacity-100"
              onClick={() => onChange(values.filter((_, j) => j !== i))}
            >
              ×
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
        className="min-w-24 flex-1 bg-transparent px-1 py-0.5 font-mono text-xs outline-none placeholder:text-muted-foreground"
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
