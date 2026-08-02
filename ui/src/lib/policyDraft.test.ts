import { describe, expect, it } from 'vitest'
import type { AclRule, AclSchema, Grant, PatchOp } from './api'
import { grantsWithPendingChanges, rulesWithPendingChanges, schemaWithPendingChanges } from './policyDraft'

const rule = (src: string[], dst: string[]): AclRule => ({ action: 'accept', src, dst })
const grant = (src: string[], dst: string[], ip: string[], via?: string[], app?: unknown): Grant => ({
  src,
  dst,
  ip,
  ...(via ? { via } : {}),
  ...(app ? { app } : {}),
})

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

describe('grantsWithPendingChanges', () => {
  it('renders the first pending grant when it creates the grants section', () => {
    const firstGrant = grant(['tag:agent'], ['13.0.0.0/24'], ['*'])

    expect(grantsWithPendingChanges([], [{ op: 'add', path: '/grants', value: [firstGrant] }])).toEqual([firstGrant])
  })

  it('renders pending grant add, replacement, and removal without losing app data', () => {
    const app = { connector: { name: 'example' } }
    const agent = grant(['tag:agent'], ['13.0.0.0/24'], ['*'], undefined, app)
    const ali = grant(['ali@'], ['11.1.0.105'], ['22'], ['tag:router'])

    expect(
      grantsWithPendingChanges(
        [agent, grant(['tag:test'], ['11.0.0.156'], ['22'])],
        [
          { op: 'add', path: '/grants/-', value: ali },
          { op: 'replace', path: '/grants/1', value: grant(['tag:dev'], ['11.1.0.106'], ['*']) },
          { op: 'remove', path: '/grants/2' },
        ],
      ),
    ).toEqual([agent, grant(['tag:dev'], ['11.1.0.106'], ['*'])])
  })
})

describe('schemaWithPendingChanges', () => {
  it('renders staged tags and routing approvals before the policy is saved', () => {
    expect(
      schemaWithPendingChanges({}, [
        { op: 'add', path: '/tagOwners/tag:agent', value: ['group:ops'] },
        { op: 'add', path: '/autoApprovers', value: { routes: { '10.0.0.0/24': ['group:ops'] } } },
        { op: 'add', path: '/autoApprovers/exitNode', value: ['group:ops'] },
        { op: 'add', path: '/autoApprovers/services/svc:metrics', value: ['tag:metrics'] },
      ]),
    ).toEqual({
      tagOwners: { 'tag:agent': ['group:ops'] },
      autoApprovers: {
        routes: { '10.0.0.0/24': ['group:ops'] },
        exitNode: ['group:ops'],
        services: { 'svc:metrics': ['tag:metrics'] },
      },
    })
  })

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
        { op: 'add', path: '/grants/-', value: grant(['tag:agent'], ['13.0.0.0/24'], ['*']) },
      ]),
    ).toEqual({
      groups: { 'group:eng': ['ali@'], 'group:qa': [] },
      hosts: { 'office-lan': '10.1.0.0/16' },
      ssh: [ssh],
      tests: [test],
      grants: [grant(['tag:agent'], ['13.0.0.0/24'], ['*'])],
    })
  })

  it('does not mutate the fetched policy schema', () => {
    const schema: AclSchema = { groups: { 'group:eng': ['ali@'] } }

    schemaWithPendingChanges(schema, [{ op: 'remove', path: '/groups/group:eng' }])

    expect(schema).toEqual({ groups: { 'group:eng': ['ali@'] } })
  })
})

describe('projection isolation', () => {
  // The Grants tab computes "does a grants section exist yet" by projecting the
  // pending queue on every render. The projection used to store the operation's
  // own array in the draft, so the next operation addressing through it wrote
  // back into the queued patch: three Add grant clicks rendered four cards, and
  // every later render grew the list again with nobody touching the button.
  it('never writes back into a queued operation', () => {
    const create: PatchOp = { op: 'add', path: '/grants', value: [grant([], [], ['*'])] }
    const append: PatchOp = { op: 'add', path: '/grants/-', value: grant([], [], ['*']) }
    const pending = [create, append, { ...append }]

    const schema: AclSchema = { acls: [rule(['group:eng'], ['tag:prod:443'])] }

    for (let pass = 0; pass < 3; pass++) {
      expect(Object.hasOwn(schemaWithPendingChanges(schema, pending), 'grants')).toBe(true)
    }

    expect(create.value).toEqual([grant([], [], ['*'])])
    expect(grantsWithPendingChanges(schema.grants ?? [], pending)).toHaveLength(3)
  })
})
