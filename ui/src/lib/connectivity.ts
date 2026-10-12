import { useSyncExternalStore } from 'react'
import { NetworkError } from './api'

export function isNetworkError(e: unknown): boolean {
  return e instanceof NetworkError
}

/**
 * Whether the last request failed to reach the server at all. Kept apart from
 * HTTP errors on purpose: a 401 means sign in again and a 503 means Headscale
 * is down, neither of which should hide the app behind an offline screen.
 */
export function createConnectivity() {
  let networkDown = false
  let snapshot = { networkDown }
  const listeners = new Set<() => void>()

  const set = (down: boolean) => {
    if (down === networkDown) return

    networkDown = down
    snapshot = { networkDown }
    listeners.forEach((l) => l())
  }

  return {
    report: (e: unknown) => {
      if (isNetworkError(e)) set(true)
    },
    ok: () => set(false),
    get: () => snapshot,
    subscribe: (fn: () => void) => {
      listeners.add(fn)

      return () => {
        listeners.delete(fn)
      }
    },
  }
}

export const connectivity = createConnectivity()

/**
 * The offline screen's rule. `meProblem` is the sign-in query's error or, while
 * it is still retrying, its last failure: a network failure there would
 * otherwise leave the app on "Loading…" for as long as the retries run.
 */
export function showOffline({ online, networkDown, meProblem }: { online: boolean; networkDown: boolean; meProblem: unknown }): boolean {
  return !online || networkDown || isNetworkError(meProblem)
}

function subscribeOnline(fn: () => void) {
  window.addEventListener('online', fn)
  window.addEventListener('offline', fn)

  return () => {
    window.removeEventListener('online', fn)
    window.removeEventListener('offline', fn)
  }
}

export function useOffline(meProblem: unknown): boolean {
  const online = useSyncExternalStore(subscribeOnline, () => navigator.onLine, () => true)
  const { networkDown } = useSyncExternalStore(connectivity.subscribe, connectivity.get, connectivity.get)

  return showOffline({ online, networkDown, meProblem })
}
