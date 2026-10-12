import { createContext, useContext } from 'react'

export interface RouterValue {
  path: string
  navigate: (to: string, opts?: { replace?: boolean }) => void
}

// Kept apart from router.tsx so that file exports only components, which is
// what Vite's fast refresh needs to hot-swap it.
export const RouterContext = createContext<RouterValue>({ path: '/', navigate: () => {} })

export function useRouter() {
  return useContext(RouterContext)
}
