import { describe, expect, it } from 'vitest'
import { duplicateAccountEmails } from './duplicateAccountEmails'

describe('duplicateAccountEmails', () => {
  it('flags an email held by two accounts, case-insensitively', () => {
    const dup = duplicateAccountEmails([{ email: 'ops@headboard.test' }, { email: 'OPS@headboard.test' }, { email: 'a@x' }])
    expect([...dup]).toEqual(['ops@headboard.test'])
  })
})

describe('duplicateAccountEmails with blank emails', () => {
  it('does not flag accounts that simply have no email', () => {
    expect([...duplicateAccountEmails([{ email: '' }, { email: '' }, { email: 'a@x' }])]).toEqual([])
  })
})
