import { describe, expect, it, vi } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { ToastHost } from '../components/Toast'
import { renderToStaticMarkup } from 'react-dom/server'
import { AgentsSection, ServerCard, SshRoute } from './SshAccess'
import type { SshServer } from '../lib/api'

const base: SshServer = { name: 'app-1', ip: '192.0.2.10', suggestions: [], rows: [], pendingClears: [] }
const noop = () => {}
const props = { users: [{ id: 1, name: 'alice' }], onSetRow: noop, onApprove: noop, onUnbind: noop }

describe('ServerCard', () => {
  it('offers Approve for a single suggestion and blocks an ambiguous one', () => {
    const single = renderToStaticMarkup(
      <ServerCard {...props} server={{ ...base, suggestions: [{ agentId: 7, agentName: 'pve1', targetRef: '105', hostname: 'app-1', ambiguous: false }] }} />,
    )
    expect(single).toContain('pve1 / CT 105')
    expect(single).toContain('Approve')

    const ambiguous = renderToStaticMarkup(
      <ServerCard
        {...props}
        server={{
          ...base,
          suggestions: [
            { agentId: 7, agentName: 'pve1', targetRef: '105', hostname: 'a', ambiguous: true },
            { agentId: 8, agentName: 'pve2', targetRef: '300', hostname: 'b', ambiguous: true },
          ],
        }}
      />,
    )
    expect(ambiguous).toContain('Two targets report this IP')
    // Name both, so the admin knows which container to fix.
    expect(ambiguous).toContain('pve1 / CT 105 (a)')
    expect(ambiguous).toContain('pve2 / CT 300 (b)')
    expect(ambiguous).not.toContain('>Approve<')
  })

  it('explains a suspended binding and shows rows, warnings, root badge and pending clears', () => {
    const markup = renderToStaticMarkup(
      <ServerCard
        {...props}
        server={{
          ...base,
          binding: { agentId: 7, agentName: 'pve1', targetRef: '105', approvedIp: '192.0.2.50', approvedLabel: 'app', state: 'suspended', reapprovable: true },
          rows: [
            { unixUser: 'root', users: [{ id: 1, name: 'alice', keys: 0, missing: false }], warnings: ['unix user does not exist'] },
          ],
          pendingClears: ['staging'],
        }}
      />,
    )
    expect(markup).toContain('Suspended')
    expect(markup).toContain('Re-approve')
    expect(markup).toContain('root login')
    expect(markup).toContain('unix user does not exist')
    expect(markup).toContain('no keys')
    expect(markup).toContain('Not removed yet, server suspended: staging')
  })

  it('hides Re-approve and says what it waits for when re-approving cannot work', () => {
    const markup = renderToStaticMarkup(
      <ServerCard
        {...props}
        server={{
          ...base,
          binding: { agentId: 7, agentName: 'pve1', targetRef: '105', approvedIp: '192.0.2.50', approvedLabel: 'app', state: 'suspended', reapprovable: false },
        }}
      />,
    )
    expect(markup).not.toContain('Re-approve')
    expect(markup).toContain('Waiting for pve1 to report CT 105 at 192.0.2.10')
  })
})

describe('SshRoute', () => {
  // The server refuses anyway; the page must not poll into 403s or offer a
  // working "Add agent" form to a role that cannot use it.
  it('shows a plain refusal to roles without manage:ssh', () => {
    const markup = renderToStaticMarkup(<SshRoute capabilities={['view:all', 'manage:policy']} />)
    expect(markup).toContain('You do not have access to SSH access.')
    expect(markup).not.toContain('Add agent')
  })
})

describe('AgentsSection', () => {
  // A skipped container or a unix user missing on a target is only ever
  // reported by the agent; if it is not shown, nobody learns about it.
  it('shows what each agent warned about', () => {
    vi.stubGlobal('window', { location: { origin: 'https://hb.example' } })
    const qc = new QueryClient()
    qc.setQueryData(['ssh-agents'], {
      agents: [{
        id: 1, name: 'pve1', mode: 'proxmox', version: '1.0.0', versionMatch: true, online: true,
        discovered: [], bindings: 1, pendingClears: 0,
        warnings: [{ ref: '105', unix_user: 'deploy', message: 'unix user does not exist on the target' }],
      }],
    })

    const markup = renderToStaticMarkup(
      <QueryClientProvider client={qc}>
        <ToastHost>
          <AgentsSection />
        </ToastHost>
      </QueryClientProvider>,
    )

    expect(markup).toContain('CT 105 deploy: unix user does not exist on the target')
    vi.unstubAllGlobals()
  })
})

describe('ServerCard warnings', () => {
  it('renders a repeated warning twice without a key clash', () => {
    const errors = vi.spyOn(console, 'error').mockImplementation(() => {})
    const markup = renderToStaticMarkup(
      <ServerCard {...props} server={{ ...base, rows: [{ unixUser: 'deploy', users: [], warnings: ['same', 'same'] }] }} />,
    )

    expect(markup.split('same').length - 1).toBe(2)
    expect(errors.mock.calls.flat().join(' ')).not.toContain('same key')
    errors.mockRestore()
  })
})
