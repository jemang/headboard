import { describe, expect, it } from 'vitest'
import type { AclRule, PatchOp } from './api'
import { rulesWithPendingChanges } from './policyDraft'

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
