import { describe, expect, it } from 'vitest'
import { changedFields, describeEntry } from './activity'
import type { AuditEntry } from './api'

const e = (over: Partial<AuditEntry>): AuditEntry => ({ id: 1, actorLabel: 'ops@example.com', action: 'x', targetType: 'x', createdAt: '2026-10-12T10:00:00Z', ...over })

describe('describeEntry', () => {
  it('names a renamed device by its old and new name', () => {
    const d = describeEntry(e({ action: 'device.rename', targetLabel: 'web-1', before: { name: 'web-1' }, after: { name: 'web-2' } }))
    expect(d.text).toBe('renamed device web-1 to web-2')
    expect(d.category).toBe('devices')
  })

  it('shows what a tag change did', () => {
    const d = describeEntry(e({ action: 'device.tags', targetLabel: 'web-1', before: ['tag:a'], after: ['tag:a', 'tag:b'] }))
    expect(d.text).toBe('changed tags on web-1')
    expect(d.detail).toBe('tag:a → tag:a, tag:b')
  })

  it('says who could not sign in, and why, without an account', () => {
    const d = describeEntry(e({ action: 'auth.login_failed', actorLabel: '', after: { method: 'password', reason: 'bad credentials' } }))
    expect(d.actor).toBe('Someone')
    expect(d.text).toBe('failed to sign in')
    expect(d.detail).toBe('bad credentials')
    expect(d.tone).toBe('warn')
  })

  it('reads a role change as from → to', () => {
    const d = describeEntry(e({ action: 'account.role', targetLabel: 'a@example.com', before: { role: 'member' }, after: { role: 'admin' } }))
    expect(d.text).toBe('changed the role of a@example.com')
    expect(d.detail).toBe('member → admin')
  })

  it('counts the lines a policy save changed and keeps the note', () => {
    const d = describeEntry(e({ action: 'policy.save', after: { added: ['a', 'b'], removed: ['c'], note: 'open 443' } }))
    expect(d.text).toBe('saved the access policy')
    expect(d.detail).toBe('+2 −1 lines · open 443')
  })

  it('names an agent as the actor of what it did on its own', () => {
    const d = describeEntry(e({ action: 'ssh_target.suspend', actorLabel: 'agent:pve1', targetLabel: 'pve1', after: { servers: ['app-1'] } }))
    expect(d.actor).toBe('Agent pve1')
    expect(d.text).toBe('suspended app-1')
  })

  it('falls back to the raw action and the target id for anything unknown', () => {
    const d = describeEntry(e({ action: 'thing.zap', targetType: 'thing', targetId: '9' }))
    expect(d.text).toBe('thing.zap thing #9')
  })
})

describe('describeEntry on rows written before labels existed', () => {
  it('names the server from the stored host instead of "ssh_host #0"', () => {
    const d = describeEntry(e({ action: 'ssh_access.set', targetType: 'ssh_host', targetId: '0', after: { host: 'web-1', unixUser: 'deploy' } }))
    expect(d.text).toBe('changed who can log in as deploy on web-1')
  })

  it('calls an unnamed agent by its number', () => {
    const d = describeEntry(e({ action: 'ssh_agent.rotate', targetType: 'ssh_agent', targetId: '8' }))
    expect(d.text).toBe('replaced the token of agent #8')
  })
})

describe('changedFields', () => {
  it('lists only what differs between two records', () => {
    expect(changedFields({ id: 3, name: 'a', tags: ['x'] }, { id: 3, name: 'b', tags: ['x', 'y'] })).toEqual([
      { key: 'name', from: 'a', to: 'b' },
      { key: 'tags', from: 'x', to: 'x, y' },
    ])
  })

  it('shows a field that appeared or went away', () => {
    expect(changedFields({ a: 1 }, { b: 2 })).toEqual([
      { key: 'a', from: '1', to: '' },
      { key: 'b', from: '', to: '2' },
    ])
  })

  it('has nothing to compare when either side is not a record', () => {
    expect(changedFields(undefined, { a: 1 })).toEqual([])
    expect(changedFields(['x'], ['y'])).toEqual([])
  })
})

describe('describeEntry for sudo and PIN events', () => {
  it('reads sudo as a sign-in event and flags wrong PINs', () => {
    expect(describeEntry(e({ action: 'sudo.grant' })).text).toBe('turned on sudo')
    const miss = describeEntry(e({ action: 'sudo.fail', after: { failures: 2 } }))
    expect(miss.text).toBe('entered a wrong PIN')
    expect(miss.detail).toBe('2 wrong in a row')
    expect(miss.tone).toBe('warn')
    expect(miss.category).toBe('signin')
    expect(describeEntry(e({ action: 'sudo.locked' })).tone).toBe('danger')
  })

  it('names a reset done from the command line', () => {
    const d = describeEntry(e({ action: 'account.pin_reset', actorLabel: 'cli', targetLabel: 'ops@example.com' }))
    expect(d.actor).toBe('Command line')
    expect(d.text).toBe('reset the sudo PIN of ops@example.com')
  })
})
