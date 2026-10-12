import { describe, expect, it } from 'vitest'
import { swBypass } from './swBoundary'

const root = 'https://hb.example.com/'
const manage = 'https://hb.example.com/manage/'

describe('swBypass', () => {
  it('never touches the API or auth at the root', () => {
    expect(swBypass('https://hb.example.com/api/devices', 'GET', root)).toBe(true)
    expect(swBypass('https://hb.example.com/api/events', 'GET', root)).toBe(true)
    expect(swBypass('https://hb.example.com/auth/callback?code=x', 'GET', root)).toBe(true)
    expect(swBypass('https://hb.example.com/auth', 'GET', root)).toBe(true)
  })

  it('never touches the API or auth under a base path', () => {
    expect(swBypass('https://hb.example.com/manage/api/me', 'GET', manage)).toBe(true)
    expect(swBypass('https://hb.example.com/manage/auth/oidc', 'GET', manage)).toBe(true)
  })

  it('ignores other origins, other paths and non-GET', () => {
    expect(swBypass('https://idp.example.com/authorize', 'GET', root)).toBe(true)
    expect(swBypass('https://hb.example.com/other/app', 'GET', manage)).toBe(true)
    expect(swBypass('https://hb.example.com/auth/logout', 'POST', root)).toBe(true)
  })

  it('handles app pages and assets', () => {
    expect(swBypass('https://hb.example.com/manage/devices?device=3', 'GET', manage)).toBe(false)
    expect(swBypass('https://hb.example.com/manage/assets/index-abc.js', 'GET', manage)).toBe(false)
    expect(swBypass('https://hb.example.com/apiary', 'GET', root)).toBe(false)
  })
})
