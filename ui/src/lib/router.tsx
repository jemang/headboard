import { createContext, useCallback, useContext, useEffect, useState, type ReactNode } from 'react'

/**
 * A ~50-line router, deliberately.
 *
 * Headboard has seven routes and no nesting, no loaders and no code splitting.
 * A routing library would be more configuration than the thing it configures,
 * and the plan's choice of TanStack Router was made before the route list was.
 * If routes start nesting, replace this — the surface is Link and useRoute.
 */

interface RouterValue {
  path: string
  navigate: (to: string, opts?: { replace?: boolean }) => void
}

const RouterContext = createContext<RouterValue>({ path: '/', navigate: () => {} })

export function Router({ children }: { children: ReactNode }) {
  const [path, setPath] = useState(() => window.location.pathname)

  useEffect(() => {
    const onPop = () => setPath(window.location.pathname)
    window.addEventListener('popstate', onPop)

    return () => window.removeEventListener('popstate', onPop)
  }, [])

  const navigate = useCallback((to: string, opts?: { replace?: boolean }) => {
    if (to === window.location.pathname) return

    window.history[opts?.replace ? 'replaceState' : 'pushState']({}, '', to)
    setPath(to)
  }, [])

  return <RouterContext value={{ path, navigate }}>{children}</RouterContext>
}

export function useRouter() {
  return useContext(RouterContext)
}

export function Link({
  to,
  className,
  children,
  onClick,
}: {
  to: string
  className?: string
  children: ReactNode
  onClick?: () => void
}) {
  const { navigate } = useRouter()

  return (
    <a
      href={to}
      className={className}
      onClick={(e) => {
        // Let modified clicks open a new tab, as any link should.
        if (e.metaKey || e.ctrlKey || e.shiftKey || e.button !== 0) return

        e.preventDefault()
        onClick?.()
        navigate(to)
      }}
    >
      {children}
    </a>
  )
}
