import { describe, expect, it } from 'vitest'
import type { Account, Device, Health, SshAccess } from './api'
import { deriveNeeds } from './needs'

const all = ['view:all', 'manage:devices', 'manage:users', 'manage:ssh']
const health = { status: 'ok', version: 'v', headscaleVersion: '0.29.3', headscaleServerVersion: '0.29.3', headscaleVersionMatch: true, headscaleState: 'connected' } as Health

const device = (over: Partial<Device>): Device => ({
  id: 1, name: 'web-1', hostname: 'web-1', ips: ['100.64.0.1'], online: true, expired: false, ephemeral: false,
  exitNode: false, mine: false, ...over,
})

const account = (over: Partial<Account>): Account => ({
  id: 7, role: 'member', admission: 'pending', email: 'bob@example.com', displayName: 'bob', createdAt: '2026-10-12T00:00:00Z', ...over,
} as Account)

describe('deriveNeeds', () => {
  it('is empty when nothing waits', () => {
    expect(deriveNeeds({ capabilities: all, health, devices: [device({})], accounts: [account({ admission: 'active' })] })).toEqual([])
  })

  it('lists a pending account with a hand-off link', () => {
    const [item] = deriveNeeds({ capabilities: all, health, accounts: [account({})] })
    expect(item).toMatchObject({ id: 'account:7', kind: 'account', to: '/people?account=7' })
    expect(item.label).toContain('bob@example.com')
  })

  it('drops an account once it is approved', () => {
    expect(deriveNeeds({ capabilities: all, health, accounts: [account({ admission: 'active' })] })).toEqual([])
  })

  it('lists routes advertised but not approved', () => {
    const [item] = deriveNeeds({ capabilities: all, health, devices: [device({ id: 4, advertisedRoutes: ['10.0.0.0/24', '0.0.0.0/0'], approvedRoutes: ['0.0.0.0/0'] })] })
    expect(item).toMatchObject({ id: 'route:4', kind: 'route', to: '/devices?device=4&section=routing' })
    expect(item.detail).toBe('10.0.0.0/24')
  })

  it('lists expired keys', () => {
    expect(deriveNeeds({ capabilities: all, health, devices: [device({ id: 5, expired: true })] })[0]).toMatchObject({ id: 'expired:5', to: '/devices?device=5' })
  })

  it('lists suspended SSH bindings', () => {
    const ssh = { servers: [{ name: 'web-1', ip: '192.0.2.10', binding: { state: 'suspended' } }] } as unknown as SshAccess
    expect(deriveNeeds({ capabilities: all, health, ssh })[0]).toMatchObject({ id: 'ssh:web-1', kind: 'ssh', to: '/ssh?server=web-1' })
  })

  it('reports Headscale problems first', () => {
    const items = deriveNeeds({ capabilities: all, healthUnavailable: true, accounts: [account({})] })
    expect(items.map((i) => i.kind)).toEqual(['headscale', 'account'])
  })

  it('reports a version mismatch', () => {
    const items = deriveNeeds({ capabilities: [], health: { ...health, headscaleVersionMatch: false, headscaleServerVersion: '0.30.0' } })
    expect(items[0]).toMatchObject({ kind: 'headscale', label: 'Headscale version mismatch' })
  })

  it('hides items a role cannot act on', () => {
    const items = deriveNeeds({
      capabilities: ['view:all'],
      health,
      devices: [device({ id: 4, advertisedRoutes: ['10.0.0.0/24'] })],
      accounts: [account({})],
    })
    expect(items).toEqual([])
  })
})
