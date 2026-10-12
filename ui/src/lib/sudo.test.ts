import { describe, expect, it } from 'vitest'
import { countdown, eraseDigit, pinProblem, typeDigit } from './sudo'

describe('pinProblem', () => {
  it('asks for six digits, the same twice, not an obvious one', () => {
    expect(pinProblem('12a', '')).toBe('Six digits, numbers only.')
    expect(pinProblem('246813', '246812')).toBe('The two PINs do not match.')
    expect(pinProblem('111111', '111111')).toBe('Too easy to guess: avoid repeated digits and runs like 123456.')
    expect(pinProblem('654321', '654321')).toBe('Too easy to guess: avoid repeated digits and runs like 123456.')
    expect(pinProblem('246813', '246813')).toBeNull()
  })

  it('says nothing about an empty confirm while the first PIN is still being typed', () => {
    expect(pinProblem('', '')).toBeNull()
  })
})

describe('countdown', () => {
  const now = new Date('2026-10-12T10:00:00Z')
  it('shows minutes and seconds left, and nothing once over', () => {
    expect(countdown('2026-10-12T10:04:12Z', now)).toBe('4:12')
    expect(countdown('2026-10-12T10:00:05Z', now)).toBe('0:05')
    expect(countdown('2026-10-12T09:59:00Z', now)).toBeNull()
  })
})

describe('typeDigit', () => {
  it('fills the box typed in and moves on', () => {
    expect(typeDigit('12', 2, '3')).toEqual({ pin: '123', focus: 3 })
  })

  it('takes a pasted code across every box', () => {
    expect(typeDigit('', 0, '246813')).toEqual({ pin: '246813', focus: 5 })
    expect(typeDigit('', 0, '24 68-13')).toEqual({ pin: '246813', focus: 5 })
  })

  it('replaces a digit in the middle without shifting the rest', () => {
    expect(typeDigit('246813', 2, '9')).toEqual({ pin: '249813', focus: 3 })
  })

  it('ignores what is not a digit', () => {
    expect(typeDigit('24', 2, 'a')).toEqual({ pin: '24', focus: 2 })
  })
})

describe('eraseDigit', () => {
  it('clears this box, or the one before when this one is empty', () => {
    expect(eraseDigit('246', 2)).toEqual({ pin: '24', focus: 2 })
    expect(eraseDigit('24', 2)).toEqual({ pin: '2', focus: 1 })
    expect(eraseDigit('', 0)).toEqual({ pin: '', focus: 0 })
  })
})
