import { clsx, type ClassValue } from 'clsx'
import { twMerge } from 'tailwind-merge'

/**
 * Join class names, letting a later one win over an earlier one.
 *
 * `clsx` alone concatenates, so a component's own `px-3` and a caller's `px-6`
 * both end up in the attribute and the winner is whichever Tailwind emitted
 * last — which is not something a call site can reason about. `twMerge` resolves
 * that by keeping the last class in each conflicting group, which is what makes
 * a `className` prop safe to offer.
 */
export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}
