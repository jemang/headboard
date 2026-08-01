import { describe, expect, it } from 'vitest'
import type { AclRule, AclSchema, PatchOp } from './api'
import { rulesWithPendingChanges, schemaWithPendingChanges } from './policyDraft'

const rule = (src: string[], dst: string[]): AclRule => ({ action: 'accept', src, dst })

describe('rulesWithPendingChanges', () => {
  it('renders a pending new rule and its staged edit before the policy is saved', () => {
    const pending: PatchOp[] = [
      { op: 'add', path: '/acls/-', value: rule([], []) },
      { op: 'replace', path: '/acls/1', value: rule(['group:ops'], ['tag:prod:22']) },
    ]

    expect(rulesWithPendingChanges([rule(['group:eng'], ['tag:prod:443'])], pending)).toEqual([
      rule(['group:eng'], ['tag:prod:443']),
      rule(['group:ops'], ['tag:prod:22']),
    ])
  })

  it('removes a pending rule from the editable list', () => {
    expect(
      rulesWithPendingChanges(
        [rule(['group:eng'], ['tag:prod:443']), rule(['group:ops'], ['tag:prod:22'])],
        [{ op: 'remove', path: '/acls/1' }],
      ),
    ).toEqual([rule(['group:eng'], ['tag:prod:443'])])
  })
})

describe('schemaWithPendingChanges', () => {
  it('renders staged map entries and appended array entries before save', () => {
    const schema: AclSchema = {
      groups: { 'group:eng': ['ali@'] },
      hosts: { 'office-lan': '10.0.0.0/24' },
      ssh: [],
      tests: [],
    }
    const ssh = { action: 'accept', src: ['group:eng'], dst: ['tag:prod'], users: ['root'] }
    const test = { src: 'group:eng', accept: ['tag:prod:22'] }

    expect(
      schemaWithPendingChanges(schema, [
        { op: 'add', path: '/groups/group:qa', value: [] },
        { op: 'replace', path: '/hosts/office-lan', value: '10.1.0.0/16' },
        { op: 'add', path: '/ssh/-', value: ssh },
        { op: 'add', path: '/tests/-', value: test },
      ]),
    ).toEqual({
      groups: { 'group:eng': ['ali@'], 'group:qa': [] },
      hosts: { 'office-lan': '10.1.0.0/16' },
      ssh: [ssh],
      tests: [test],
    })
  })

  it('does not mutate the fetched policy schema', () => {
    const schema: AclSchema = { groups: { 'group:eng': ['ali@'] } }

    schemaWithPendingChanges(schema, [{ op: 'remove', path: '/groups/group:eng' }])

    expect(schema).toEqual({ groups: { 'group:eng': ['ali@'] } })
  })
})
