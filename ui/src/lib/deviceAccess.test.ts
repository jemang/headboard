import { describe, expect, it } from 'vitest'
import type { Device, DeviceRules, Peer } from './api'
import { accessibleDevices, ownedDevices } from './deviceAccess'

const device = (overrides: Partial<Device> = {}): Device => ({
  id: 1,
  name: 'device',
  hostname: 'device',
  ips: ['100.64.0.1'],
  online: true,
  expired: false,
  ephemeral: false,
  exitNode: false,
  mine: true,
  ...overrides,
})

const peer = (overrides: Partial<Peer> = {}): Peer => ({
  id: 9,
  givenName: 'nas',
  ips: ['100.64.0.9'],
  online: true,
  ...overrides,
})

const rules = (peers: Peer[]): DeviceRules => ({ inbound: [], outbound: [], peers })

describe('device access', () => {
  it('keeps only devices owned by the signed-in user', () => {
    expect(ownedDevices([device({ id: 1, mine: true }), device({ id: 2, mine: false })])).toEqual([
      device({ id: 1, mine: true }),
    ])
  })

  it('deduplicates a reachable peer and records every owned source device', () => {
    const devices = [device({ id: 1, name: 'laptop', mine: true }), device({ id: 2, name: 'phone', mine: true })]
    const peerRules = new Map([
      [1, rules([peer({ id: 9, givenName: 'nas' })])],
      [2, rules([peer({ id: 9, givenName: 'nas' })])],
    ])

    expect(accessibleDevices(devices, peerRules)).toEqual([
      { id: 9, name: 'nas', ips: ['100.64.0.9'], online: true, sources: ['laptop', 'phone'] },
    ])
  })

  it('does not surface peers from a device that is not mine', () => {
    const devices = [device({ id: 1, name: 'mine', mine: true }), device({ id: 2, name: 'shared', mine: false })]
    const peerRules = new Map([[2, rules([peer({ id: 9 })])]])

    expect(accessibleDevices(devices, peerRules)).toEqual([])
  })

  it('returns an empty access list when owned devices have no peers', () => {
    const devices = [device({ id: 1, mine: true })]
    const peerRules = new Map([[1, rules([])]])

    expect(accessibleDevices(devices, peerRules)).toEqual([])
  })

  it('does not repeat one of the user’s own devices in the access list', () => {
    const devices = [device({ id: 1, name: 'laptop' }), device({ id: 2, name: 'phone' })]
    const peerRules = new Map([[1, rules([peer({ id: 2, givenName: 'phone' })])]])

    expect(accessibleDevices(devices, peerRules)).toEqual([])
  })
})
