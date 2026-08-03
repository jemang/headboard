import { describe, expect, it } from 'vitest'
import type { Device } from './api'
import { devicesForOwner } from './ownerDevices'

const device = (overrides: Partial<Device> = {}): Device => ({
  id: 1,
  name: 'device',
  hostname: 'device',
  ips: ['100.64.0.1'],
  online: true,
  expired: false,
  ephemeral: false,
  exitNode: false,
  mine: false,
  ...overrides,
})

describe('devicesForOwner', () => {
  it('returns only devices with the selected Headscale owner ID', () => {
    const devices = [device({ id: 1, ownerId: 10 }), device({ id: 2, ownerId: 11 })]

    expect(devicesForOwner(devices, 10)).toEqual([device({ id: 1, ownerId: 10 })])
  })
})
