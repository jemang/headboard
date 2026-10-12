const WEAK = 'Too easy to guess: avoid repeated digits and runs like 123456.'

/** What is wrong with a new PIN and its confirmation, null when it can be sent. Mirrors the server. */
export function pinProblem(pin: string, confirm: string): string | null {
  if (pin === '' && confirm === '') return null
  if (!/^\d{6}$/.test(pin)) return 'Six digits, numbers only.'

  const steps = [...pin].slice(1).map((c, i) => Number(c) - Number(pin[i]))
  if (steps.every((d) => d === 0) || steps.every((d) => d === 1) || steps.every((d) => d === -1)) return WEAK

  if (confirm !== pin) return 'The two PINs do not match.'

  return null
}

/** Time left on sudo as m:ss, null when it has ended. */
export function countdown(until: string, now = new Date()): string | null {
  const left = Math.floor((new Date(until).getTime() - now.getTime()) / 1000)
  if (left <= 0) return null

  return `${Math.floor(left / 60)}:${String(left % 60).padStart(2, '0')}`
}

/**
 * What a keystroke (or a paste) in box `index` of the six PIN boxes does. The
 * PIN stays one string with no gaps, so a digit lands at the first empty box
 * at most; a paste fills from there. Returns the box to focus next.
 */
export function typeDigit(pin: string, index: number, input: string): { pin: string; focus: number } {
  const digits = input.replace(/\D/g, '')
  if (digits === '') return { pin, focus: index }

  const start = Math.min(index, pin.length)
  const next = digits.length > 1 ? (pin.slice(0, start) + digits).slice(0, 6) : pin.slice(0, start) + digits + pin.slice(start + 1)

  return { pin: next.slice(0, 6), focus: Math.min(start + digits.length, 5) }
}

/** Backspace in box `index`: clear it, or step back when it is already empty. */
export function eraseDigit(pin: string, index: number): { pin: string; focus: number } {
  if (index < pin.length) return { pin: pin.slice(0, index) + pin.slice(index + 1), focus: index }
  if (pin === '') return { pin, focus: 0 }

  return { pin: pin.slice(0, -1), focus: pin.length - 1 }
}
