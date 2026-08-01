import { describe, expect, it } from 'vitest'
import { serverRegistrationCommand } from './serverRegistrationCommand'

describe('serverRegistrationCommand', () => {
  it('builds pending-registration commands for every server type', () => {
    expect(serverRegistrationCommand('https://hs.example', 'standard', '')).toBe(
      "sudo tailscale up --login-server 'https://hs.example'",
    )
    expect(serverRegistrationCommand('https://hs.example', 'tagged', 'tag:server,tag:prod')).toBe(
      "sudo tailscale up --login-server 'https://hs.example' --advertise-tags 'tag:server,tag:prod'",
    )
    expect(serverRegistrationCommand('https://hs.example', 'subnet', '10.0.0.0/24')).toBe(
      "sudo tailscale up --login-server 'https://hs.example' --advertise-routes '10.0.0.0/24'",
    )
    expect(serverRegistrationCommand('https://hs.example', 'exit', '')).toBe(
      "sudo tailscale up --login-server 'https://hs.example' --advertise-exit-node",
    )
  })

  it('quotes operator input as a single shell argument', () => {
    expect(serverRegistrationCommand("https://hs.example/a'b", 'standard', '')).toContain(
      "'https://hs.example/a'\"'\"'b'",
    )
  })
})
