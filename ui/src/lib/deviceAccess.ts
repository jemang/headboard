import type { Device, DeviceRules } from './api'

export interface AccessibleDevice {
  id: number
  name: string
  owner?: string
  ips: string[]
  online: boolean
  sources: string[]
}

export function ownedDevices(devices: Device[]) {
  return devices.filter((device) => device.mine)
}

/** Combines Headscale's per-source peer lists without pretending all sources
 * have the same policy access. */
export function accessibleDevices(devices: Device[], rules: Map<number, DeviceRules>) {
  const peers = new Map<number, AccessibleDevice>()
  const mine = ownedDevices(devices)
  const ownedIDs = new Set(mine.map((device) => device.id))

  for (const source of mine) {
    for (const peer of rules.get(source.id)?.peers ?? []) {
      if (ownedIDs.has(peer.id)) continue

      const current = peers.get(peer.id)
      if (current) {
        current.sources.push(source.name)
        continue
      }

      peers.set(peer.id, {
        id: peer.id,
        name: peer.givenName,
        ...(peer.owner ? { owner: peer.owner } : {}),
        ips: peer.ips,
        online: peer.online,
        sources: [source.name],
      })
    }
  }

  return [...peers.values()].sort((a, b) => a.name.localeCompare(b.name))
}
