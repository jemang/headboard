import { describe, expect, it } from 'vitest'
import type { Device } from './api'
import { boardRows } from './board'

const d = (over: Partial<Device>): Device => ({
  id: 1, name: 'a', hostname: 'a', ips: ['100.64.0.1'], online: false, expired: false, ephemeral: false, exitNode: false, mine: false, ...over,
})

const devices = [
  d({ id: 1, name: 'web-1', online: true, lastSeen: '2026-10-12T10:00:00Z', tags: ['tag:prod'] }),
  d({ id: 2, name: 'alice-laptop', owner: 'alice@example.com', lastSeen: '2026-10-12T09:00:00Z' }),
  d({ id: 3, name: 'bob-desktop', expired: true, advertisedRoutes: ['10.0.0.0/24'], lastSeen: '2026-10-12T11:00:00Z' }),
]

const names = (rows: Device[]) => rows.map((r) => r.name)

describe('boardRows', () => {
  it('sorts by name by default', () => {
    expect(names(boardRows(devices, '', 'all', 'name'))).toEqual(['alice-laptop', 'bob-desktop', 'web-1'])
  })

  it('filters by chip', () => {
    expect(names(boardRows(devices, '', 'online', 'name'))).toEqual(['web-1'])
    expect(names(boardRows(devices, '', 'offline', 'name'))).toEqual(['alice-laptop', 'bob-desktop'])
    expect(names(boardRows(devices, '', 'routes', 'name'))).toEqual(['bob-desktop'])
    expect(names(boardRows(devices, '', 'expired', 'name'))).toEqual(['bob-desktop'])
    expect(names(boardRows(devices, '', 'tagged', 'name'))).toEqual(['web-1'])
  })

  it('searches name, owner, tag and address', () => {
    expect(names(boardRows(devices, 'ALICE', 'all', 'name'))).toEqual(['alice-laptop'])
    expect(names(boardRows(devices, 'prod', 'all', 'name'))).toEqual(['web-1'])
    expect(names(boardRows(devices, '100.64', 'all', 'name'))).toHaveLength(3)
  })

  it('sorts online first, then expired, then name, for status', () => {
    expect(names(boardRows(devices, '', 'all', 'status'))).toEqual(['web-1', 'bob-desktop', 'alice-laptop'])
  })

  it('sorts online devices as most recent for last seen', () => {
    expect(names(boardRows(devices, '', 'all', 'lastSeen'))).toEqual(['web-1', 'bob-desktop', 'alice-laptop'])
  })

  it('does not mutate its input', () => {
    const copy = [...devices]
    boardRows(devices, '', 'all', 'status')
    expect(devices).toEqual(copy)
  })
})
