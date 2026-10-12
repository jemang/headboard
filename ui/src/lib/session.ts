import type { QueryClient } from '@tanstack/react-query'
import { ApiError } from './api'
import { connectivity } from './connectivity'

/**
 * Every failed request passes through here. A 401 means the session ended
 * (expired, or signed out in another tab): resetting `me` sends the shell back
 * to the sign-in screen instead of rendering cached tailnet data.
 */
export function onQueryError(qc: QueryClient, error: unknown, queryKey: readonly unknown[] = []) {
  connectivity.report(error)

  // `me` failing with 401 is already the signed-out state; resetting it would
  // refetch, fail again and loop.
  if (!(error instanceof ApiError) || error.status !== 401) return

  // Whoever signs in next must not see this account's tailnet, even briefly.
  qc.removeQueries({ predicate: (q) => q.queryKey[0] !== 'me' })
  if (queryKey[0] !== 'me') void qc.resetQueries({ queryKey: ['me'], exact: true })
}

/**
 * Mutations never end the session from here: /auth/login and /api/me/password
 * answer 401 for a wrong password, and the form must keep its fields and show
 * the error. A real expiry is caught by the next query instead.
 */
export function onMutationError(error: unknown) {
  connectivity.report(error)
}
