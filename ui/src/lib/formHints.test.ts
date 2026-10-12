import { describe, expect, it } from 'vitest'
import { hostEntryProblem, registrationIdHint, unixUserProblem } from './formHints'

describe('registrationIdHint', () => {
  it('says nothing while empty or well formed', () => {
    expect(registrationIdHint('')).toBeNull()
    expect(registrationIdHint('hskey-authreq-abc123def456ghi789jkl012')).toBeNull()
  })

  it('explains an id that is not a registration id', () => {
    expect(registrationIdHint('not-a-real-id')).toMatch(/hskey-authreq-/)
  })

  it('explains an id that is cut short', () => {
    expect(registrationIdHint('hskey-authreq-')).toMatch(/incomplete/)
  })
})

describe('hostEntryProblem', () => {
  it('accepts a name and an IPv4, CIDR or IPv6 address', () => {
    expect(hostEntryProblem('web-1', '192.0.2.10')).toBeNull()
    expect(hostEntryProblem('office-lan', '10.0.0.0/24')).toBeNull()
    expect(hostEntryProblem('v6-host', 'fd7a:115c:a1e0::1')).toBeNull()
  })

  it('says nothing until both fields have something', () => {
    expect(hostEntryProblem('', '')).toBeNull()
    expect(hostEntryProblem('web-1', '')).toBeNull()
  })

  it('rejects a name with spaces or capitals', () => {
    expect(hostEntryProblem('bad host', '10.0.0.1')).toMatch(/name/i)
  })

  it('rejects an address that is not an IP or CIDR', () => {
    expect(hostEntryProblem('web-1', '999.1.1.1/40')).toMatch(/address/i)
    expect(hostEntryProblem('web-1', 'example.com')).toMatch(/address/i)
  })

  it('rejects a name already in the policy', () => {
    expect(hostEntryProblem('web-1', '10.0.0.1', ['web-1'])).toMatch(/already/i)
  })
})

describe('unixUserProblem', () => {
  it('accepts a normal unix user', () => {
    expect(unixUserProblem('deploy', [])).toBeNull()
    expect(unixUserProblem('', [])).toBeNull()
  })

  it('explains an invalid name', () => {
    expect(unixUserProblem('Bad User!', [])).toMatch(/lowercase/i)
  })

  it('explains a duplicate', () => {
    expect(unixUserProblem('deploy', ['deploy'])).toMatch(/already/i)
  })
})
