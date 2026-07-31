import { useEffect, useState, type ReactNode } from 'react'
import { clsx } from 'clsx'

/**
 * Status is a dot *plus* a label. Colour never carries meaning on its own.
 *
 * `bad` is opt-in because most "not ok" states here are merely absent — a
 * device that is offline, an identity not yet linked — and those read better
 * grey than red. A failed assertion is the other kind, and says so.
 */
export function Status({
  ok,
  label,
  warn,
  bad,
}: {
  ok: boolean
  label: string
  warn?: boolean
  bad?: boolean
}) {
  return (
    <span className="inline-flex items-center gap-1.5 whitespace-nowrap">
      <span
        aria-hidden
        className={clsx(
          'size-2 rounded-full',
          bad ? 'bg-danger' : warn ? 'bg-warn' : ok ? 'bg-ok' : 'bg-muted-foreground/40',
        )}
      />
      <span className={clsx('text-sm', bad && 'text-danger', !ok && !warn && !bad && 'text-muted-foreground')}>
        {label}
      </span>
    </span>
  )
}

export function Badge({
  children,
  tone = 'neutral',
}: {
  children: ReactNode
  tone?: 'neutral' | 'accent' | 'warn' | 'danger'
}) {
  return (
    <span
      className={clsx(
        'inline-flex items-center rounded px-1.5 py-0.5 font-mono text-xs',
        tone === 'neutral' && 'bg-surface-2 text-muted-foreground',
        tone === 'accent' && 'bg-accent-500/15 text-accent-700 dark:text-accent-400',
        tone === 'warn' && 'bg-warn/15 text-warn',
        tone === 'danger' && 'bg-danger/15 text-danger',
      )}
    >
      {children}
    </span>
  )
}

/** Technical values are monospace and copy on click. */
export function Mono({ value, className }: { value: string; className?: string }) {
  const [copied, setCopied] = useState(false)

  useEffect(() => {
    if (!copied) return

    const t = setTimeout(() => setCopied(false), 1200)

    return () => clearTimeout(t)
  }, [copied])

  return (
    <button
      type="button"
      title="Copy"
      onClick={() => {
        void navigator.clipboard.writeText(value).then(() => setCopied(true))
      }}
      className={clsx(
        'rounded px-1 font-mono text-xs hover:bg-surface-2',
        copied && 'bg-ok/20',
        className,
      )}
    >
      {copied ? 'copied' : value}
    </button>
  )
}

export function Button({
  children,
  onClick,
  variant = 'default',
  disabled,
  type = 'button',
  className,
}: {
  children: ReactNode
  onClick?: () => void
  variant?: 'default' | 'primary' | 'danger' | 'ghost'
  disabled?: boolean
  type?: 'button' | 'submit'
  className?: string
}) {
  return (
    <button
      type={type}
      disabled={disabled}
      onClick={onClick}
      className={clsx(
        'inline-flex items-center gap-1.5 rounded-md px-2.5 py-1.5 text-sm font-medium transition-colors',
        'disabled:cursor-not-allowed disabled:opacity-50',
        variant === 'default' && 'border border-border bg-surface-1 hover:bg-surface-2',
        variant === 'primary' && 'bg-accent-500 text-white hover:bg-accent-600',
        variant === 'danger' && 'bg-danger text-white hover:opacity-90',
        variant === 'ghost' && 'hover:bg-surface-2',
        className,
      )}
    >
      {children}
    </button>
  )
}

export function Input({
  value,
  onChange,
  placeholder,
  className,
  type = 'text',
  onKeyDown,
  autoFocus,
}: {
  value: string
  onChange: (v: string) => void
  placeholder?: string
  className?: string
  type?: string
  onKeyDown?: (e: React.KeyboardEvent<HTMLInputElement>) => void
  autoFocus?: boolean
}) {
  return (
    <input
      type={type}
      value={value}
      placeholder={placeholder}
      onChange={(e) => onChange(e.target.value)}
      onKeyDown={onKeyDown}
      // eslint-disable-next-line jsx-a11y/no-autofocus
      autoFocus={autoFocus}
      className={clsx(
        'rounded-md border border-border bg-surface-1 px-2 py-1.5 text-sm',
        'placeholder:text-muted-foreground focus:border-accent-500 focus:outline-none',
        className,
      )}
    />
  )
}

/** Detail lives in a sheet rather than a page, so the table stays in view. */
export function Drawer({
  open,
  onClose,
  title,
  subtitle,
  children,
}: {
  open: boolean
  onClose: () => void
  title: string
  subtitle?: ReactNode
  children: ReactNode
}) {
  useEffect(() => {
    if (!open) return

    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }

    window.addEventListener('keydown', onKey)

    return () => window.removeEventListener('keydown', onKey)
  }, [open, onClose])

  if (!open) return null

  return (
    <div className="fixed inset-0 z-40 flex justify-end">
      <button
        type="button"
        aria-label="Close"
        className="absolute inset-0 bg-black/40"
        onClick={onClose}
      />
      <aside className="relative z-50 flex h-full w-full max-w-xl flex-col overflow-y-auto border-l border-border bg-surface-0 shadow-xl">
        <header className="sticky top-0 flex items-start justify-between gap-4 border-b border-border bg-surface-0 px-5 py-4">
          <div>
            <h2 className="text-lg font-semibold">{title}</h2>
            {subtitle && <div className="mt-1 text-sm text-muted-foreground">{subtitle}</div>}
          </div>
          <Button variant="ghost" onClick={onClose}>
            Close
          </Button>
        </header>
        <div className="flex-1 px-5 py-4">{children}</div>
      </aside>
    </div>
  )
}

/** Every destructive confirm names the thing being destroyed. */
export function Confirm({
  open,
  title,
  body,
  confirmLabel,
  onConfirm,
  onCancel,
}: {
  open: boolean
  title: string
  body: ReactNode
  confirmLabel: string
  onConfirm: () => void
  onCancel: () => void
}) {
  if (!open) return null

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
      <button type="button" aria-label="Cancel" className="absolute inset-0 bg-black/50" onClick={onCancel} />
      <div className="relative z-50 w-full max-w-md rounded-lg border border-border bg-surface-0 p-5 shadow-xl">
        <h3 className="text-base font-semibold">{title}</h3>
        <div className="mt-2 text-sm text-muted-foreground">{body}</div>
        <div className="mt-5 flex justify-end gap-2">
          <Button onClick={onCancel}>Cancel</Button>
          <Button variant="danger" onClick={onConfirm}>
            {confirmLabel}
          </Button>
        </div>
      </div>
    </div>
  )
}

export function Empty({ title, hint }: { title: string; hint?: ReactNode }) {
  return (
    <div className="rounded-lg border border-dashed border-border px-6 py-10 text-center">
      <p className="text-sm font-medium">{title}</p>
      {hint && <p className="mt-1 text-sm text-muted-foreground">{hint}</p>}
    </div>
  )
}

export function ErrorNote({ error }: { error: unknown }) {
  const message = error instanceof Error ? error.message : String(error)

  return (
    <div className="rounded-md border border-danger/40 bg-danger/10 px-3 py-2 text-sm text-danger">
      {message}
    </div>
  )
}

export function Section({ title, children, actions }: { title: string; children: ReactNode; actions?: ReactNode }) {
  return (
    <section className="space-y-3">
      <div className="flex items-center justify-between gap-3">
        <h3 className="text-sm font-semibold uppercase tracking-wide text-muted-foreground">
          {title}
        </h3>
        {actions}
      </div>
      {children}
    </section>
  )
}
