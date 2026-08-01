import { describe, expect, it } from 'vitest'
import type { Device } from './api'
import { devicePulse } from './devicePulse'

const device = (overrides: Partial<Device>): Device => ({
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

describe('devicePulse', () => {
  it('summarizes the complete device list independently from table filtering', () => {
    expect(
      devicePulse([
        device({ id: 1, online: true }),
        device({ id: 2, online: false }),
        device({ id: 3, online: false, expired: true }),
      ]),
    ).toEqual({ total: 3, online: 1, offline: 2, expired: 1 })
  })
})
