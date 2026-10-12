import { describe, expect, it } from 'vitest'
import { QueryClient } from '@tanstack/react-query'
import { ApiError, NetworkError } from './api'
import { onMutationError, onQueryError } from './session'

describe('onQueryError', () => {
  it('leaves me alone when me itself is the unauthorised request, but still purges the rest', () => {
    const qc = new QueryClient()
    qc.setQueryData(['me'], { user: { id: 1 } })
    qc.setQueryData(['devices'], { devices: [] })

    onQueryError(qc, new ApiError(401, 'unauthorised'), ['me'])

    expect(qc.getQueryData(['me'])).toEqual({ user: { id: 1 } })
    expect(qc.getQueryData(['devices'])).toBeUndefined()
  })

  it('drops the signed-in account when any request is unauthorised', () => {
    const qc = new QueryClient()
    qc.setQueryData(['me'], { user: { id: 1 } })

    onQueryError(qc, new ApiError(401, 'unauthorised'))

    expect(qc.getQueryData(['me'])).toBeUndefined()
  })

  it('drops every other cached query so the next person never sees them', () => {
    const qc = new QueryClient()
    qc.setQueryData(['devices'], { devices: [{ id: 1 }] })
    qc.setQueryData(['accounts'], { accounts: [] })

    onQueryError(qc, new ApiError(401, 'unauthorised'), ['devices'])

    expect(qc.getQueryData(['devices'])).toBeUndefined()
    expect(qc.getQueryData(['accounts'])).toBeUndefined()
  })

  it('keeps the account for other failures', () => {
    const qc = new QueryClient()
    qc.setQueryData(['me'], { user: { id: 1 } })

    onQueryError(qc, new ApiError(503, 'unavailable'))
    onQueryError(qc, new NetworkError())

    expect(qc.getQueryData(['me'])).toEqual({ user: { id: 1 } })
  })
})

describe('onMutationError', () => {
  it('keeps the account when a form gets a 401 (wrong password)', () => {
    const qc = new QueryClient()
    qc.setQueryData(['me'], { user: { id: 1 } })

    onMutationError(new ApiError(401, 'invalid email or password'))

    expect(qc.getQueryData(['me'])).toEqual({ user: { id: 1 } })
  })
})
