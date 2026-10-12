import { describe, expect, it } from 'vitest'
import { renderToStaticMarkup } from 'react-dom/server'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { ActivityList } from './Activity'
import { dayLabel } from '../lib/activity'
import type { AuditEntry } from '../lib/api'

const now = new Date('2026-10-12T15:00:00')

describe('dayLabel', () => {
  it('says Today and Yesterday, then the date', () => {
    expect(dayLabel('2026-10-12T09:00:00', now)).toBe('Today')
    expect(dayLabel('2026-10-11T23:59:00', now)).toBe('Yesterday')
    expect(dayLabel('2026-10-01T09:00:00', now)).not.toMatch(/Today|Yesterday/)
  })
})

describe('ActivityList', () => {
  const entries: AuditEntry[] = [
    { id: 3, actorLabel: 'ops@example.com', action: 'device.delete', targetType: 'device', targetLabel: 'web-1', ip: '203.0.113.7', createdAt: '2026-10-12T10:00:00Z' },
    { id: 2, actorLabel: 'a***@example.com', action: 'auth.login_failed', targetType: 'session', after: { reason: 'bad credentials' }, createdAt: '2026-10-12T09:00:00Z' },
  ]
  const render = (next: number) =>
    renderToStaticMarkup(
      <QueryClientProvider client={new QueryClient()}>
        <ActivityList entries={entries} hasMore={next !== 0} loadingMore={false} onMore={() => {}} onActor={() => {}} now={now} />
      </QueryClientProvider>,
    )

  it('reads each change as a sentence with who, what and from where', () => {
    const html = render(0)
    expect(html).toContain('ops@example.com</span> removed device web-1')
    expect(html).toContain('203.0.113.7')
    expect(html).toContain('bad credentials')
    expect(html).toContain('Today')
  })

  it('offers more only when the server says there is more', () => {
    expect(render(5)).toContain('Load more')
    expect(render(0)).not.toContain('Load more')
  })
})
