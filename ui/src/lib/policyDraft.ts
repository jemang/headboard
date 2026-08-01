import type { AclRule, AclSchema, PatchOp } from './api'

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

function isAclRule(value: unknown): value is AclRule {
  return typeof value === 'object' && value !== null && 'action' in value && 'src' in value && 'dst' in value
}

function copyRule(rule: AclRule): AclRule {
  return { ...rule, src: [...rule.src], dst: [...rule.dst] }
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

    if (operation.op === 'add') parent.splice(index, 0, operation.value)
    else if (operation.op === 'replace' && index < parent.length) parent[index] = operation.value
    else if (operation.op === 'remove' && index < parent.length) parent.splice(index, 1)

    return
  }

  if (operation.op === 'add' || (operation.op === 'replace' && key in parent)) parent[key] = operation.value
  else if (operation.op === 'remove') delete parent[key]
}

function editableSection(section: string) {
  return ['groups', 'hosts', 'tagOwners', 'acls', 'ssh', 'autoApprovers', 'tests', 'sshTests'].includes(section)
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
