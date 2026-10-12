import { useEffect, useRef, useState } from 'react'
import type { Device } from './api'

export type StatusSnap = Map<number, { online: boolean; expired: boolean }>
export type FlipField = 'online' | 'expired'

export function snapshot(devices: Device[]): StatusSnap {
  return new Map(devices.map((d) => [d.id, { online: d.online, expired: d.expired }]))
}

/**
 * Which status changed per device between two responses; new and removed ids
 * never count. Connectivity wins when both changed, since it is the headline.
 */
export function statusChanges(prev: StatusSnap | null, next: StatusSnap): Map<number, FlipField> {
  const changed = new Map<number, FlipField>()
  if (!prev) return changed

  for (const [id, now] of next) {
    const was = prev.get(id)
    if (!was) continue
    if (was.online !== now.online) changed.set(id, 'online')
    else if (was.expired !== now.expired) changed.set(id, 'expired')
  }

  return changed
}

/**
 * Compares consecutive unfiltered device responses, so a filter or sort change
 * never animates. Resets when the signed-in account changes. Each change gets
 * a fresh key, so a second flip of the same device replays.
 */
export function useStatusFlips(devices: Device[] | undefined, principal: number): Map<number, { field: FlipField; key: number }> {
  const prev = useRef<{ principal: number; snap: StatusSnap } | null>(null)
  const counter = useRef(0)
  const [flips, setFlips] = useState<Map<number, { field: FlipField; key: number }>>(() => new Map())

  useEffect(() => {
    if (!devices) return

    const next = snapshot(devices)
    const base = prev.current?.principal === principal ? prev.current.snap : null
    prev.current = { principal, snap: next }

    const changed = statusChanges(base, next)
    if (changed.size === 0) return

    counter.current += 1
    const key = counter.current
    setFlips(new Map([...changed].map(([id, field]) => [id, { field, key }])))
  }, [devices, principal])

  // Cleared on its own timer, so a later refetch with no changes cannot cancel it.
  useEffect(() => {
    if (flips.size === 0) return

    const t = setTimeout(() => setFlips(new Map()), 1000)

    return () => clearTimeout(t)
  }, [flips])

  return flips
}
