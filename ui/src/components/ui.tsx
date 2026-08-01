import { useEffect, useRef, useState, type ReactNode } from 'react'
import { AlertTriangle, Check, Copy, X } from 'lucide-react'
import { cn } from '../lib/cn'
import { copyText } from '../lib/clipboard'

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
        className={cn(
          'size-2 rounded-full',
          bad ? 'bg-danger' : warn ? 'bg-warn' : ok ? 'bg-ok' : 'bg-muted-foreground/40',
        )}
      />
      <span className={cn('text-sm', bad && 'text-danger', !ok && !warn && !bad && 'text-muted-foreground')}>
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
      className={cn(
        'badge inline-flex h-auto items-center rounded-md border-0 px-1.5 py-0.5 font-mono text-xs font-medium',
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

/**
 * Technical values are monospace and copy on click.
 *
 * `compact` is for values too long to show twice — an enrolment command sitting
 * next to its own copy button. It renders as an icon with an accessible label
 * rather than the value clipped to a single character, which is what a long
 * string did to this control.
 */
export function Mono({
  value,
  className,
  compact,
  label,
}: {
  value: string
  className?: string
  compact?: boolean
  label?: string
}) {
  const [copied, setCopied] = useState(false)

  useEffect(() => {
    if (!copied) return

    const t = setTimeout(() => setCopied(false), 1200)

    return () => clearTimeout(t)
  }, [copied])

  return (
    <button
      type="button"
      title={copied ? 'Copied' : label ? `Copy ${label}` : `Copy ${value}`}
      onClick={() => {
        void copyText(value).then(setCopied)
      }}
      className={cn(
        'group inline-flex items-center gap-1 rounded px-1 font-mono text-xs transition-colors hover:bg-surface-2',
        copied && 'bg-ok/15 text-ok',
        className,
      )}
    >
      {/* The value stays put while copying — swapping it for the word
          "copied" made a column of addresses jump. */}
      {!compact && <span>{value}</span>}
      {copied ? (
        <Check aria-hidden className={cn('shrink-0', compact ? 'size-4' : 'size-3')} strokeWidth={2} />
      ) : (
        <Copy
          aria-hidden
          className={cn(
            'shrink-0 transition-opacity',
            compact
              ? 'size-4'
              : 'size-3 opacity-0 group-hover:opacity-60 group-focus-visible:opacity-60',
          )}
          strokeWidth={1.5}
        />
      )}
      <span className="sr-only">
        {copied ? 'copied' : `copy ${label ?? value}`}
      </span>
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
  icon: Icon,
  title,
}: {
  children: ReactNode
  onClick?: () => void
  variant?: 'default' | 'primary' | 'danger' | 'ghost'
  disabled?: boolean
  type?: 'button' | 'submit'
  className?: string
  /** A lucide icon component. Always alongside the label, never instead. */
  icon?: React.ComponentType<{ className?: string; strokeWidth?: number; 'aria-hidden'?: boolean }>
  title?: string
}) {
  return (
    <button
      type={type}
      disabled={disabled}
      onClick={onClick}
      title={title}
      className={cn(
        'btn inline-flex h-auto min-h-0 items-center gap-1.5 rounded-lg px-3 py-2 text-sm font-medium shadow-none',
        'transition-[background-color,border-color,opacity] duration-100',
        'disabled:cursor-not-allowed disabled:opacity-50',
        variant === 'default' &&
          'border border-border bg-surface-1 hover:border-accent-500/40 hover:bg-surface-2 active:bg-surface-2',
        variant === 'primary' && 'border-0 bg-accent-500 text-slate-950 hover:bg-accent-400 active:bg-accent-600',
        variant === 'danger' && 'bg-danger text-white hover:opacity-90 active:opacity-80',
        variant === 'ghost' && 'text-muted-foreground hover:bg-surface-2 hover:text-foreground',
        className,
      )}
    >
      {Icon && <Icon aria-hidden className="size-4 shrink-0" strokeWidth={1.5} />}
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
      className={cn(
        'input h-auto rounded-lg border border-border bg-surface-1 px-3 py-2 text-sm shadow-none',
        'transition-colors placeholder:text-muted-foreground',
        'focus:border-accent-500 focus:outline-none',
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
        className="absolute inset-0 bg-black/50 animate-fade-in"
        onClick={onClose}
      />
      <aside
        role="dialog"
        aria-modal="true"
        aria-label={title}
        className="relative z-50 flex h-full w-full max-w-xl flex-col overflow-y-auto border-l border-border bg-surface-0 shadow-raised animate-slide-in"
      >
        <header className="sticky top-0 z-10 flex items-start justify-between gap-4 border-b border-border bg-surface-0/95 px-5 py-4 backdrop-blur">
          <div className="min-w-0">
            <h2 className="truncate text-display font-semibold">{title}</h2>
            {subtitle && <div className="mt-1 text-sm text-muted-foreground">{subtitle}</div>}
          </div>
          <Button variant="ghost" onClick={onClose} icon={X} className="shrink-0">
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
  const cancelRef = useRef<HTMLButtonElement>(null)

  // Escape cancels, and focus starts on the safe choice rather than the
  // destructive one — this dialog deletes devices.
  useEffect(() => {
    if (!open) return

    cancelRef.current?.focus()

    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onCancel()
    }

    window.addEventListener('keydown', onKey)

    return () => window.removeEventListener('keydown', onKey)
  }, [open, onCancel])

  if (!open) return null

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
      <button type="button" aria-label="Cancel" className="absolute inset-0 bg-black/50 animate-fade-in" onClick={onCancel} />
      <div
        role="alertdialog"
        aria-modal="true"
        aria-label={title}
        className="relative z-50 w-full max-w-md rounded-xl border border-border bg-surface-0 p-5 shadow-raised animate-rise"
      >
        <h3 className="text-base font-semibold">{title}</h3>
        <div className="mt-2 text-sm text-muted-foreground">{body}</div>
        <div className="mt-5 flex justify-end gap-2">
          <button
            ref={cancelRef}
            type="button"
            onClick={onCancel}
            className="inline-flex items-center gap-1.5 rounded-md border border-border bg-surface-1 px-2.5 py-1.5 text-sm font-medium hover:bg-surface-2"
          >
            Cancel
          </button>
          <Button variant="danger" onClick={onConfirm}>
            {confirmLabel}
          </Button>
        </div>
      </div>
    </div>
  )
}

export function Empty({
  title,
  hint,
  icon: Icon,
  action,
}: {
  title: string
  hint?: ReactNode
  icon?: React.ComponentType<{ className?: string; strokeWidth?: number; 'aria-hidden'?: boolean }>
  action?: ReactNode
}) {
  return (
    <div className="rounded-xl border border-dashed border-border bg-surface-1/40 px-6 py-12 text-center">
      {Icon && (
        <Icon aria-hidden className="mx-auto mb-3 size-6 text-muted-foreground" strokeWidth={1.25} />
      )}
      <p className="text-sm font-medium">{title}</p>
      {hint && <p className="mx-auto mt-1 max-w-sm text-sm text-muted-foreground">{hint}</p>}
      {action && <div className="mt-4 flex justify-center">{action}</div>}
    </div>
  )
}

export function ErrorNote({ error }: { error: unknown }) {
  const message = error instanceof Error ? error.message : String(error)

  return (
    <div className="alert flex items-start gap-2 rounded-lg border border-danger/40 bg-danger/10 px-3 py-2 text-sm text-danger shadow-none">
      <AlertTriangle aria-hidden className="mt-0.5 size-4 shrink-0" strokeWidth={1.5} />
      <span className="min-w-0 break-words">{message}</span>
    </div>
  )
}

export function Section({ title, children, actions }: { title: string; children: ReactNode; actions?: ReactNode }) {
  return (
    <section className="space-y-4 rounded-xl border border-border bg-surface-1/80 p-4 shadow-sm sm:p-5">
      <div className="flex items-center justify-between gap-3 border-b border-border/70 pb-3">
        <h3 className="text-eyebrow font-semibold uppercase text-muted-foreground">{title}</h3>
        {actions}
      </div>
      {children}
    </section>
  )
}
