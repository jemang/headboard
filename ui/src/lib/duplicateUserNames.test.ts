import { describe, expect, it } from 'vitest'
import type { TailnetUser } from './api'
import { duplicateUserNames } from './duplicateUserNames'

const user = (overrides: Partial<TailnetUser> = {}): TailnetUser => ({
  id: 1,
  name: 'user',
  devices: 0,
  ...overrides,
})

describe('duplicateUserNames', () => {
  it('flags names shared by more than one user, case-insensitively', () => {
    const users = [
      user({ id: 1, name: 'jemang' }),
      user({ id: 2, name: 'Jemang' }),
      user({ id: 3, name: 'alice' }),
    ]

    expect(duplicateUserNames(users)).toEqual(new Set(['jemang']))
  })

  it('returns an empty set when every name is unique', () => {
    const users = [user({ id: 1, name: 'alice' }), user({ id: 2, name: 'bob' })]

    expect(duplicateUserNames(users)).toEqual(new Set())
  })
})
