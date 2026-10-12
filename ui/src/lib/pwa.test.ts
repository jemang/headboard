import { describe, expect, it } from 'vitest'
import { isRunningBuild } from './pwa'

describe('isRunningBuild', () => {
  it('matches when the page already runs the waiting worker’s bundle', () => {
    expect(isRunningBuild(['assets/index-abc.js', 'assets/index-def.css'], ['https://hb.example.com/manage/assets/index-abc.js'])).toBe(true)
  })

  it('does not match a different bundle', () => {
    expect(isRunningBuild(['assets/index-new.js'], ['https://hb.example.com/assets/index-old.js'])).toBe(false)
  })

  it('does not match when the worker reports nothing', () => {
    expect(isRunningBuild([], ['https://hb.example.com/assets/index-abc.js'])).toBe(false)
  })
})
