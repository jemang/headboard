import { describe, expect, it } from 'vitest'
import { ApiError, NetworkError } from './api'
import { createConnectivity, isNetworkError, showOffline } from './connectivity'

describe('isNetworkError', () => {
  it('treats a request that never reached the server as a network error', () => {
    expect(isNetworkError(new NetworkError())).toBe(true)
  })

  it('does not treat a programming TypeError as offline', () => {
    expect(isNetworkError(new TypeError("Cannot read properties of undefined"))).toBe(false)
  })

  it('does not treat 401 or 503 as offline', () => {
    expect(isNetworkError(new ApiError(401, 'unauthorised'))).toBe(false)
    expect(isNetworkError(new ApiError(503, 'unavailable'))).toBe(false)
  })
})

describe('connectivity', () => {
  it('goes down on a network error and back up on success', () => {
    const c = createConnectivity()
    const seen: boolean[] = []
    c.subscribe(() => seen.push(c.get().networkDown))

    c.report(new NetworkError())
    c.report(new ApiError(500, 'boom'))
    c.ok()

    expect(seen).toEqual([true, false])
  })
})

describe('showOffline', () => {
  it('shows offline while me keeps failing to reach the server', () => {
    expect(showOffline({ online: true, networkDown: false, meProblem: new NetworkError() })).toBe(true)
  })

  it('does not show offline for a 401', () => {
    expect(showOffline({ online: true, networkDown: false, meProblem: new ApiError(401, 'unauthorised') })).toBe(false)
  })

  it('shows offline when the browser says so or a request failed', () => {
    expect(showOffline({ online: false, networkDown: false, meProblem: null })).toBe(true)
    expect(showOffline({ online: true, networkDown: true, meProblem: null })).toBe(true)
    expect(showOffline({ online: true, networkDown: false, meProblem: null })).toBe(false)
  })
})
