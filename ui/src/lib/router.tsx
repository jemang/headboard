import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react'
import { stripBase, withBase } from './basePath'
import { RouterContext, useRouter } from './routerContext'
import { parseLocation } from './routes'

/**
 * A ~50-line router, deliberately.
 *
 * Headboard has seven routes and no nesting, no loaders and no code splitting.
 * A routing library would be more configuration than the thing it configures,
 * and the plan's choice of TanStack Router was made before the route list was.
 * If routes start nesting, replace this — the surface is Link and useRoute.
 */

function currentHref() {
  return stripBase(window.location.pathname) + window.location.search + window.location.hash
}

export function Router({ children }: { children: ReactNode }) {
  // Routes are written without the deployment's base path, so a route table
  // does not change when the app moves from / to /manage. The base is added
  // back on the way into history and stripped on the way out. Query and hash
  // ride along so pages can hand off to each other (?device=3, #needs).
  const [href, setHref] = useState(currentHref)

  useEffect(() => {
    const onPop = () => setHref(currentHref())
    window.addEventListener('popstate', onPop)

    return () => window.removeEventListener('popstate', onPop)
  }, [])

  const navigate = useCallback((to: string, opts?: { replace?: boolean }) => {
    const target = withBase(to)
    if (target === window.location.pathname + window.location.search + window.location.hash) return

    window.history[opts?.replace ? 'replaceState' : 'pushState']({}, '', target)
    setHref(to)
  }, [])

  const loc = useMemo(() => parseLocation(href), [href])

  return <RouterContext value={{ ...loc, navigate }}>{children}</RouterContext>
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
      href={withBase(to)}
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
