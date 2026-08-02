import { describe, expect, it } from 'vitest'
import type { AclSchema, Grant } from './api'
import {
  grantStageOp,
  jumpTarget,
  policyStateNotice,
  simulationRequest,
  testDestinationIssue,
} from './grantEditor'

const grant: Grant = { src: ['tag:agent'], dst: ['13.0.0.0/24'], ip: ['*'] }

describe('policyStateNotice', () => {
  it('distinguishes Headscale allow-all from an explicit deny-all grants array', () => {
    expect(policyStateNotice([], {})).toMatchObject({
      tone: 'info',
      message: 'Headscale default: all devices may communicate.',
    })
    expect(policyStateNotice(['grants'], { grants: [] })).toMatchObject({
      tone: 'warn',
      message: 'Deny all: no traffic is allowed by policy.',
    })
  })

  it('marks legacy ACLs as Raw HuJSON-only', () => {
    const schema: AclSchema = { acls: [{ action: 'accept', src: ['*'], dst: ['*:*'] }] }

    expect(policyStateNotice(['acls'], schema)).toMatchObject({
      tone: 'warn',
      rawPointer: '/acls',
    })
  })
})

describe('grantStageOp', () => {
  it('creates a grants section only when it is absent', () => {
    expect(grantStageOp('new', grant, false)).toEqual({ op: 'add', path: '/grants', value: [grant] })
    expect(grantStageOp('new', grant, true)).toEqual({ op: 'add', path: '/grants/-', value: grant })
  })

  it('appends a second pending grant after the draft creates the section', () => {
    const second = { ...grant, src: ['tag:dev'] }

    expect([
      grantStageOp('new', grant, false),
      grantStageOp('new', second, true),
    ]).toEqual([
      { op: 'add', path: '/grants', value: [grant] },
      { op: 'add', path: '/grants/-', value: second },
    ])
  })

  it('replaces exactly the staged grant index', () => {
    expect(grantStageOp(2, grant, true)).toEqual({ op: 'replace', path: '/grants/2', value: grant })
  })
})

describe('jumpTarget', () => {
  it('opens managed sections and sends legacy ACLs to Raw HuJSON', () => {
    expect(jumpTarget('/grants/0')).toBe('grants')
    expect(jumpTarget('/ssh/0')).toBe('ssh')
    expect(jumpTarget('/acls/0')).toBe('raw')
  })
})

describe('simulationRequest', () => {
  it('builds exactly one destination form and rejects CIDRs before submission', () => {
    expect(simulationRequest('device', 1, 2, '', '22')).toEqual({ src: 1, dst: 2, port: 22 })
    expect(simulationRequest('ip', 1, 0, '13.0.0.25', '443')).toEqual({
      src: 1,
      destinationIP: '13.0.0.25',
      port: 443,
    })
    expect(simulationRequest('ip', 1, 0, '13.0.0.0/24', '443')).toEqual({
      error: 'Enter one IPv4 or IPv6 address, not a CIDR.',
    })
    expect(simulationRequest('device', 0, 2, '', '22')).toEqual({ error: 'Choose a source device.' })
    expect(simulationRequest('device', 1, 0, '', '22')).toEqual({ error: 'Choose a destination device.' })
    expect(simulationRequest('ip', 1, 0, '13.0.0.25', '70000')).toEqual({ error: 'Enter a port from 1 to 65535.' })
  })
})

describe('testDestinationIssue', () => {
  const hosts = { 'backend-lan': '11.0.0.0/24', printer: '11.0.0.9', 'printer32': '11.0.0.9/32' }

  it('blocks a destination covering more than one machine', () => {
    expect(testDestinationIssue('backend-lan:22', hosts)).toBe(
      'backend-lan is 11.0.0.0/24 — a test needs one machine, e.g. 11.0.0.5:22',
    )
    expect(testDestinationIssue('11.0.0.0/24:22', hosts)).toBe(
      '11.0.0.0/24 covers a range of addresses — a test needs one machine, e.g. 11.0.0.5:22',
    )
  })

  it('accepts every form Headscale treats as a single host', () => {
    expect(testDestinationIssue('tag:prod:22', hosts)).toBeUndefined()
    expect(testDestinationIssue('11.0.0.106:22', hosts)).toBeUndefined()
    expect(testDestinationIssue('printer:22', hosts)).toBeUndefined()
    expect(testDestinationIssue('printer32:22', hosts)).toBeUndefined()
    expect(testDestinationIssue('', hosts)).toBeUndefined()
    expect(testDestinationIssue('tag:prod:22', undefined)).toBeUndefined()
  })

  it('rejects a literal prefix even at /32, matching the engine', () => {
    expect(testDestinationIssue('11.0.0.9/32:22', hosts)).toBe(
      '11.0.0.9/32 is written as a prefix — a test destination is a plain address, e.g. 11.0.0.9:22',
    )
  })
})
