import type { AclRule, AclSchema, AclTest, Grant, PatchOp, SshRule, SshTest } from './api'

/**
 * The API query is the last saved schema; form tabs also need to render the
 * ordered JSON Patch draft. Keeping this projection pure means every tab sees
 * the same document that Preview and Save submit, without mutating query data.
 */
export function schemaWithPendingChanges(schema: AclSchema | undefined, pending: PatchOp[]): AclSchema {
  const draft = structuredClone(schema ?? {}) as AclSchema

  for (const operation of pending) applySchemaOperation(draft as Record<string, unknown>, operation)

  return draft
}

type MapSection = 'groups' | 'tagOwners' | 'hosts'

// newMapEntryPatch creates the top-level map when the stored policy does not
// have it yet. RFC 6902 cannot add /groups/group:ops while /groups is absent.
export function newMapEntryPatch(
  schema: AclSchema | undefined,
  pending: PatchOp[],
  section: MapSection,
  key: string,
  value: string | string[],
): PatchOp {
  const projected = schemaWithPendingChanges(schema, pending)

  if (projected[section] === undefined) {
    return { op: 'add', path: `/${section}`, value: { [key]: value } }
  }

  return { op: 'add', path: `/${section}/${escapePointer(key)}`, value }
}

type DeletionTarget = { section: 'groups' | 'tagOwners'; name: string }

export type CascadeResult = { ops: PatchOp[]; affected: number }

/**
 * cascadePolicyDeletion removes a group or tag together with the policy
 * references that would otherwise make the document invalid. It projects the
 * current staged patch queue first, then emits only section-level patches for
 * the resulting cleanup so Preview and Save review the exact same document.
 */
export function cascadePolicyDeletion(
  schema: AclSchema | undefined,
  pending: PatchOp[],
  target: DeletionTarget,
): CascadeResult {
  const projected = schemaWithPendingChanges(schema, pending)
  const next = structuredClone(projected)
  const removed = new Set<string>()
  let affected = 0

  const removeTarget = (current: DeletionTarget) => {
    const identity = `${current.section}:${current.name}`
    if (removed.has(identity)) return
    removed.add(identity)

    if (current.section === 'groups') {
      if (next.groups?.[current.name] === undefined) return

      delete next.groups[current.name]
      affected++

      for (const [tag, owners] of Object.entries(next.tagOwners ?? {})) {
        if (!owners.includes(current.name)) continue

        const remaining = owners.filter((owner) => owner !== current.name)
        if (remaining.length === 0) removeTarget({ section: 'tagOwners', name: tag })
        else {
          next.tagOwners![tag] = remaining
          affected++
        }
      }
    } else {
      if (next.tagOwners?.[current.name] === undefined) return

      delete next.tagOwners[current.name]
      affected++
    }

    cleanReferences(next, current, () => {
      affected++
    })
  }

  removeTarget(target)

  return { ops: cascadeOperations(projected, next), affected }
}

function cleanReferences(schema: AclSchema, target: DeletionTarget, changed: () => void) {
  schema.acls = cleanRules(schema.acls, target, changed)
  schema.grants = cleanGrants(schema.grants, target, changed)
  schema.ssh = cleanSSH(schema.ssh, target, changed)
  schema.autoApprovers = cleanAutoApprovers(schema.autoApprovers, target, changed)
  schema.tests = cleanTests(schema.tests, target, changed)
  schema.sshTests = cleanSSHTests(schema.sshTests, target, changed)
}

function cleanRules(rules: AclRule[] | undefined, target: DeletionTarget, changed: () => void): AclRule[] | undefined {
  if (!rules) return rules

  return rules.flatMap((rule) => {
    const src = withoutReference(rule.src, target)
    const dst = withoutReference(rule.dst, target)
    if (src.length === rule.src.length && dst.length === rule.dst.length) return [rule]
    if (src.length === 0 || dst.length === 0) {
      changed()
      return []
    }

    changed()
    return [{ ...rule, src, dst }]
  })
}

function cleanGrants(grants: Grant[] | undefined, target: DeletionTarget, changed: () => void): Grant[] | undefined {
  if (!grants) return grants

  return grants.flatMap((grant) => {
    const src = withoutReference(grant.src, target)
    const dst = withoutReference(grant.dst, target)
    const viaReferencesTarget = grant.via?.some((via) => isReference(via, target)) ?? false
    if (src.length === grant.src.length && dst.length === grant.dst.length && !viaReferencesTarget) return [grant]
    if (src.length === 0 || dst.length === 0 || viaReferencesTarget) {
      changed()
      return []
    }

    changed()
    return [{ ...grant, src, dst }]
  })
}

function cleanSSH(rules: SshRule[] | undefined, target: DeletionTarget, changed: () => void): SshRule[] | undefined {
  if (!rules) return rules

  return rules.flatMap((rule) => {
    const src = withoutReference(rule.src, target)
    const dst = withoutReference(rule.dst, target)
    if (src.length === rule.src.length && dst.length === rule.dst.length) return [rule]
    if (src.length === 0 || dst.length === 0) {
      changed()
      return []
    }

    changed()
    return [{ ...rule, src, dst }]
  })
}

function cleanAutoApprovers(
  auto: AclSchema['autoApprovers'],
  target: DeletionTarget,
  changed: () => void,
): AclSchema['autoApprovers'] {
  if (!auto) return auto

  const routes = Object.fromEntries(Object.entries(auto.routes ?? {}).flatMap(([route, approvers]) => {
    const remaining = withoutReference(approvers, target)
    if (remaining.length === approvers.length) return [[route, approvers]]
    changed()
    return remaining.length > 0 ? [[route, remaining]] : []
  }))
  const exitNode = withoutReference(auto.exitNode ?? [], target)
  if (exitNode.length !== (auto.exitNode ?? []).length) changed()

  return { ...auto, ...(auto.routes ? { routes } : {}), ...(auto.exitNode ? { exitNode } : {}) }
}

function cleanTests(tests: AclTest[] | undefined, target: DeletionTarget, changed: () => void): AclTest[] | undefined {
  if (!tests) return tests

  return tests.flatMap((test) => {
    if (isReference(test.src, target)) {
      changed()
      return []
    }

    const accept = withoutReference(test.accept ?? [], target)
    const deny = withoutReference(test.deny ?? [], target)
    if (accept.length === (test.accept ?? []).length && deny.length === (test.deny ?? []).length) return [test]
    if (accept.length === 0 && deny.length === 0) {
      changed()
      return []
    }

    changed()
    const { accept: _accept, deny: _deny, ...remaining } = test
    return [{ ...remaining, ...(accept.length > 0 ? { accept } : {}), ...(deny.length > 0 ? { deny } : {}) }]
  })
}

function cleanSSHTests(tests: SshTest[] | undefined, target: DeletionTarget, changed: () => void): SshTest[] | undefined {
  if (!tests) return tests

  return tests.flatMap((test) => {
    const dst = withoutReference(test.dst, target)
    if (isReference(test.src, target) || dst.length === 0) {
      changed()
      return []
    }
    if (dst.length === test.dst.length) return [test]

    changed()
    return [{ ...test, dst }]
  })
}

function withoutReference(values: string[], target: DeletionTarget) {
  return values.filter((value) => !isReference(value, target))
}

function isReference(value: string, target: DeletionTarget) {
  return target.section === 'groups'
    ? value === target.name
    : value === target.name || value.startsWith(`${target.name}:`)
}

function same(left: unknown, right: unknown) {
  return JSON.stringify(left) === JSON.stringify(right)
}

function cascadeOperations(before: AclSchema, after: AclSchema): PatchOp[] {
  return [
    ...mapOperations('/groups', before.groups, after.groups),
    ...mapOperations('/tagOwners', before.tagOwners, after.tagOwners),
    ...arrayOperations('/acls', before.acls, after.acls),
    ...arrayOperations('/grants', before.grants, after.grants),
    ...arrayOperations('/ssh', before.ssh, after.ssh),
    ...autoApproverOperations(before.autoApprovers, after.autoApprovers),
    ...arrayOperations('/tests', before.tests, after.tests),
    ...arrayOperations('/sshTests', before.sshTests, after.sshTests),
  ]
}

function mapOperations<T>(path: string, before: Record<string, T> | undefined, after: Record<string, T> | undefined): PatchOp[] {
  const beforeEntries = before ?? {}
  const afterEntries = after ?? {}
  const ops: PatchOp[] = []

  for (const key of Object.keys(beforeEntries)) {
    const previous = beforeEntries[key]
    const next = afterEntries[key]
    const entryPath = `${path}/${escapePointer(key)}`

    if (next === undefined) ops.push({ op: 'remove', path: entryPath })
    else if (!same(previous, next)) ops.push({ op: 'replace', path: entryPath, value: next })
  }

  for (const [key, next] of Object.entries(afterEntries)) {
    if (beforeEntries[key] === undefined) ops.push({ op: 'add', path: `${path}/${escapePointer(key)}`, value: next })
  }

  return ops
}

function arrayOperations<T>(path: string, before: T[] | undefined, after: T[] | undefined): PatchOp[] {
  const previous = before ?? []
  const next = after ?? []
  const pairs = unchangedPairs(previous, next)
  const replacements: PatchOp[] = []
  const removals: PatchOp[] = []
  const additions: PatchOp[] = []
  let previousStart = 0
  let nextStart = 0

  for (const [previousIndex, nextIndex] of [...pairs, [previous.length, next.length]]) {
    const previousLength = previousIndex - previousStart
    const nextLength = nextIndex - nextStart
    const shared = Math.min(previousLength, nextLength)

    for (let index = 0; index < shared; index++) {
      replacements.push({ op: 'replace', path: `${path}/${previousStart + index}`, value: next[nextStart + index] })
    }
    for (let index = previousLength - 1; index >= shared; index--) {
      removals.push({ op: 'remove', path: `${path}/${previousStart + index}` })
    }
    for (let index = shared; index < nextLength; index++) {
      additions.push({ op: 'add', path: `${path}/-`, value: next[nextStart + index] })
    }

    previousStart = previousIndex + 1
    nextStart = nextIndex + 1
  }

  return [...replacements, ...removals.sort((left, right) => Number(right.path.split('/').at(-1)) - Number(left.path.split('/').at(-1))), ...additions]
}

function unchangedPairs<T>(before: T[], after: T[]): Array<[number, number]> {
  const lengths = Array.from({ length: before.length + 1 }, () => Array(after.length + 1).fill(0))

  for (let previous = before.length - 1; previous >= 0; previous--) {
    for (let next = after.length - 1; next >= 0; next--) {
      lengths[previous][next] = same(before[previous], after[next])
        ? lengths[previous + 1][next + 1] + 1
        : Math.max(lengths[previous + 1][next], lengths[previous][next + 1])
    }
  }

  const pairs: Array<[number, number]> = []
  let previous = 0
  let next = 0

  while (previous < before.length && next < after.length) {
    if (same(before[previous], after[next])) {
      pairs.push([previous, next])
      previous++
      next++
    } else if (lengths[previous + 1][next] >= lengths[previous][next + 1]) {
      previous++
    } else {
      next++
    }
  }

  return pairs
}

function autoApproverOperations(before: AclSchema['autoApprovers'], after: AclSchema['autoApprovers']): PatchOp[] {
  if (!before || !after) return same(before, after) ? [] : [{ op: before ? 'replace' : 'add', path: '/autoApprovers', value: after }]

  return [
    ...mapOperations('/autoApprovers/routes', before.routes, after.routes),
    ...(same(before.exitNode, after.exitNode) ? [] : [{ op: before.exitNode === undefined ? 'add' : 'replace', path: '/autoApprovers/exitNode', value: after.exitNode } satisfies PatchOp]),
  ]
}

export function rulesWithPendingChanges(rules: AclRule[], pending: PatchOp[]): AclRule[] {
  const draft = rules.map(copyRule)

  for (const operation of pending) {
    const match = operation.path.match(/^\/acls\/(\d+|-)$/)

    if (!match) continue

    const target = match[1]
    const index = target === '-' ? draft.length : Number(target)

    if (operation.op === 'add' && isAclRule(operation.value)) {
      draft.splice(index, 0, copyRule(operation.value))
    }

    if (operation.op === 'replace' && isAclRule(operation.value) && index < draft.length) {
      draft[index] = copyRule(operation.value)
    }

    if (operation.op === 'remove' && index < draft.length) {
      draft.splice(index, 1)
    }
  }

  return draft
}

export function grantsWithPendingChanges(grants: Grant[], pending: PatchOp[]): Grant[] {
  const draft = grants.map(copyGrant)

  for (const operation of pending) {
    if (operation.path === '/grants') {
      if ((operation.op === 'add' || operation.op === 'replace') && Array.isArray(operation.value) && operation.value.every(isGrant)) {
        draft.splice(0, draft.length, ...operation.value.map(copyGrant))
      }

      if (operation.op === 'remove') draft.splice(0, draft.length)

      continue
    }

    const match = operation.path.match(/^\/grants\/(\d+|-)$/)

    if (!match) continue

    const target = match[1]
    const index = target === '-' ? draft.length : Number(target)

    if (operation.op === 'add' && isGrant(operation.value)) {
      draft.splice(index, 0, copyGrant(operation.value))
    }

    if (operation.op === 'replace' && isGrant(operation.value) && index < draft.length) {
      draft[index] = copyGrant(operation.value)
    }

    if (operation.op === 'remove' && index < draft.length) {
      draft.splice(index, 1)
    }
  }

  return draft
}

function isAclRule(value: unknown): value is AclRule {
  return typeof value === 'object' && value !== null && 'action' in value && 'src' in value && 'dst' in value
}

function isGrant(value: unknown): value is Grant {
  return typeof value === 'object' && value !== null && 'src' in value && 'dst' in value
}

function copyRule(rule: AclRule): AclRule {
  return { ...rule, src: [...rule.src], dst: [...rule.dst] }
}

function copyGrant(grant: Grant): Grant {
  return {
    ...grant,
    src: [...grant.src],
    dst: [...grant.dst],
    ...(grant.ip ? { ip: [...grant.ip] } : {}),
    ...(grant.via ? { via: [...grant.via] } : {}),
  }
}

function applySchemaOperation(root: Record<string, unknown>, operation: PatchOp) {
  if (!['add', 'replace', 'remove'].includes(operation.op) || !operation.path.startsWith('/')) return

  const parts = operation.path.slice(1).split('/').map(unescape)
  if (parts.length === 0 || !editableSection(parts[0])) return

  let parent: unknown = root

  for (let index = 0; index < parts.length - 1; index++) {
    if (!isRecord(parent) && !Array.isArray(parent)) return

    const key = parts[index]
    const next = parts[index + 1]
    const current = Array.isArray(parent) ? parent[arrayIndex(key)] : parent[key]

    if (current !== undefined) {
      parent = current
      continue
    }

    if (operation.op !== 'add' || Array.isArray(parent)) return

    const container: unknown = next === '-' || isArrayIndex(next) ? [] : {}
    parent[key] = container
    parent = container
  }

  const key = parts.at(-1)
  if (!key || (!isRecord(parent) && !Array.isArray(parent))) return

  if (Array.isArray(parent)) {
    const index = key === '-' ? parent.length : arrayIndex(key)
    if (index < 0 || index > parent.length) return

    if (operation.op === 'add') parent.splice(index, 0, detached(operation.value))
    else if (operation.op === 'replace' && index < parent.length) parent[index] = detached(operation.value)
    else if (operation.op === 'remove' && index < parent.length) parent.splice(index, 1)

    return
  }

  if (operation.op === 'add' || (operation.op === 'replace' && key in parent)) {
    parent[key] = detached(operation.value)
  } else if (operation.op === 'remove') delete parent[key]
}

/**
 * detached copies a patch value on its way into the draft.
 *
 * Without it the draft holds the operation's own object, so a later operation
 * addressing through that container writes back into the queued patch: staging
 * `add /grants` then `add /grants/-` appended the second grant to the *first
 * operation's* array. Every render projects again, so each pass grew the queue
 * — three clicks rendered four cards, then six, and the list kept growing
 * without anyone touching the button.
 */
function detached(value: unknown): unknown {
  return typeof value === 'object' && value !== null ? structuredClone(value) : value
}

function editableSection(section: string) {
  return ['groups', 'hosts', 'tagOwners', 'acls', 'grants', 'ssh', 'autoApprovers', 'tests', 'sshTests'].includes(section)
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

function isArrayIndex(value: string) {
  return /^0$|^[1-9]\d*$/.test(value)
}

function arrayIndex(value: string) {
  return isArrayIndex(value) ? Number(value) : -1
}

function unescape(value: string) {
  return value.replaceAll('~1', '/').replaceAll('~0', '~')
}

function escapePointer(value: string) {
  return value.replaceAll('~', '~0').replaceAll('/', '~1')
}
