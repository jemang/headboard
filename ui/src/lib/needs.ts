import type { Account, Device, Health, SshAccess } from './api'

export type NeedKind = 'headscale' | 'account' | 'route' | 'expired' | 'ssh'

export interface NeedItem {
  id: string
  kind: NeedKind
  label: string
  detail: string
  to?: string
}

export interface NeedsInput {
  capabilities: string[]
  health?: Health
  healthUnavailable?: boolean
  devices?: Device[]
  accounts?: Account[]
  ssh?: SshAccess
}

/**
 * Everything waiting on a person, computed only from data the role may
 * already read. A missing input contributes nothing, so a query that is
 * still loading or failed never reads as "nothing to do" for its kind.
 */
export function deriveNeeds({ capabilities, health, healthUnavailable, devices, accounts, ssh }: NeedsInput): NeedItem[] {
  const can = (c: string) => capabilities.includes(c)
  const items: NeedItem[] = []

  if (healthUnavailable || health?.headscaleState === 'unavailable') {
    items.push({ id: 'headscale:down', kind: 'headscale', label: 'Headscale unavailable', detail: 'Device and policy data may be out of date.' })
  } else if (health?.headscaleState === 'stale') {
    items.push({ id: 'headscale:stale', kind: 'headscale', label: 'Headscale reconnecting', detail: 'Showing the last successful sync.' })
  }
  if (health && !health.headscaleVersionMatch) {
    items.push({
      id: 'headscale:version',
      kind: 'headscale',
      label: 'Headscale version mismatch',
      detail: `Server runs ${health.headscaleServerVersion}; Headboard was built for ${health.headscaleVersion}.`,
    })
  }

  if (can('manage:users')) {
    for (const a of accounts ?? []) {
      if (a.admission !== 'pending') continue
      items.push({ id: `account:${a.id}`, kind: 'account', label: `${a.email} wants access`, detail: 'Approve or reject the account.', to: `/people?account=${a.id}` })
    }
  }

  for (const d of devices ?? []) {
    const waiting = (d.advertisedRoutes ?? []).filter((r) => !(d.approvedRoutes ?? []).includes(r))
    if (can('manage:devices') && waiting.length > 0) {
      items.push({
        id: `route:${d.id}`,
        kind: 'route',
        label: `${d.name} advertises ${waiting.length === 1 ? 'a route' : `${waiting.length} routes`}`,
        detail: waiting.join(', '),
        to: `/devices?device=${d.id}&section=routing`,
      })
    }
    if (can('view:all') && d.expired) {
      items.push({ id: `expired:${d.id}`, kind: 'expired', label: `${d.name} key expired`, detail: 'The device must re-authenticate.', to: `/devices?device=${d.id}` })
    }
  }

  if (can('manage:ssh')) {
    for (const s of ssh?.servers ?? []) {
      if (s.binding?.state !== 'suspended') continue
      items.push({ id: `ssh:${s.name}`, kind: 'ssh', label: `${s.name} is suspended`, detail: 'Re-approve the binding or unbind it.', to: `/ssh?server=${encodeURIComponent(s.name)}` })
    }
  }

  return items
}
