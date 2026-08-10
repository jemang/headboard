import { describe, expect, it } from 'vitest'
import { operatorCommand } from './operatorCommand'

describe('operatorCommand', () => {
  it.each([
    ['advertise-subnet', '10.0.0.0/24,192.168.1.0/24', "tailscale set --advertise-routes='10.0.0.0/24,192.168.1.0/24'"],
    ['accept-routes', '', 'tailscale set --accept-routes'],
    ['advertise-exit', '', 'tailscale set --advertise-exit-node'],
    ['use-exit', 'gateway', "tailscale set --exit-node='gateway'"],
    ['enable-ssh', '', 'tailscale set --ssh'],
    ['status', '', 'tailscale status'],
    ['netcheck', '', 'tailscale netcheck'],
    ['ping', 'db-1', "tailscale ping 'db-1'"],
  ] as const)('%s builds the intended command', (kind, value, expected) => {
    expect(operatorCommand(kind, value)).toBe(expected)
  })

  it('withholds commands requiring a missing value and safely quotes supplied values', () => {
    expect(operatorCommand('use-exit', '   ')).toBe('')
    expect(operatorCommand('ping', "node'; rm -rf /")).toBe("tailscale ping 'node'\"'\"'; rm -rf /'")
  })
})
