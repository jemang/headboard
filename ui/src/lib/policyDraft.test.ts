import { describe, expect, it } from 'vitest'
import type { AclRule, AclSchema, Grant, PatchOp } from './api'
import { cascadePolicyDeletion, grantsWithPendingChanges, newMapEntryPatch, rulesWithPendingChanges, schemaWithPendingChanges } from './policyDraft'

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
  const firstMapEntries: Array<['groups' | 'tagOwners' | 'hosts', string, string | string[]]> = [
    ['groups', 'group:ops', ['ali@']],
    ['tagOwners', 'tag:prod', ['group:ops']],
    ['hosts', 'db', '100.64.0.8'],
  ]

  it.each(firstMapEntries)('creates the missing %s map before adding its first entry', (section, key, value) => {
    expect(newMapEntryPatch({}, [], section, key, value)).toEqual({
      op: 'add', path: `/${section}`, value: { [key]: value },
    })
  })

  it('adds another entry under a map that already exists', () => {
    expect(newMapEntryPatch({ hosts: { db: '100.64.0.8' } }, [], 'hosts', 'cache', '100.64.0.9')).toEqual({
      op: 'add', path: '/hosts/cache', value: '100.64.0.9',
    })
  })

  it('keeps the first group addressable when staging its members', () => {
    const firstGroup = newMapEntryPatch({}, [], 'groups', 'group:ops', [])

    expect(schemaWithPendingChanges({}, [
      firstGroup,
      { op: 'replace', path: '/groups/group:ops', value: ['ali@'] },
    ])).toEqual({ groups: { 'group:ops': ['ali@'] } })
  })

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

describe('cascadePolicyDeletion', () => {
  it('removes a group and its dependencies without dropping surviving access', () => {
    const schema: AclSchema = {
      groups: { 'group:ops': ['ops@'], 'group:eng': ['alice@'] },
      tagOwners: { 'tag:prod': ['group:ops'], 'tag:ci': ['group:eng'] },
      acls: [
        rule(['group:ops', 'group:eng'], ['tag:prod:443', 'tag:ci:443']),
        rule(['group:ops'], ['tag:ci:22']),
      ],
      grants: [grant(['group:ops', 'alice@'], ['tag:prod:443', 'tag:ci:443'], ['*'])],
      ssh: [{ action: 'accept', src: ['group:ops'], dst: ['tag:prod'], users: ['root'] }],
      autoApprovers: { routes: { '10.0.0.0/24': ['tag:prod', 'tag:ci'] }, exitNode: ['tag:prod', 'tag:ci'] },
      tests: [
        { src: 'group:ops', accept: ['tag:prod:22'] },
        { src: 'alice@', accept: ['tag:prod:22', 'tag:ci:22'] },
      ],
      sshTests: [
        { src: 'group:ops', dst: ['tag:prod'], accept: ['root'] },
        { src: 'alice@', dst: ['tag:prod', 'tag:ci'], accept: ['root'] },
      ],
    }

    const result = cascadePolicyDeletion(schema, [], { section: 'groups', name: 'group:ops' })
    const next = schemaWithPendingChanges(schema, result.ops)

    expect(next.groups).toEqual({ 'group:eng': ['alice@'] })
    expect(next.tagOwners).toEqual({ 'tag:ci': ['group:eng'] })
    expect(next.acls).toEqual([rule(['group:eng'], ['tag:ci:443'])])
    expect(next.grants).toEqual([grant(['alice@'], ['tag:ci:443'], ['*'])])
    expect(next.ssh).toEqual([])
    expect(next.autoApprovers).toEqual({ routes: { '10.0.0.0/24': ['tag:ci'] }, exitNode: ['tag:ci'] })
    expect(next.tests).toEqual([{ src: 'alice@', accept: ['tag:ci:22'] }])
    expect(next.sshTests).toEqual([{ src: 'alice@', dst: ['tag:ci'], accept: ['root'] }])
    expect(result.affected).toBeGreaterThan(1)
    expect(result.ops).toEqual(expect.arrayContaining([
      { op: 'remove', path: '/groups/group:ops' },
      { op: 'remove', path: '/tagOwners/tag:prod' },
      { op: 'remove', path: '/acls/1' },
      { op: 'remove', path: '/ssh/0' },
    ]))
    expect(result.ops).not.toContainEqual(expect.objectContaining({ path: '/acls' }))
  })

  it('removes a tag, including port-qualified references and grants restricted through it', () => {
    const schema: AclSchema = {
      tagOwners: { 'tag:prod': ['ali@'], 'tag:ci': ['ali@'] },
      acls: [rule(['tag:prod', 'alice@'], ['tag:prod:443', 'tag:ci:443'])],
      grants: [
        grant(['alice@'], ['tag:ci:443'], ['*'], ['tag:prod']),
        grant(['alice@'], ['tag:prod:443', 'tag:ci:443'], ['*']),
      ],
      ssh: [{ action: 'accept', src: ['alice@'], dst: ['tag:prod', 'tag:ci'], users: ['root'] }],
      autoApprovers: { routes: { '10.0.0.0/24': ['tag:prod', 'tag:ci'] }, exitNode: ['tag:prod', 'tag:ci'] },
      tests: [
        { src: 'tag:prod', accept: ['tag:ci:22'] },
        { src: 'alice@', accept: ['tag:prod:22'], deny: ['tag:ci:22'] },
      ],
      sshTests: [{ src: 'alice@', dst: ['tag:prod', 'tag:ci'], accept: ['root'] }],
    }

    const result = cascadePolicyDeletion(schema, [], { section: 'tagOwners', name: 'tag:prod' })
    const next = schemaWithPendingChanges(schema, result.ops)

    expect(next.tagOwners).toEqual({ 'tag:ci': ['ali@'] })
    expect(next.acls).toEqual([rule(['alice@'], ['tag:ci:443'])])
    expect(next.grants).toEqual([grant(['alice@'], ['tag:ci:443'], ['*'])])
    expect(next.ssh).toEqual([{ action: 'accept', src: ['alice@'], dst: ['tag:ci'], users: ['root'] }])
    expect(next.autoApprovers).toEqual({ routes: { '10.0.0.0/24': ['tag:ci'] }, exitNode: ['tag:ci'] })
    expect(next.tests).toEqual([{ src: 'alice@', deny: ['tag:ci:22'] }])
    expect(next.sshTests).toEqual([{ src: 'alice@', dst: ['tag:ci'], accept: ['root'] }])
    expect(result.affected).toBeGreaterThan(1)
  })
})
