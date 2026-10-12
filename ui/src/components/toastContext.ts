import { createContext, useContext } from 'react'

// Kept apart from Toast.tsx so that file exports only components (fast refresh).
export interface ToastApi {
  /** Confirm something happened. Dismisses itself. */
  ok: (message: string) => void

  /** Report a failure. Stays until dismissed — see below. */
  error: (error: unknown) => void
}

export const ToastContext = createContext<ToastApi>({ ok: () => {}, error: () => {} })

/** Announce the outcome of an action. */
export function useToast() {
  return useContext(ToastContext)
}
