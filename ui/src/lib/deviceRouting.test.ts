import { describe, expect, it } from 'vitest'
import { approveRoutes, revokeRoutes, routeSummary } from './deviceRouting'

describe('routeSummary', () => {
  it('recognises a complete pending exit-node request', () => {
    expect(routeSummary(['0.0.0.0/0', '::/0'], []).exit).toEqual({
      state: 'pending',
      routes: ['0.0.0.0/0', '::/0'],
    })
  })

  it('flags a partial exit-node advertisement as incomplete', () => {
    expect(routeSummary(['0.0.0.0/0'], []).exit).toEqual({
      state: 'incomplete',
      routes: ['0.0.0.0/0'],
    })
  })

  it('keeps subnet routes separate from exit-node routes', () => {
    expect(routeSummary(['10.0.0.0/24'], ['10.0.0.0/24'])).toEqual({
      exit: { state: 'none', routes: [] },
      subnets: [{ route: '10.0.0.0/24', approved: true }],
    })
  })
})

describe('route approval sets', () => {
  it('approves an exit node without dropping an approved subnet', () => {
    expect(approveRoutes(['10.0.0.0/24'], ['0.0.0.0/0', '::/0']))
      .toEqual(['10.0.0.0/24', '0.0.0.0/0', '::/0'])
  })

  it('revokes one subnet without dropping the exit-node pair', () => {
    expect(revokeRoutes(['10.0.0.0/24', '0.0.0.0/0', '::/0'], ['10.0.0.0/24']))
      .toEqual(['0.0.0.0/0', '::/0'])
  })
})
