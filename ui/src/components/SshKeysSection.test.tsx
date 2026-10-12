import { describe, expect, it } from 'vitest'
import { renderToStaticMarkup } from 'react-dom/server'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { ToastHost } from './Toast'
import { SshKeysSection } from './SshKeysSection'

describe('SshKeysSection', () => {
  it('renders listed keys with fingerprint and an add form', () => {
    const qc = new QueryClient()
    qc.setQueryData(['ssh-keys', 4], {
      keys: [{ id: 1, headscaleUserId: 4, type: 'ssh-ed25519', fingerprint: 'SHA256:abc', label: 'laptop', createdAt: '2026-10-12T00:00:00Z' }],
    })

    const markup = renderToStaticMarkup(
      <QueryClientProvider client={qc}>
        <ToastHost>
          <SshKeysSection userId={4} userName="alice" />
        </ToastHost>
      </QueryClientProvider>,
    )

    expect(markup).toContain('SHA256:abc')
    expect(markup).toContain('laptop')
    expect(markup).toContain('Paste a public key')
    expect(markup).toContain('Remove key laptop')
  })

  it('shows a loading placeholder instead of an empty list while keys load', () => {
    const markup = renderToStaticMarkup(
      <QueryClientProvider client={new QueryClient()}>
        <ToastHost>
          <SshKeysSection userId={9} userName="bob" />
        </ToastHost>
      </QueryClientProvider>,
    )

    expect(markup).toContain('Loading bob&#x27;s SSH keys')
  })
})
