import type { Device } from './api'

export type DeviceFilter = 'all' | 'online' | 'offline' | 'routes' | 'expired' | 'tagged'
export type DeviceSort = 'name' | 'lastSeen' | 'status'

export const FILTERS: { id: DeviceFilter; label: string }[] = [
  { id: 'all', label: 'All' },
  { id: 'online', label: 'Online' },
  { id: 'offline', label: 'Offline' },
  { id: 'routes', label: 'Routes pending' },
  { id: 'expired', label: 'Expired' },
  { id: 'tagged', label: 'Tagged' },
]

export const SORTS: { id: DeviceSort; label: string }[] = [
  { id: 'name', label: 'Name' },
  { id: 'lastSeen', label: 'Last seen' },
  { id: 'status', label: 'Status' },
]

const pendingRoutes = (d: Device) => (d.advertisedRoutes ?? []).some((r) => !(d.approvedRoutes ?? []).includes(r))

const keep: Record<DeviceFilter, (d: Device) => boolean> = {
  all: () => true,
  online: (d) => d.online,
  offline: (d) => !d.online,
  routes: pendingRoutes,
  expired: (d) => d.expired,
  tagged: (d) => (d.tags ?? []).length > 0,
}

const byName = (a: Device, b: Device) => a.name.localeCompare(b.name)
const rank = (d: Device) => (d.online ? 0 : d.expired ? 1 : 2)
const seen = (d: Device) => (d.online ? Number.POSITIVE_INFINITY : d.lastSeen ? Date.parse(d.lastSeen) : 0)

const order: Record<DeviceSort, (a: Device, b: Device) => number> = {
  name: byName,
  status: (a, b) => rank(a) - rank(b) || byName(a, b),
  // Infinity - Infinity is NaN for two online devices; fall through to name.
  lastSeen: (a, b) => (seen(b) - seen(a)) || byName(a, b),
}

/**
 * The rows the board shows. Name order only moves a row when a name or the
 * set of devices changes; Last seen and Status re-order on live updates, which
 * is why they are opt-in.
 */
export function boardRows(devices: Device[], query: string, filter: DeviceFilter, sort: DeviceSort): Device[] {
  const q = query.trim().toLowerCase()

  return devices
    .filter(keep[filter])
    .filter((d) => q === ''
      || d.name.toLowerCase().includes(q)
      || (d.owner ?? '').toLowerCase().includes(q)
      || (d.tags ?? []).some((t) => t.toLowerCase().includes(q))
      || d.ips.some((ip) => ip.includes(q)))
    .sort(order[sort])
}
