import { describe, expect, it } from 'vitest'
import { renderToStaticMarkup } from 'react-dom/server'
import { Rail } from './Rail'
import type { Me } from '../../lib/api'

const me = { user: { email: 'ops@example.test', role: 'owner' }, capabilities: ['view:all', 'manage:ssh'] } as unknown as Me
const base = { me, active: 'ssh' as const, theme: 'light' as const, setTheme: () => {}, unavailable: false, health: { headscaleState: 'connected' } as never }

describe('Rail', () => {
  it('shows Headscale state and what is waiting in the sidebar card', () => {
    const html = renderToStaticMarkup(<Rail {...base} needCount={3} showNeeds />)

    expect(html).toContain('Headscale connected')
    expect(html).toContain('3 need you')
    expect(html).toContain('href="/devices#needs"')
  })

  it('leaves the count out on Devices, where the list itself is shown', () => {
    const html = renderToStaticMarkup(<Rail {...base} needCount={3} showNeeds={false} />)

    expect(html).toContain('Headscale connected')
    expect(html).not.toContain('need you')
  })
})
