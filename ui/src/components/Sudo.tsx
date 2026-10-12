import { useEffect, useRef, useState, type FormEvent } from 'react'
import { createPortal } from 'react-dom'
import { useQueryClient } from '@tanstack/react-query'
import { LockKeyhole, LogOut, ShieldCheck, X } from 'lucide-react'
import { api, ApiError, setSudoPrompt, type Me } from '../lib/api'
import { withBase } from '../lib/basePath'
import { cn } from '../lib/cn'
import { useModal } from '../lib/modal'
import { countdown, eraseDigit, pinProblem, typeDigit } from '../lib/sudo'
import { Button, IconButton } from './ui'

/**
 * Six boxes, one digit each, like a one-time-code form. Digits show as dots:
 * this is a secret that sits on screen while someone may be looking. Typing
 * moves to the next box, Backspace steps back, a paste fills them all, and
 * onComplete fires when the sixth digit lands.
 */
export function PinField({
  label,
  value,
  onChange,
  autoFocus,
  onComplete,
  center,
}: {
  label: string
  value: string
  onChange: (v: string) => void
  autoFocus?: boolean
  onComplete?: (pin: string) => void
  center?: boolean
}) {
  const boxes = useRef<(HTMLInputElement | null)[]>([])
  // Focus moves before React re-renders, so the focus guard below must read
  // the PIN as just typed, not as of the last render.
  const latest = useRef(value)
  latest.current = value
  const focus = (i: number) => boxes.current[i]?.focus()
  const apply = (next: { pin: string; focus: number }) => {
    const before = latest.current
    latest.current = next.pin
    onChange(next.pin)
    focus(next.focus)
    if (next.pin.length === 6 && next.pin !== before) onComplete?.(next.pin)
  }

  return (
    <div role="group" aria-label={label} className={cn('space-y-1.5', center && 'text-center')}>
      <span className="block text-sm text-muted-foreground">{label}</span>
      <div className={cn('flex gap-2', center && 'justify-center')}>
        {Array.from({ length: 6 }, (_, i) => (
          <input
            key={i}
            ref={(el) => {
              boxes.current[i] = el
            }}
            type="password"
            inputMode="numeric"
            autoComplete="off"
            aria-label={`${label}, digit ${i + 1}`}
            value={value[i] ?? ''}
            // eslint-disable-next-line jsx-a11y/no-autofocus
            autoFocus={autoFocus && i === 0}
            onFocus={(e) => {
              // A gap is not allowed: clicking past the end lands on the first empty box.
              if (i > latest.current.length) focus(latest.current.length)
              else e.target.select()
            }}
            onChange={(e) => apply(typeDigit(value, i, e.target.value.replace(value[i] ?? '', '') || e.target.value))}
            onPaste={(e) => {
              e.preventDefault()
              apply(typeDigit(value, i, e.clipboardData.getData('text')))
            }}
            onKeyDown={(e) => {
              if (e.key === 'Backspace') {
                e.preventDefault()
                apply(eraseDigit(value, i))
              } else if (e.key === 'ArrowLeft') focus(Math.max(i - 1, 0))
              else if (e.key === 'ArrowRight') focus(Math.min(i + 1, value.length, 5))
            }}
            className="size-11 rounded-lg border border-border bg-surface-1 text-center font-mono text-xl shadow-card transition-colors focus:border-accent-500 focus:outline-none focus:ring-2 focus:ring-accent-500/25 sm:size-12"
          />
        ))}
      </div>
    </div>
  )
}

const message = (err: unknown) => (err instanceof Error ? err.message : 'Something went wrong')

/**
 * Answers the API client's "this needs sudo": opens a PIN dialog and resolves
 * true once sudo is on, false if the person cancels. Mounted once in the shell.
 */
export function SudoPromptHost() {
  const qc = useQueryClient()
  const [open, setOpen] = useState(false)
  const [pin, setPin] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const pending = useRef<{ promise: Promise<boolean>; resolve: (ok: boolean) => void } | null>(null)
  const dialog = useRef<HTMLFormElement>(null)

  useEffect(() => {
    setSudoPrompt(() => {
      // Two refused requests at once share one prompt.
      if (pending.current) return pending.current.promise

      let resolve!: (ok: boolean) => void
      const promise = new Promise<boolean>((r) => (resolve = r))
      pending.current = { promise, resolve }
      setPin('')
      setError(null)
      setOpen(true)

      return promise
    })

    return () => {
      setSudoPrompt(null)
      // The shell is going away (signed out, session ended): whatever was
      // waiting for a PIN gives up instead of hanging.
      pending.current?.resolve(false)
      pending.current = null
    }
  }, [])

  const close = (ok: boolean) => {
    setOpen(false)
    pending.current?.resolve(ok)
    pending.current = null
  }

  useModal(open, dialog, () => close(false), 'first')

  const submit = async (e?: FormEvent, typed = pin) => {
    e?.preventDefault()
    if (typed.length !== 6 || busy) return

    setBusy(true)
    try {
      await api.startSudo(typed)
      await qc.invalidateQueries({ queryKey: ['me'] })
      close(true)
    } catch (err) {
      setPin('')
      setError(message(err))
    } finally {
      setBusy(false)
    }
  }

  if (!open) return null

  return createPortal(
    <div className="fixed inset-0 z-[60] flex items-end justify-center p-4 sm:items-center">
      <button type="button" aria-label="Cancel" tabIndex={-1} className="absolute inset-0 bg-black/50 animate-fade-in" onClick={() => close(false)} />
      <form
        ref={dialog}
        role="dialog"
        aria-modal="true"
        aria-label="Confirm with your PIN"
        onSubmit={submit}
        className="relative z-50 w-full max-w-sm space-y-5 rounded-2xl border border-border bg-surface-0 px-6 pb-[max(1.5rem,env(safe-area-inset-bottom))] pt-7 text-center shadow-raised animate-rise"
      >
        <div>
          <h3 className="text-display font-semibold">Confirm it's you</h3>
          <p className="mt-1.5 text-sm text-muted-foreground">Enter your sudo PIN. It stays on for 5 minutes.</p>
        </div>
        <PinField label="Sudo PIN" value={pin} onChange={setPin} autoFocus center onComplete={(typed) => void submit(undefined, typed)} />
        {error && <p role="alert" className="text-sm text-danger">{error}</p>}
        <Button type="submit" variant="primary" disabled={pin.length !== 6 || busy} className="min-w-32">
          {busy ? 'Checking…' : 'Confirm'}
        </Button>
        <p className="text-xs text-muted-foreground">Forgot it? An admin with access to the server can clear it.</p>
        {/* Last in the DOM so the modal's first-control focus lands on the PIN, not here. */}
        <IconButton icon={X} label="Cancel" variant="ghost" onClick={() => close(false)} className="absolute right-2 top-2" />
      </form>
    </div>,
    document.body,
  )
}

/** "Sudo 4:12" with a way to end it early. Renders nothing while sudo is off. */
export function SudoChip({ until, onRail }: { until?: string; onRail?: boolean }) {
  const qc = useQueryClient()
  const [now, setNow] = useState(() => new Date())

  useEffect(() => {
    if (!until) return
    const t = setInterval(() => setNow(new Date()), 1000)

    return () => clearInterval(t)
  }, [until])

  const left = until ? countdown(until, now) : null
  if (!left) return null

  const end = async () => {
    await api.endSudo().catch(() => undefined)
    await qc.invalidateQueries({ queryKey: ['me'] })
  }

  return (
    <span
      className={cn(
        'inline-flex h-8 shrink-0 items-center gap-1.5 whitespace-nowrap rounded-full border pl-2.5 text-xs font-medium',
        onRail ? 'border-accent-400/40 bg-accent-500/15 pr-0.5 text-accent-400' : 'border-accent-500/40 bg-accent-500/10 pr-1 text-accent-700 dark:text-accent-400',
      )}
      title="Dangerous actions do not ask for your PIN until this runs out"
    >
      <ShieldCheck aria-hidden className="size-3.5" strokeWidth={1.75} />
      {/* The phone header has no room for words; the labels carry them. */}
      <span aria-label={`Sudo on for ${left}`}>
        {!onRail && 'Sudo '}
        <span className="font-mono tabular-nums">{left}</span>
      </span>
      {onRail ? (
        <button type="button" aria-label="End sudo" title="End sudo" onClick={() => void end()} className="relative grid size-7 place-items-center rounded-full after:absolute after:-inset-1.5 hover:bg-accent-500/20">
          <X aria-hidden className="size-3.5" strokeWidth={2} />
        </button>
      ) : (
        <button type="button" onClick={() => void end()} className="rounded-full px-2 py-0.5 hover:bg-accent-500/20">
          End
        </button>
      )}
    </span>
  )
}

/**
 * Holds an admin without a PIN here until they set one. The server refuses
 * every other request meanwhile, so this is the only way forward.
 */
export function PinSetupScreen({ me }: { me: Me }) {
  const qc = useQueryClient()
  const [password, setPassword] = useState('')
  const [pin, setPin] = useState('')
  const [confirm, setConfirm] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const problem = confirm.length === 6 ? pinProblem(pin, confirm) : pin.length === 6 ? pinProblem(pin, pin) : null
  const ready = pin.length === 6 && confirm.length === 6 && !problem && (!me.local || password !== '')
  const identityFirst = !me.local && !me.reauthFresh

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    if (!ready || busy) return

    setBusy(true)
    setError(null)
    try {
      await api.setPin(pin, me.local ? password : undefined)
      await qc.invalidateQueries({ queryKey: ['me'] })
    } catch (err) {
      setError(err instanceof ApiError && err.code === 'reauth_required' ? 'Your confirmation expired. Confirm with your identity provider again.' : message(err))
      void qc.invalidateQueries({ queryKey: ['me'] })
    } finally {
      setBusy(false)
    }
  }

  return (
    <main className="grid min-h-dvh place-items-center p-4">
      <section className="w-full max-w-md space-y-6 rounded-2xl border border-border bg-surface-1 px-6 py-8 text-center shadow-card">
        <div>
          <span aria-hidden className="mx-auto mb-3 grid size-11 place-items-center rounded-full bg-accent-500/12 text-accent-700 dark:text-accent-400">
            <LockKeyhole className="size-5" strokeWidth={1.6} />
          </span>
          <h1 className="text-display font-semibold">Set your sudo PIN</h1>
          <p className="mx-auto mt-2 max-w-sm text-sm text-muted-foreground">
            Your role can change the network, so Headboard asks for this 6-digit PIN before dangerous actions like
            removing devices or changing roles, the policy, keys and SSH access. Once entered, it stays on for 5 minutes.
          </p>
        </div>

        {identityFirst ? (
          <div className="space-y-3">
            <p className="text-sm">First, confirm it is you by signing in again at your identity provider.</p>
            <Button variant="primary" onClick={() => window.location.assign(withBase('/auth/oidc?reauth=1&return_to=/'))}>
              Confirm with your identity provider
            </Button>
          </div>
        ) : (
          <form onSubmit={submit} className="space-y-4">
            {me.local && (
              <label className="mx-auto block max-w-xs space-y-1.5 text-left">
                <span className="text-sm text-muted-foreground">Your password</span>
                <input
                  type="password"
                  autoComplete="current-password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  className="h-10 w-full rounded-lg border border-border bg-surface-1 px-3 text-sm focus:border-accent-500 focus:outline-none"
                />
              </label>
            )}
            <PinField label="New PIN" value={pin} onChange={setPin} autoFocus={!me.local} center />
            <PinField label="Same PIN again" value={confirm} onChange={setConfirm} center />
            {(error || problem) && <p role="alert" className="text-sm text-danger">{error ?? problem}</p>}
            <Button type="submit" variant="primary" disabled={!ready || busy} className="min-w-40">
              {busy ? 'Saving…' : 'Save PIN'}
            </Button>
          </form>
        )}

        <div className="flex items-center justify-between border-t border-border pt-4 text-left text-xs text-muted-foreground">
          <span>Signed in as {me.user.email}</span>
          <form method="post" action={withBase('/auth/logout')}>
            <Button type="submit" variant="ghost" icon={LogOut}>Sign out</Button>
          </form>
        </div>
      </section>
    </main>
  )
}
