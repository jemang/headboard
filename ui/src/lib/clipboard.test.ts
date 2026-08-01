import { describe, expect, it, vi } from 'vitest'
import { copyText } from './clipboard'

describe('copyText', () => {
  it('uses the Clipboard API when available', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    const fallback = vi.fn(() => true)

    await expect(copyText('api key', { clipboard: { writeText }, fallback })).resolves.toBe(true)
    expect(writeText).toHaveBeenCalledWith('api key')
    expect(fallback).not.toHaveBeenCalled()
  })

  it('falls back when the Clipboard API is unavailable', async () => {
    const fallback = vi.fn(() => true)

    await expect(copyText('api key', { fallback })).resolves.toBe(true)
    expect(fallback).toHaveBeenCalledWith('api key')
  })

  it('falls back when the Clipboard API rejects the copy', async () => {
    const fallback = vi.fn(() => true)

    await expect(copyText('api key', {
      clipboard: { writeText: vi.fn().mockRejectedValue(new Error('not allowed')) },
      fallback,
    })).resolves.toBe(true)
    expect(fallback).toHaveBeenCalledWith('api key')
  })
})
