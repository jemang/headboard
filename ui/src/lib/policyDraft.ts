import type { AclRule, PatchOp } from './api'

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
