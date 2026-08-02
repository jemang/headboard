import { describe, expect, it } from 'vitest'
import type { Tokens } from '../lib/api'
import { validateTokens } from './TokenPicker'

const tokens: Tokens = {
  users: ['ali@'],
  groups: ['group:eng'],
  tags: ['tag:prod'],
  hosts: ['office-lan'],
  autogroups: ['autogroup:member'],
}

describe('validateTokens for grants', () => {
  it('accepts Headscale protocol-qualified grant ports', () => {
    expect(validateTokens(['tcp:443', 'udp:53', 'sctp:8000-9000', '*'], tokens, 'ip')).toEqual([])
  })

  it('keeps syntactically valid unresolved policy identifiers as warnings', () => {
    expect(validateTokens(['tag:agent'], tokens, 'src')).toEqual([
      { token: 'tag:agent', reason: '"tag:agent" has no owner in tagOwners', severity: 'warning' },
    ])
  })
})
