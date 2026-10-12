import { describe, expect, it } from 'vitest'
import { parseTheme } from './theme'

describe('parseTheme', () => {
  it('starts light for someone who never chose', () => {
    expect(parseTheme(null)).toBe('light')
  })

  it('keeps an explicit choice', () => {
    expect(parseTheme('dark')).toBe('dark')
    expect(parseTheme('light')).toBe('light')
  })

  it('falls back to light on anything else', () => {
    expect(parseTheme('sepia')).toBe('light')
  })
})
