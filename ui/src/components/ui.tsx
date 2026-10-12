import { useEffect, useRef, useState, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { AlertTriangle, Check, Copy, MoreVertical, X } from 'lucide-react'
import { isCancelledSudo } from '../lib/api'
import { cn } from '../lib/cn'
import { copyText } from '../lib/clipboard'
import { useModal } from '../lib/modal'

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
    <span className="inline-flex items-start gap-1.5">
      <span
        aria-hidden
        className={cn(
          'mt-1.5 size-2 shrink-0 rounded-full',
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
        // Standalone copy buttons are controls and share the control size.
        compact && 'size-10 justify-center rounded-lg border border-border px-0 md:size-9',
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
  ariaLabel,
}: {
  children: ReactNode
  onClick?: () => void
  variant?: 'default' | 'primary' | 'danger' | 'danger-solid' | 'ghost'
  disabled?: boolean
  type?: 'button' | 'submit'
  className?: string
  /** A lucide icon component. Always alongside the label, never instead. */
  icon?: React.ComponentType<{ className?: string; strokeWidth?: number; 'aria-hidden'?: boolean }>
  title?: string
  /** Names the target when the visible label alone is ambiguous in a list. */
  ariaLabel?: string
}) {
  return (
    <button
      type={type}
      disabled={disabled}
      onClick={onClick}
      title={title}
      aria-label={ariaLabel}
      className={cn(
        // One control height for every button and field: 40px on a phone (a 2px invisible
        // margin makes the tap area 44px), 36px on desktop.
        'btn relative inline-flex h-10 min-h-0 items-center gap-1.5 rounded-lg px-3 text-sm font-medium shadow-none after:absolute after:-inset-0.5 md:h-9 md:after:hidden',
        'transition-[background-color,border-color,opacity] duration-100',
        'disabled:cursor-not-allowed disabled:opacity-50',
        variant === 'default' &&
          'border border-border bg-surface-1 hover:border-accent-500/40 hover:bg-surface-2 active:bg-surface-2',
        // Deep teal with white text in light, bright teal with ink text in dark.
        variant === 'primary' && 'border-0 bg-accent-700 text-white hover:bg-accent-600 active:bg-accent-700 dark:bg-accent-500 dark:text-slate-950 dark:hover:bg-accent-400',
        // Outlined in lists and sheets; solid red is kept for the final confirm.
        variant === 'danger' && 'border border-danger/50 bg-transparent text-danger hover:bg-danger/10 active:bg-danger/15',
        variant === 'danger-solid' && 'bg-danger text-white hover:opacity-90 active:opacity-80',
        variant === 'ghost' && 'text-muted-foreground hover:bg-surface-2 hover:text-foreground',
        className,
      )}
    >
      {Icon && <Icon aria-hidden className="size-4 shrink-0" strokeWidth={1.5} />}
      {children}
    </button>
  )
}

/**
 * Small icon-only button for everyday actions (edit, rename, delete, view,
 * close). The label is the accessible name and the hover tooltip. 32px to look
 * at; on a phone an invisible hit area grows it to 44px.
 */
export function IconButton({
  icon: Icon,
  label,
  onClick,
  variant = 'default',
  disabled,
  className,
}: {
  icon: React.ComponentType<{ className?: string; strokeWidth?: number; 'aria-hidden'?: boolean }>
  label: string
  onClick?: () => void
  variant?: 'default' | 'danger' | 'ghost'
  disabled?: boolean
  className?: string
}) {
  return (
    <button
      type="button"
      aria-label={label}
      title={label}
      disabled={disabled}
      onClick={onClick}
      className={cn(
        'relative inline-grid size-10 shrink-0 place-items-center rounded-lg transition-colors after:absolute after:-inset-0.5 md:size-9 md:after:hidden',
        'disabled:cursor-not-allowed disabled:opacity-40',
        variant === 'default' && 'border border-border bg-surface-1 text-muted-foreground hover:bg-surface-2 hover:text-foreground',
        variant === 'ghost' && 'text-muted-foreground hover:bg-surface-2 hover:text-foreground',
        variant === 'danger' && 'border border-danger/40 text-danger hover:bg-danger/10',
        className,
      )}
    >
      <Icon aria-hidden className="size-4" strokeWidth={1.5} />
    </button>
  )
}

/**
 * A ⋮ menu for actions that are rare or destructive, so a card header stays
 * calm. Closes on Escape, on a pick, and on a press anywhere outside it.
 */
export function Menu({
  label,
  items,
}: {
  label: string
  items: { label: string; icon?: React.ComponentType<{ className?: string; strokeWidth?: number; 'aria-hidden'?: boolean }>; onSelect: () => void; danger?: boolean }[]
}) {
  const [open, setOpen] = useState(false)
  const root = useRef<HTMLDivElement>(null)
  const trigger = useRef<HTMLButtonElement>(null)

  useEffect(() => {
    if (!open) return
    root.current?.querySelector<HTMLElement>('[role="menuitem"]')?.focus()
    const onDown = (e: PointerEvent) => {
      if (!root.current?.contains(e.target as Node)) setOpen(false)
    }
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== 'Escape') return
      setOpen(false)
      trigger.current?.focus()
    }
    document.addEventListener('pointerdown', onDown)
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('pointerdown', onDown)
      document.removeEventListener('keydown', onKey)
    }
  }, [open])

  return (
    <div ref={root} className="relative">
      <button
        ref={trigger}
        type="button"
        aria-label={label}
        title={label}
        aria-haspopup="menu"
        aria-expanded={open}
        onClick={() => setOpen(!open)}
        className="relative inline-grid size-10 shrink-0 place-items-center rounded-lg text-muted-foreground transition-colors after:absolute after:-inset-0.5 hover:bg-surface-2 hover:text-foreground md:size-9 md:after:hidden"
      >
        <MoreVertical aria-hidden className="size-4" strokeWidth={1.5} />
      </button>
      {open && (
        <div role="menu" aria-label={label} className="absolute right-0 top-full z-20 mt-1 min-w-44 rounded-lg border border-border bg-surface-1 p-1 shadow-raised animate-rise">
          {items.map((it) => (
            <button
              key={it.label}
              type="button"
              role="menuitem"
              onClick={() => {
                setOpen(false)
                it.onSelect()
              }}
              className={cn(
                'flex min-h-10 w-full items-center gap-2 rounded-md px-3 text-left text-sm outline-none transition-colors md:min-h-9',
                it.danger ? 'text-danger hover:bg-danger/10 focus-visible:bg-danger/10' : 'hover:bg-surface-2 focus-visible:bg-surface-2',
              )}
            >
              {it.icon && <it.icon aria-hidden className="size-4" strokeWidth={1.5} />}
              {it.label}
            </button>
          ))}
        </div>
      )}
    </div>
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
        'input h-10 min-h-0 rounded-lg border border-border bg-surface-1 px-3 text-sm shadow-none md:h-9',
        'transition-colors placeholder:text-muted-foreground',
        'focus:border-accent-500 focus:outline-none',
        className,
      )}
    />
  )
}

/**
 * Detail lives in a sheet rather than a page, so the list stays in view. On a
 * phone the sheet fills the screen and its actions sit in a footer within
 * thumb reach.
 */
export function Drawer({
  open,
  onClose,
  title,
  subtitle,
  icon: Icon,
  actions,
  children,
}: {
  open: boolean
  onClose: () => void
  title: string
  subtitle?: ReactNode
  /** What the sheet is about (a device, a person), shown in a soft circle before the title. */
  icon?: React.ComponentType<{ className?: string; strokeWidth?: number; 'aria-hidden'?: boolean }>
  /** Rendered in a sticky footer; destructive actions go last. */
  actions?: ReactNode
  children: ReactNode
}) {
  const panel = useRef<HTMLElement>(null)
  useModal(open, panel, onClose)

  if (!open) return null

  return portal(
    <div className="fixed inset-0 z-40 flex justify-end">
      <button
        type="button"
        aria-label="Close"
        tabIndex={-1}
        className="absolute inset-0 bg-black/50 animate-fade-in"
        onClick={onClose}
      />
      <aside
        ref={panel}
        role="dialog"
        aria-modal="true"
        aria-label={title}
        className="relative z-50 flex h-full w-full flex-col border-border bg-surface-0 shadow-raised outline-none animate-slide-in md:max-w-xl md:border-l"
      >
        <header className="flex items-start justify-between gap-4 border-b border-border bg-surface-0/95 px-4 pb-3 pt-[max(0.75rem,env(safe-area-inset-top))] backdrop-blur md:px-5 md:py-4">
          <div className="flex min-w-0 items-start gap-3">
            {Icon && (
              <span aria-hidden className="grid size-10 shrink-0 place-items-center rounded-full bg-accent-500/12 text-accent-700 dark:text-accent-400">
                <Icon className="size-5" strokeWidth={1.6} />
              </span>
            )}
            <div className="min-w-0">
              <h2 className="truncate text-display font-semibold">{title}</h2>
              {subtitle && <div className="mt-1 text-sm text-muted-foreground">{subtitle}</div>}
            </div>
          </div>
          <IconButton icon={X} label="Close" variant="ghost" onClick={onClose} />
        </header>
        <div className="min-h-0 flex-1 overflow-y-auto px-4 py-4 md:px-5">{children}</div>
        {actions && (
          <footer className="flex flex-wrap gap-2 border-t border-border bg-surface-1 px-4 pb-[max(0.75rem,env(safe-area-inset-bottom))] pt-3 md:px-5">
            {actions}
          </footer>
        )}
      </aside>
    </div>,
  )
}

/** Modals render into <body> so the app behind them can be made inert; on the server there is no body, so they render in place. */
function portal(node: ReactNode) {
  return typeof document === 'undefined' ? node : createPortal(node, document.body)
}

/** Every destructive confirm names the thing being destroyed. */
export function Confirm({
  open,
  title,
  body,
  confirmLabel,
  onConfirm,
  onCancel,
  busy = false,
}: {
  open: boolean
  title: string
  body: ReactNode
  confirmLabel: string
  onConfirm: () => void
  onCancel: () => void
  /** The confirmed request is running; a second click would send it again. */
  busy?: boolean
}) {
  // Focus starts on the safe choice: Cancel is the dialog's first control,
  // and useModal focuses the first control.
  const dialog = useRef<HTMLDivElement>(null)
  useModal(open, dialog, onCancel, 'first')

  if (!open) return null

  return portal(
    <div className="fixed inset-0 z-50 flex items-end justify-center p-4 sm:items-center">
      <button type="button" aria-label="Cancel" tabIndex={-1} className="absolute inset-0 bg-black/50 animate-fade-in" onClick={onCancel} />
      <div
        ref={dialog}
        role="alertdialog"
        aria-modal="true"
        aria-label={title}
        className="relative z-50 w-full max-w-md rounded-xl border border-border bg-surface-0 p-5 pb-[max(1.25rem,env(safe-area-inset-bottom))] shadow-raised animate-rise"
      >
        <h3 className="text-title">{title}</h3>
        <div className="mt-2 text-sm text-muted-foreground">{body}</div>
        <div className="mt-5 flex justify-end gap-2">
          <Button onClick={onCancel}>Cancel</Button>
          <Button variant="danger-solid" onClick={onConfirm} disabled={busy}>
            {confirmLabel}
          </Button>
        </div>
      </div>
    </div>,
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
    <div className="rounded-xl border border-dashed border-border bg-surface-1/40 px-6 py-12 text-center [td_&]:border-0 [td_&]:bg-transparent">
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
  if (isCancelledSudo(error)) return null

  const message = error instanceof Error ? error.message : String(error)

  return (
    <div className="alert flex items-start gap-2 rounded-lg border border-danger/40 bg-danger/10 px-3 py-2 text-sm text-danger shadow-none">
      <AlertTriangle aria-hidden className="mt-0.5 size-4 shrink-0" strokeWidth={1.5} />
      <span className="min-w-0 break-words">{message}</span>
    </div>
  )
}

export function Section({ title, children, actions, id }: { title: string; children: ReactNode; actions?: ReactNode; id?: string }) {
  return (
    // A titled block, not a card: the table or list inside is the only frame,
    // so nothing ends up as a card inside a card.
    <section id={id} className="scroll-mt-24 space-y-3">
      <div className="flex min-h-8 flex-wrap items-baseline justify-between gap-x-3 gap-y-1">
        <h2 className="shrink-0 text-title text-foreground">{title}</h2>
        {actions}
      </div>
      {children}
    </section>
  )
}
