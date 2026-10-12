import { describe, expect, it } from 'vitest'
import type { Device } from './api'
import { snapshot, statusChanges } from './statusFlip'

const d = (id: number, online: boolean, expired = false) =>
  ({ id, name: `d${id}`, hostname: `d${id}`, ips: [], online, expired, ephemeral: false, exitNode: false, mine: false }) as Device

describe('statusChanges', () => {
  it('reports nothing on first load', () => {
    expect(statusChanges(null, snapshot([d(1, true)]))).toEqual(new Map())
  })

  it('names which status changed, keeping connectivity and expiry apart', () => {
    const prev = snapshot([d(1, true), d(2, true), d(3, false)])
    const next = snapshot([d(1, false), d(2, true, true), d(3, false)])
    expect(statusChanges(prev, next)).toEqual(new Map([[1, 'online'], [2, 'expired']]))
  })

  it('ignores new and removed devices', () => {
    expect(statusChanges(snapshot([d(1, true)]), snapshot([d(2, false)]))).toEqual(new Map())
  })
})
