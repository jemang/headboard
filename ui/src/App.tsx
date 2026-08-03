import { useEffect, useState } from 'react'
import { withBase } from './lib/basePath'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, ApiError, type Health, type Me } from './lib/api'
import { connectionPresentation } from './lib/headscaleConnection'
import { admissionPresentation } from './lib/accountAdmission'
import { Link, Router, useRouter } from './lib/router'
import { useTheme, type Theme } from './lib/theme'
import { Palette } from './components/Palette'
import { Button, ErrorNote } from './components/ui'
import { ToastHost } from './components/Toast'
import { CircleHelp, KeyRound, Laptop, LogOut, Menu, Moon, ShieldCheck, Sun, Users, X } from 'lucide-react'
import { Devices } from './routes/Devices'
import { Acl } from './routes/Acl'
import { Keys, People } from './routes/Admin'
import { Account } from './routes/Account'
import { Help } from './routes/Help'
import { BrandMark } from './components/BrandMark'

export function App() {
  return (
    <Router>
      <ToastHost>
        <Shell />
      </ToastHost>
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
  const health = useQuery({
    queryKey: ['health'],
    queryFn: api.health,
    refetchInterval: 5000,
    enabled: me.data?.user.admission === 'active',
  })

  useLiveUpdates(me.data?.user.admission === 'active')

  if (me.isPending) return <Centered>Loading…</Centered>
  if (me.error) return <Login error={me.error} />

  const user = me.data
  const admission = admissionPresentation(user.user.admission)

  if (!admission.canUseApp) return <AdmissionScreen email={user.user.email} title={admission.title} detail={admission.detail} />

  const known = ['/', '/devices', '/acl', '/people', '/keys', '/help', '/account']

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
      <main className="min-h-dvh lg:pl-64">
        <div className="border-b border-border bg-surface-0/75 px-4 py-3 backdrop-blur sm:px-6 lg:px-8">
          <div className="mx-auto flex max-w-7xl items-center gap-3">
            <span className="text-eyebrow font-semibold uppercase text-muted-foreground">Tailnet control plane</span>
            <span className="hidden h-4 w-px bg-border sm:block" />
            <HeadscaleConnectionIndicator health={health.data} unavailable={health.isError} />
            <kbd className="ml-auto hidden items-center gap-1 rounded-md border border-border bg-surface-1 px-2 py-1 font-mono text-[0.6875rem] text-muted-foreground md:inline-flex">
              <span>⌘</span>K to search
            </kbd>
          </div>
        </div>
        <div className="mx-auto w-full max-w-7xl px-4 py-6 sm:px-6 sm:py-8 lg:px-8">
          <VersionBanner />
          {path === '/acl' && <Acl />}
          {path === '/people' && <People me={user} />}
          {path === '/keys' && <Keys me={user} />}
          {path === '/help' && (
            user.capabilities.includes('manage:devices')
              ? <Help />
              : <p className="text-sm text-muted-foreground">You do not have access to the operator guide.</p>
          )}
          {path === '/account' && <Account me={user} />}
          {(path === '/' || path === '/devices') && (
            <Devices
              me={user}
              focus={focusDevice}
              onFocused={() => setFocusDevice(null)}
            />
          )}
          {!known.includes(path) && <p className="text-sm text-muted-foreground">Nothing here.</p>}
        </div>
      </main>
    </div>
  )
}

function AdmissionScreen({ email, title, detail }: { email: string; title: string; detail: string }) {
  return (
    <main className="grid min-h-dvh place-items-center bg-surface-0 p-6">
      <section className="w-full max-w-md rounded-xl border border-border bg-surface-1 p-6 shadow-sm">
        <p className="text-eyebrow font-semibold uppercase text-muted-foreground">Headboard access</p>
        <h1 className="mt-2 text-display font-semibold">{title}</h1>
        <p className="mt-3 text-sm text-muted-foreground">{detail}</p>
        <p className="mt-5 text-xs text-muted-foreground">Signed in as {email}</p>
        <form method="post" action={withBase('/auth/logout')} className="mt-5"><Button type="submit" variant="ghost" icon={LogOut}>Sign out</Button></form>
      </section>
    </main>
  )
}

function HeadscaleConnectionIndicator({ health, unavailable }: { health?: Health; unavailable: boolean }) {
  const presentation = connectionPresentation(unavailable ? { headscaleState: 'unavailable' } : health)
  const tone = {
    connected: 'bg-ok',
    stale: 'bg-warn',
    unavailable: 'bg-danger',
    checking: 'bg-muted-foreground/40',
  }[presentation.tone]

  return (
    <span className="inline-flex items-center gap-1.5" title={presentation.title}>
      <span aria-hidden className={`size-2 rounded-full ${tone}`} />
      <span className="hidden text-xs text-muted-foreground sm:block">{presentation.label}</span>
    </span>
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

    const source = new EventSource(withBase('/api/events'))

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
  const [menuOpen, setMenuOpen] = useState(false)

  const links = [
    { to: '/devices', label: can('view:all') ? 'Devices' : 'My devices', icon: Laptop, show: true },
    { to: '/acl', label: 'Access control', icon: ShieldCheck, show: can('manage:policy') },
    { to: '/people', label: 'People', icon: Users, show: can('manage:users') },
    { to: '/keys', label: 'Keys', icon: KeyRound, show: true },
    { to: '/help', label: 'Help', icon: CircleHelp, show: can('manage:devices') },
  ].filter((l) => l.show)

  const active = path === '/' ? '/devices' : path

  const rail = (compact = false) => (
    <nav className={compact ? 'space-y-1' : 'space-y-1'} aria-label="Primary navigation">
      <p className={compact ? 'sr-only' : 'px-3 pb-2 pt-2 text-eyebrow font-semibold uppercase text-muted-foreground'}>
        Tailnet
      </p>
      {links.map((l) => (
        <Link
          key={l.to}
          to={l.to}
          onClick={() => setMenuOpen(false)}
          className={
            active === l.to
              ? 'flex items-center gap-3 rounded-lg bg-accent-500/15 px-3 py-2.5 text-sm font-semibold text-accent-700 dark:text-accent-400'
              : 'flex items-center gap-3 rounded-lg px-3 py-2.5 text-sm text-muted-foreground transition-colors hover:bg-surface-2 hover:text-foreground'
          }
        >
          <l.icon aria-hidden className="size-4.5 shrink-0" strokeWidth={1.7} />
          <span className={compact ? 'sr-only' : ''}>{l.label}</span>
        </Link>
      ))}
    </nav>
  )

  return (
    <>
      <aside className="fixed inset-y-0 left-0 z-30 hidden w-64 flex-col border-r border-border bg-surface-1/95 px-3 py-5 backdrop-blur lg:flex">
        <Link to="/devices" className="mb-8 flex items-center gap-3 px-3">
          <span className="grid size-8 place-items-center rounded-lg bg-accent-500 text-slate-950 shadow-sm"><BrandMark className="size-5" /></span>
          <span>
            <span className="block text-base font-semibold tracking-tight">Headboard</span>
            <span className="block text-[0.6875rem] text-muted-foreground">Headscale control plane</span>
          </span>
        </Link>
        {rail()}
        <div className="mt-auto space-y-3 border-t border-border px-3 pt-4">
          <Link to="/account" className="block rounded-lg p-1 transition-colors hover:bg-surface-2">
            <span className="block truncate text-sm font-medium">{me.user.email}</span>
            <span className="font-mono text-xs text-muted-foreground">{me.user.role}</span>
          </Link>
          <div className="flex items-center gap-1">
            <button
              type="button"
              aria-label={theme === 'dark' ? 'Switch to the light theme' : 'Switch to the dark theme'}
              onClick={() => setTheme(theme === 'dark' ? 'light' : 'dark')}
              className="rounded-lg p-2 text-muted-foreground transition-colors hover:bg-surface-2 hover:text-foreground"
            >
              {theme === 'dark' ? <Sun aria-hidden className="size-4" strokeWidth={1.5} /> : <Moon aria-hidden className="size-4" strokeWidth={1.5} />}
            </button>
            <form method="post" action={withBase('/auth/logout')}>
              <Button type="submit" variant="ghost" icon={LogOut} title="Sign out" className="px-2">
                <span className="sr-only">Sign out</span>
              </Button>
            </form>
          </div>
        </div>
      </aside>

      <header className="sticky top-0 z-30 flex items-center border-b border-border bg-surface-1/95 px-4 py-3 backdrop-blur lg:hidden">
        <Link to="/devices" className="flex items-center gap-2 font-semibold tracking-tight">
          <span className="grid size-7 place-items-center rounded-md bg-accent-500 text-slate-950"><BrandMark className="size-4" /></span>
          Headboard
        </Link>
        <button
          type="button"
          aria-expanded={menuOpen}
          aria-label={menuOpen ? 'Close navigation' : 'Open navigation'}
          onClick={() => setMenuOpen((open) => !open)}
          className="ml-auto rounded-lg p-2 text-muted-foreground hover:bg-surface-2 hover:text-foreground"
        >
          {menuOpen ? <X aria-hidden className="size-5" /> : <Menu aria-hidden className="size-5" />}
        </button>
      </header>
      {menuOpen && (
        <div className="fixed inset-x-0 bottom-0 top-[53px] z-20 border-b border-border bg-surface-0/98 p-4 shadow-raised backdrop-blur lg:hidden animate-fade-in">
          {rail()}
          <div className="mt-5 border-t border-border pt-4">
            <Link to="/account" onClick={() => setMenuOpen(false)} className="block rounded-lg p-3 hover:bg-surface-2">
              <span className="block text-sm font-medium">{me.user.email}</span>
              <span className="font-mono text-xs text-muted-foreground">{me.user.role}</span>
            </Link>
            <div className="mt-2 flex gap-2">
              <Button variant="default" onClick={() => setTheme(theme === 'dark' ? 'light' : 'dark')} icon={theme === 'dark' ? Sun : Moon}>
                {theme === 'dark' ? 'Light theme' : 'Dark theme'}
              </Button>
              <form method="post" action={withBase('/auth/logout')}><Button type="submit" variant="ghost" icon={LogOut}>Sign out</Button></form>
            </div>
          </div>
        </div>
      )}
    </>
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
    <div className="mb-6 rounded-xl border border-warn/40 bg-warn/10 px-4 py-3 text-sm shadow-sm">
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
      <div className="grid w-full max-w-4xl overflow-hidden rounded-2xl border border-border bg-surface-1 shadow-raised md:grid-cols-[1.05fr_.95fr]">
        <div className="hidden flex-col justify-between bg-slate-950 p-8 text-slate-100 md:flex">
          <div>
            <span className="grid size-10 place-items-center rounded-xl bg-accent-500 text-slate-950"><BrandMark className="size-6" /></span>
            <p className="mt-12 text-eyebrow font-semibold uppercase tracking-[0.14em] text-accent-400">Headscale control plane</p>
            <h1 className="mt-3 max-w-sm text-3xl font-semibold tracking-tight">Your tailnet, made legible.</h1>
            <p className="mt-4 max-w-sm text-sm leading-6 text-slate-300">Manage devices, policy, people, and keys with the same rules Headscale enforces.</p>
          </div>
          <div className="border-t border-slate-700 pt-5 text-sm text-slate-400">Built for operators and members.</div>
        </div>

        <div className="space-y-5 p-6 sm:p-8">
          <div className="flex items-center gap-2.5 md:hidden">
            <span className="grid size-9 shrink-0 place-items-center rounded-lg bg-accent-500 text-slate-950"><BrandMark className="size-5" /></span>
            <div>
              <h1 className="text-display font-semibold leading-none">Headboard</h1>
              <p className="mt-1 text-sm text-muted-foreground">A control plane for Headscale.</p>
            </div>
          </div>
          <div className="hidden md:block">
            <p className="text-eyebrow font-semibold uppercase text-muted-foreground">Welcome back</p>
            <h2 className="mt-1 text-display font-semibold">Sign in to Headboard</h2>
          </div>

        {!unauthenticated && <ErrorNote error={error} />}
        {signIn.error && <ErrorNote error={signIn.error} />}

        <form
          className="space-y-4"
          onSubmit={(e) => {
            e.preventDefault()
            signIn.mutate()
          }}
        >
          <label className="block space-y-1">
            <span className="text-xs font-medium text-muted-foreground">Email</span>
            <input
              type="email"
              autoComplete="username"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              className="w-full rounded-lg border border-border bg-surface-0 px-3 py-2 text-sm"
            />
          </label>

          <label className="block space-y-1">
            <span className="text-xs font-medium text-muted-foreground">Password</span>
            <input
              type="password"
              autoComplete="current-password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              className="w-full rounded-lg border border-border bg-surface-0 px-3 py-2 text-sm"
            />
          </label>

          <button
            type="submit"
            disabled={signIn.isPending || email === '' || password === ''}
            className="btn h-auto min-h-0 w-full rounded-lg border-0 bg-accent-500 px-3 py-2.5 text-sm font-semibold text-slate-950 shadow-none hover:bg-accent-400 disabled:opacity-50"
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
              href={status.data.loginUrl ?? withBase('/auth/oidc')}
              className="block rounded-lg border border-border px-3 py-2.5 text-center text-sm font-medium transition-colors hover:border-accent-500/40 hover:bg-surface-2"
            >
              Sign in with your identity provider
            </a>
            <p className="text-xs text-muted-foreground">
              via <code>{status.data.issuer}</code>
            </p>
          </>
        )}
        </div>
      </div>
    </Centered>
  )
}

function Centered({ children }: { children: React.ReactNode }) {
  return <div className="grid min-h-dvh place-items-center p-6">{children}</div>
}
