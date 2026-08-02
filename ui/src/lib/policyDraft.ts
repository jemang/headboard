import type { AclRule, AclSchema, Grant, PatchOp } from './api'

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
