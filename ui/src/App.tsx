import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, ApiError, type Me } from './lib/api'
import { Link, Router, useRouter } from './lib/router'
import { useTheme, type Theme } from './lib/theme'
import { Palette } from './components/Palette'
import { Button, ErrorNote } from './components/ui'
import { Devices } from './routes/Devices'
import { Acl } from './routes/Acl'
import { Keys, People } from './routes/Admin'
import { Account } from './routes/Account'

export function App() {
  return (
    <Router>
      <Shell />
    </Router>
  )
}

function Shell() {
  const { path, navigate } = useRouter()
  const [theme, setTheme] = useTheme()

  // Set by the palette when someone picks a machine, consumed by Devices to
  // open that drawer. A query parameter would be nicer, but the router tracks
  // pathname only and widening it is not worth one hand-off.
  const [focusDevice, setFocusDevice] = useState<number | null>(null)

  const me = useQuery({
    queryKey: ['me'],
    queryFn: api.me,
    // A 401 means "not signed in", which is a state rather than a failure
    // worth retrying.
    retry: (_, error) => !(error instanceof ApiError && error.status === 401),
  })

  useLiveUpdates(me.data !== undefined)

  if (me.isPending) return <Centered>Loading…</Centered>
  if (me.error) return <Login error={me.error} />

  const user = me.data
  const known = ['/', '/devices', '/acl', '/people', '/keys', '/account']

  return (
    <div className="min-h-dvh">
      <Nav me={user} path={path} theme={theme} setTheme={setTheme} />
      <Palette
        me={user}
        theme={theme}
        setTheme={setTheme}
        onDevice={(id) => {
          setFocusDevice(id)
          navigate('/devices')
        }}
      />
      <main className="mx-auto w-full max-w-6xl px-5 py-6">
        <VersionBanner />
        {path === '/acl' && <Acl />}
        {path === '/people' && <People me={user} />}
        {path === '/keys' && <Keys me={user} />}
        {path === '/account' && <Account me={user} />}
        {(path === '/' || path === '/devices') && (
          <Devices me={user} focus={focusDevice} onFocused={() => setFocusDevice(null)} />
        )}
        {!known.includes(path) && <p className="text-sm text-muted-foreground">Nothing here.</p>}
      </main>
    </div>
  )
}

/**
 * The tailnet stream says *that* something changed, never what.
 *
 * Refetching the queries actually on screen keeps a member's connection from
 * carrying the whole tailnet, and keeps every scoping decision on the server
 * where it can be enforced.
 */
function useLiveUpdates(enabled: boolean) {
  const qc = useQueryClient()

  useEffect(() => {
    if (!enabled) return

    const source = new EventSource('/api/events')

    source.addEventListener('tailnet', () => {
      void qc.invalidateQueries({ queryKey: ['devices'] })
      void qc.invalidateQueries({ queryKey: ['device'] })
      void qc.invalidateQueries({ queryKey: ['device-rules'] })
      void qc.invalidateQueries({ queryKey: ['tailnet-users'] })
    })

    return () => source.close()
  }, [enabled, qc])
}

function Nav({
  me,
  path,
  theme,
  setTheme,
}: {
  me: Me
  path: string
  theme: Theme
  setTheme: (t: Theme) => void
}) {
  const can = (c: string) => me.capabilities.includes(c)

  const links = [
    { to: '/devices', label: can('view:all') ? 'Devices' : 'My devices', show: true },
    { to: '/acl', label: 'Access control', show: can('manage:policy') },
    { to: '/people', label: 'People', show: can('manage:users') },
    { to: '/keys', label: 'Keys', show: true },
  ].filter((l) => l.show)

  const active = path === '/' ? '/devices' : path

  return (
    <header className="border-b border-border bg-surface-1">
      <div className="mx-auto flex w-full max-w-6xl flex-wrap items-center gap-x-6 gap-y-2 px-5 py-3">
        <Link to="/devices" className="text-base font-semibold tracking-tight">
          Headboard
        </Link>

        <nav className="flex flex-1 gap-1">
          {links.map((l) => (
            <Link
              key={l.to}
              to={l.to}
              className={
                active === l.to
                  ? 'rounded-md bg-surface-2 px-2.5 py-1.5 text-sm font-medium'
                  : 'rounded-md px-2.5 py-1.5 text-sm text-muted-foreground hover:bg-surface-2'
              }
            >
              {l.label}
            </Link>
          ))}
        </nav>

        <div className="flex items-center gap-3 text-sm">
          <button
            type="button"
            aria-label={theme === 'dark' ? 'Switch to the light theme' : 'Switch to the dark theme'}
            title="⌘K for everything else"
            onClick={() => setTheme(theme === 'dark' ? 'light' : 'dark')}
            className="rounded-md px-2 py-1 text-muted-foreground hover:bg-surface-2"
          >
            {theme === 'dark' ? '☀' : '☾'}
          </button>
          <Link to="/account" className="text-muted-foreground hover:text-foreground">
            {me.user.email}
          </Link>
          <span className="rounded bg-surface-2 px-1.5 py-0.5 font-mono text-xs">{me.user.role}</span>
          <form method="post" action="/auth/logout">
            <Button type="submit" variant="ghost">
              Sign out
            </Button>
          </form>
        </div>
      </div>
    </header>
  )
}

/**
 * Headboard compiles Headscale's policy engine in, so a version mismatch means
 * the rules shown here are computed by a different engine than the one
 * enforcing traffic. That deserves a banner, not a log line.
 */
function VersionBanner() {
  const health = useQuery({ queryKey: ['health'], queryFn: api.health })

  if (!health.data || health.data.headscaleVersionMatch) return null

  return (
    <div className="mb-4 rounded-md border border-warn/40 bg-warn/10 px-3 py-2 text-sm">
      This Headscale runs <code>{health.data.headscaleServerVersion}</code>, but Headboard was built
      against <code>{health.data.headscaleVersion}</code>. Effective rules may not match what the
      server actually enforces.
    </div>
  )
}

function Login({ error }: { error: unknown }) {
  const qc = useQueryClient()
  const status = useQuery({ queryKey: ['auth-status'], queryFn: api.authStatus })

  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')

  const signIn = useMutation({
    mutationFn: () => api.login(email, password),
    // Everything on screen was rendered for an anonymous browser, so the
    // simplest correct thing after a login is to start again.
    onSuccess: () => qc.invalidateQueries(),
  })

  const unauthenticated = error instanceof ApiError && error.status === 401

  return (
    <Centered>
      <div className="w-full max-w-sm space-y-4 rounded-lg border border-border bg-surface-1 p-6">
        <div>
          <h1 className="text-lg font-semibold">Headboard</h1>
          <p className="mt-1 text-sm text-muted-foreground">A control plane for Headscale.</p>
        </div>

        {!unauthenticated && <ErrorNote error={error} />}
        {signIn.error && <ErrorNote error={signIn.error} />}

        <form
          className="space-y-3"
          onSubmit={(e) => {
            e.preventDefault()
            signIn.mutate()
          }}
        >
          <label className="block space-y-1">
            <span className="text-xs text-muted-foreground">Email</span>
            <input
              type="email"
              autoComplete="username"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              className="w-full rounded-md border border-border bg-surface-0 px-2.5 py-1.5 text-sm"
            />
          </label>

          <label className="block space-y-1">
            <span className="text-xs text-muted-foreground">Password</span>
            <input
              type="password"
              autoComplete="current-password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              className="w-full rounded-md border border-border bg-surface-0 px-2.5 py-1.5 text-sm"
            />
          </label>

          <button
            type="submit"
            disabled={signIn.isPending || email === '' || password === ''}
            className="w-full rounded-md bg-accent-500 px-3 py-2 text-sm font-medium text-white hover:bg-accent-600 disabled:opacity-50"
          >
            {signIn.isPending ? 'Signing in…' : 'Sign in'}
          </button>
        </form>

        {status.data?.oidcEnabled && (
          <>
            <div className="flex items-center gap-3 text-xs text-muted-foreground">
              <span className="h-px flex-1 bg-border" />
              or
              <span className="h-px flex-1 bg-border" />
            </div>

            <a
              href={status.data.loginUrl ?? '/auth/oidc'}
              className="block rounded-md border border-border px-3 py-2 text-center text-sm font-medium hover:bg-surface-2"
            >
              Sign in with your identity provider
            </a>
            <p className="text-xs text-muted-foreground">
              via <code>{status.data.issuer}</code>
            </p>
          </>
        )}
      </div>
    </Centered>
  )
}

function Centered({ children }: { children: React.ReactNode }) {
  return <div className="grid min-h-dvh place-items-center p-6">{children}</div>
}
