import { useEffect } from 'react'
import { useRouter } from './routerContext'

/**
 * Scrolls to the element named by the URL hash. Link prevents the browser's
 * own anchor jump, and sections often render after their queries load, so this
 * re-runs whenever `ready` changes.
 */
export function useHashScroll(ready: unknown) {
  const { hash } = useRouter()

  useEffect(() => {
    if (hash) document.getElementById(hash)?.scrollIntoView({ block: 'start' })
  }, [hash, ready])
}
