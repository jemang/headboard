import type { Tokens } from './api'

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

export function validateToken(
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
