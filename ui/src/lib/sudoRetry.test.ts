import { afterEach, describe, expect, it, vi } from 'vitest'
import { api, ApiError, isCancelledSudo, setSudoPrompt } from './api'

const problem = (status: number, code: string) =>
  new Response(JSON.stringify({ status, detail: 'confirm with your PIN to do this', code }), { status, headers: { 'Content-Type': 'application/problem+json' } })

afterEach(() => {
  vi.unstubAllGlobals()
  setSudoPrompt(null)
})

describe('a request that needs sudo', () => {
  it('asks for the PIN once and sends the same request again', async () => {
    const fetch = vi.fn().mockResolvedValueOnce(problem(403, 'sudo_required')).mockResolvedValueOnce(new Response(null, { status: 204 }))
    vi.stubGlobal('fetch', fetch)
    const prompt = vi.fn().mockResolvedValue(true)
    setSudoPrompt(prompt)

    await api.deleteDevice(4)

    expect(prompt).toHaveBeenCalledOnce()
    expect(fetch).toHaveBeenCalledTimes(2)
    expect(fetch.mock.calls[1][1].method).toBe('DELETE')
  })

  it('gives up with the coded error when the PIN prompt is cancelled', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(problem(403, 'sudo_required')))
    setSudoPrompt(() => Promise.resolve(false))

    const err = await api.deleteDevice(4).catch((e: unknown) => e)

    expect(err).toBeInstanceOf(ApiError)
    expect((err as ApiError).code).toBe('sudo_required')
  })
})

describe('a cancelled PIN prompt', () => {
  it('is not an error to report: the person chose it', () => {
    expect(isCancelledSudo(new ApiError(403, 'confirm with your PIN to do this', 'sudo_required'))).toBe(true)
    expect(isCancelledSudo(new ApiError(403, 'your role does not allow this'))).toBe(false)
    expect(isCancelledSudo(new Error('x'))).toBe(false)
  })
})
