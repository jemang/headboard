import { useCallback, useSyncExternalStore } from 'react'

export const MD = '(min-width: 48rem)'
export const LG = '(min-width: 64rem)'
export const XL = '(min-width: 80rem)'

/**
 * Mounts one renderer per breakpoint instead of hiding a second tree with CSS,
 * which doubled the work for a long device list. False when there is no window.
 */
export function useMediaQuery(query: string): boolean {
  const subscribe = useCallback((fn: () => void) => {
    const m = window.matchMedia(query)
    m.addEventListener('change', fn)

    return () => m.removeEventListener('change', fn)
  }, [query])

  return useSyncExternalStore(subscribe, () => window.matchMedia(query).matches, () => false)
}
