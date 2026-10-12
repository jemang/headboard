import { useEffect, useRef, type RefObject } from 'react'

const FOCUSABLE = 'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'
let openCount = 0

/**
 * Focus in, Tab trapped, focus restored on close, the app behind made inert
 * and the page scroll locked. Modals portal to <body> so #root can go inert;
 * the count keeps a Confirm opened over a Drawer from releasing it early.
 */
export function useModal(
  open: boolean,
  ref: RefObject<HTMLElement | null>,
  onClose: () => void,
  /** 'panel' for sheets (nothing pre-selected); 'first' where a control should take focus (Confirm, palette). */
  initialFocus: 'panel' | 'first' = 'panel',
) {
  // Callers pass inline closures; reading through a ref keeps the effect from
  // re-running (and stealing focus) on every render.
  const close = useRef(onClose)
  close.current = onClose

  useEffect(() => {
    if (!open) return

    const opener = document.activeElement as HTMLElement | null
    const root = document.getElementById('root')
    openCount += 1
    if (root) root.inert = true
    document.body.style.overflow = 'hidden'

    if (initialFocus === 'first') {
      ref.current?.querySelector<HTMLElement>(FOCUSABLE)?.focus()
    } else {
      ref.current?.setAttribute('tabindex', '-1')
      ref.current?.focus({ preventScroll: true })
    }

    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.stopPropagation()
        close.current()
      }
      if (e.key !== 'Tab' || !ref.current) return

      const items = [...ref.current.querySelectorAll<HTMLElement>(FOCUSABLE)]
      if (items.length === 0) return

      const head = items[0]
      const tail = items[items.length - 1]

      if (e.shiftKey && document.activeElement === head) {
        e.preventDefault()
        tail.focus()
      } else if (!e.shiftKey && document.activeElement === tail) {
        e.preventDefault()
        head.focus()
      }
    }

    const node = ref.current
    node?.addEventListener('keydown', onKey)

    return () => {
      node?.removeEventListener('keydown', onKey)
      openCount -= 1
      if (openCount === 0) {
        if (root) root.inert = false
        document.body.style.overflow = ''
      }
      opener?.focus()
    }
  }, [open, ref, initialFocus])
}
