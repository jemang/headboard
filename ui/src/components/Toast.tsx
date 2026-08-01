import { createContext, useCallback, useContext, useMemo, useState, type ReactNode } from 'react'
import { CheckCircle2, X, AlertTriangle } from 'lucide-react'
import { cn } from '../lib/cn'

type Tone = 'ok' | 'error'

interface Toast {
  id: number
  tone: Tone
  message: string
}

interface ToastApi {
  /** Confirm something happened. Dismisses itself. */
  ok: (message: string) => void

  /** Report a failure. Stays until dismissed — see below. */
  error: (error: unknown) => void
}

const ToastContext = createContext<ToastApi>({ ok: () => {}, error: () => {} })

/** Announce the outcome of an action. */
export function useToast() {
  return useContext(ToastContext)
}

/**
 * Toasts for the results of actions — saved, renamed, expired, failed.
 *
 * Only for outcomes. Anything a person has to *act* on stays inline where the
 * problem is: field validation, a policy that will not parse, the version
 * mismatch banner. A toast for a field error moves the message away from the
 * field it belongs to.
 *
 * Success messages dismiss themselves; failures do not. An error that
 * disappears after four seconds is an error someone can miss, and this console
 * changes who can reach what.
 */
export function ToastHost({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([])

  const dismiss = useCallback((id: number) => {
    setToasts((prev) => prev.filter((t) => t.id !== id))
  }, [])

  const push = useCallback(
    (tone: Tone, message: string) => {
      const id = Date.now() + Math.random()

      setToasts((prev) => [...prev, { id, tone, message }])

      if (tone === 'ok') setTimeout(() => dismiss(id), 4000)
    },
    [dismiss],
  )

  const api = useMemo<ToastApi>(
    () => ({
      ok: (message) => push('ok', message),
      error: (error) => push('error', error instanceof Error ? error.message : String(error)),
    }),
    [push],
  )

  return (
    <ToastContext value={api}>
      {children}

      <div
        // Announced to assistive tech, but never focus-stealing.
        role="status"
        aria-live="polite"
        className="pointer-events-none fixed inset-x-0 bottom-0 z-[60] flex flex-col items-center gap-2 p-4 sm:items-end"
      >
        {toasts.map((t) => (
          <div
            key={t.id}
            className={cn(
              'pointer-events-auto flex w-full max-w-sm items-start gap-2.5 rounded-xl border px-3 py-2.5 text-sm shadow-raised animate-rise',
              t.tone === 'ok'
                ? 'border-ok/40 bg-surface-1 text-foreground'
                : 'border-danger/50 bg-surface-1 text-foreground',
            )}
          >
            {t.tone === 'ok' ? (
              <CheckCircle2 aria-hidden className="mt-0.5 size-4 shrink-0 text-ok" strokeWidth={1.5} />
            ) : (
              <AlertTriangle aria-hidden className="mt-0.5 size-4 shrink-0 text-danger" strokeWidth={1.5} />
            )}

            <span className="flex-1 break-words">{t.message}</span>

            <button
              type="button"
              aria-label="Dismiss"
              onClick={() => dismiss(t.id)}
              className="rounded p-0.5 text-muted-foreground hover:bg-surface-2 hover:text-foreground"
            >
              <X aria-hidden className="size-3.5" strokeWidth={1.5} />
            </button>
          </div>
        ))}
      </div>
    </ToastContext>
  )
}
