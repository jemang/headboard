import { useEffect, useState } from 'react'
import { withBase } from './lib/basePath'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, ApiError } from './lib/api'
import { admissionPresentation } from './lib/accountAdmission'
import { Link, Router } from './lib/router'
import { useRouter } from './lib/routerContext'
import { navItems, resolveRoute } from './lib/routes'
import { useNeeds } from './lib/useNeeds'
import { useOffline } from './lib/connectivity'
import { registerServiceWorker } from './lib/pwa'
import { UpdateBar } from './components/UpdateBar'
import { Offline } from './components/Offline'
import { LG, useMediaQuery } from './lib/useMediaQuery'
import { Rail } from './components/shell/Rail'
import { TopBar } from './components/shell/TopBar'
import { BottomNav } from './components/shell/BottomNav'
import { MoreSheet } from './components/shell/MoreSheet'
import { HealthWord } from './components/shell/HealthWord'
import { useTheme } from './lib/theme'
import { Palette } from './components/Palette'
import { Button, ErrorNote } from './components/ui'
import { ToastHost } from './components/Toast'
import { LogOut, Search } from 'lucide-react'
import { Devices } from './routes/Devices'
import { Acl } from './routes/Acl'
import { Keys, People } from './routes/Admin'
import { Account } from './routes/Account'
import { Help } from './routes/Help'
import { Activity } from './routes/Activity'
import { PinSetupScreen, SudoChip, SudoPromptHost } from './components/Sudo'
import { SshRoute } from './routes/SshAccess'
import { BrandMark } from './components/BrandMark'

export function App() {
  const [applyUpdate, setApplyUpdate] = useState<(() => void) | null>(null)

  // In App, not Shell, so the sign-in and offline screens can offer it too.
  useEffect(() => registerServiceWorker((apply) => setApplyUpdate(() => apply)), [])

  return (
    <Router>
      <ToastHost>
        {applyUpdate && <UpdateBar onReload={applyUpdate} />}
        <Shell />
      </ToastHost>
    </Router>
  )
}

function Shell() {
  const { path, navigate } = useRouter()
  const [theme, setTheme] = useTheme()

  const [paletteOpen, setPaletteOpen] = useState(false)
  const [moreOpen, setMoreOpen] = useState(false)
  const wide = useMediaQuery(LG)

  const me = useQuery({
    queryKey: ['me'],
    queryFn: api.me,
    // A 401 means "not signed in", which is a state rather than a failure
    // worth retrying.
    retry: (_, error) => !(error instanceof ApiError && error.status === 401),
    // Every role notices a session that ended elsewhere within a minute, not
    // only the roles whose pages poll (lib/session.ts handles the 401).
    refetchInterval: 60_000,
  })
  const needs = useNeeds(me.data)
  const offline = useOffline(me.error ?? me.failureReason)
  const qc = useQueryClient()
  const health = needs.health

  useLiveUpdates(me.data?.user.admission === 'active')

  // Resolved before the early returns below so the redirect effect always
  // runs in the same hook order. /login after a successful sign-in, an
  // unknown path, or a page the role cannot open all land on Devices.
  const resolved = me.data ? resolveRoute(path, me.data.capabilities) : null
  const redirect = resolved && 'redirect' in resolved ? resolved.redirect : null
  const route = resolved && 'route' in resolved ? resolved.route : null

  useEffect(() => setMoreOpen(false), [path])

  useEffect(() => {
    if (redirect && me.data?.user.admission === 'active') navigate(redirect, { replace: true })
  }, [redirect, navigate, me.data?.user.admission])

  // Retry asks again; the offline latch clears only when a request actually
  // reaches the server.
  const retry = () => void qc.refetchQueries({ type: 'active' })

  // Without a signed-in account there is nothing to keep, so the offline screen
  // replaces the app. With one, it covers the app instead (see the overlay below).
  if (offline && !me.data) return <Offline onRetry={retry} />

  if (me.isPending) return <Centered>Loading…</Centered>
  if (me.error) return <Login error={me.error} />

  const user = me.data
  const admission = admissionPresentation(user.user.admission)

  if (!admission.canUseApp) return <AdmissionScreen email={user.user.email} title={admission.title} detail={admission.detail} />
  if (user.needsPin) return <PinSetupScreen me={user} />

  const active = route
  const nav = navItems(user.capabilities)
  const moreActive = active !== null && nav.more.some((i) => i.id === active)

  return (
    <div className="min-h-dvh">
      {wide ? (
        <Rail
          me={user}
          active={active}
          theme={theme}
          setTheme={setTheme}
          health={health.data}
          unavailable={health.isError}
          needCount={needs.items.length}
          showNeeds={route !== 'devices'}
        />
      ) : (
        <header className="sticky top-0 z-30 flex items-center gap-3 border-b border-rail-border bg-rail/95 px-4 pb-2 pt-[max(0.5rem,env(safe-area-inset-top))] backdrop-blur">
          <Link to="/devices" className="flex min-h-11 items-center gap-2 font-semibold tracking-tight text-rail-fg">
            <BrandMark className="size-7" />
            Headboard
          </Link>
          <HealthWord health={health.data} unavailable={health.isError} compact onRail />
          <SudoChip until={user.sudoUntil} onRail />
          <button
            type="button"
            aria-label="Search pages and devices"
            onClick={() => setPaletteOpen(true)}
            className="ml-auto grid size-11 place-items-center rounded-lg text-rail-muted hover:bg-white/10 hover:text-rail-fg"
          >
            <Search aria-hidden className="size-5" strokeWidth={1.5} />
          </button>
        </header>
      )}
      <SudoPromptHost />
      <Palette
        me={user}
        theme={theme}
        setTheme={setTheme}
        open={paletteOpen}
        onOpenChange={setPaletteOpen}
        onDevice={(id) => navigate(`/devices?device=${id}`)}
      />
      <main className="min-h-dvh pb-[calc(4rem+env(safe-area-inset-bottom))] lg:pb-0 lg:pl-64">
        {wide && (
          <TopBar onSearch={() => setPaletteOpen(true)} sudoUntil={user.sudoUntil} />
        )}
        <div className="mx-auto w-full max-w-7xl px-4 py-6 sm:px-6 sm:py-8 lg:px-8">
          {/* On Devices the needs list already carries a version mismatch. */}
          {route !== 'devices' && <VersionBanner />}
          {route === 'acl' && <Acl />}
          {route === 'people' && <People me={user} />}
          {route === 'ssh' && <SshRoute capabilities={user.capabilities} />}
          {route === 'keys' && <Keys me={user} />}
          {route === 'activity' && <Activity />}
          {route === 'help' && <Help />}
          {route === 'account' && <Account me={user} />}
          {route === 'devices' && (
<Devices me={user} needs={needs.items} />
          )}
        </div>
      </main>
      {offline && <Offline overlay onRetry={retry} />}
      {!wide && (
        <>
          <BottomNav items={nav.primary} active={active} moreActive={moreActive || moreOpen} onMore={() => setMoreOpen(true)} />
          <MoreSheet open={moreOpen} onClose={() => setMoreOpen(false)} items={nav.more} me={user} theme={theme} setTheme={setTheme} />
        </>
      )}
    </div>
  )
}

function AdmissionScreen({ email, title, detail }: { email: string; title: string; detail: string }) {
  return (
    <main className="grid min-h-dvh place-items-center bg-surface-0 p-6">
      <section className="w-full max-w-md rounded-xl border border-border bg-surface-1 p-6">
        <h1 className="text-display font-semibold">{title}</h1>
        <p className="mt-3 text-sm text-muted-foreground">{detail}</p>
        <p className="mt-5 text-xs text-muted-foreground">Signed in as {email}</p>
        <form method="post" action={withBase('/auth/logout')} className="mt-5"><Button type="submit" variant="ghost" icon={LogOut}>Sign out</Button></form>
      </section>
    </main>
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

/**
 * Headboard compiles Headscale's policy engine in, so a version mismatch means
 * the rules shown here are computed by a different engine than the one
 * enforcing traffic. That deserves a banner, not a log line.
 */
function VersionBanner() {
  const health = useQuery({ queryKey: ['health'], queryFn: api.health })

  if (!health.data || health.data.headscaleVersionMatch) return null

  return (
    <div className="mb-6 rounded-xl border border-warn/40 bg-warn/10 px-4 py-3 text-sm">
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
    onSuccess: () => qc.resetQueries(),
  })

  const unauthenticated = error instanceof ApiError && error.status === 401

  return (
    <Centered>
      <div className="grid w-full max-w-4xl overflow-hidden rounded-2xl border border-border bg-surface-1 shadow-raised md:grid-cols-[1.05fr_.95fr]">
        <div className="relative hidden flex-col justify-between border-r border-side-border bg-side-wash p-8 text-side-fg md:flex">
          <div>
            <BrandMark className="size-11" />
            <h1 className="mt-12 max-w-sm text-3xl font-semibold tracking-tight">Your tailnet, made legible.</h1>
            <p className="mt-4 max-w-sm text-sm leading-6 text-side-muted">Manage devices, policy, people, and keys with the same rules Headscale enforces.</p>
          </div>
          <div className="border-t border-side-border pt-5 text-sm text-side-muted">Built for operators and members.</div>
        </div>

        <div className="space-y-5 p-6 sm:p-8">
          <div className="flex items-center gap-2.5 md:hidden">
            <BrandMark className="size-9 shrink-0" />
            <div>
              <h1 className="text-display font-semibold leading-none">Headboard</h1>
              <p className="mt-1 text-sm text-muted-foreground">A control plane for Headscale.</p>
            </div>
          </div>
          <div className="hidden md:block">
            <h2 className="text-display font-semibold">Sign in to Headboard</h2>
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
              className="h-10 w-full rounded-lg border border-border bg-surface-0 px-3 text-sm"
            />
          </label>

          <label className="block space-y-1">
            <span className="text-xs font-medium text-muted-foreground">Password</span>
            <input
              type="password"
              autoComplete="current-password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              className="h-10 w-full rounded-lg border border-border bg-surface-0 px-3 text-sm"
            />
          </label>

          <button
            type="submit"
            disabled={signIn.isPending || email === '' || password === ''}
            className="btn h-10 min-h-0 w-full rounded-lg border-0 bg-accent-700 px-3 text-sm font-semibold text-white shadow-none hover:bg-accent-600 disabled:opacity-50 dark:bg-accent-500 dark:text-slate-950 dark:hover:bg-accent-400"
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
              className="flex h-10 items-center justify-center rounded-lg border border-border px-3 text-center text-sm font-medium transition-colors hover:border-accent-500/40 hover:bg-surface-2"
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
