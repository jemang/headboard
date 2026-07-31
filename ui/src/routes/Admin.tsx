import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, type Me, type Role } from '../lib/api'
import { Badge, Button, Empty, ErrorNote, Input, Mono, Section, Status } from '../components/ui'

const roles: Role[] = ['owner', 'admin', 'network-admin', 'auditor', 'member']

/** Headscale users and Headboard accounts, side by side — because linking them
 *  is the one thing that has to happen for a member to see anything. */
export function People({ me }: { me: Me }) {
  const qc = useQueryClient()
  const users = useQuery({ queryKey: ['tailnet-users'], queryFn: api.tailnetUsers })
  const accounts = useQuery({ queryKey: ['accounts'], queryFn: api.accounts })

  const [newUser, setNewUser] = useState('')

  const invalidate = () => {
    void qc.invalidateQueries({ queryKey: ['tailnet-users'] })
    void qc.invalidateQueries({ queryKey: ['accounts'] })
  }

  const create = useMutation({
    mutationFn: (name: string) => api.createTailnetUser({ name }),
    onSuccess: () => {
      setNewUser('')
      invalidate()
    },
  })

  const link = useMutation({
    mutationFn: ({ id, hsID }: { id: number; hsID: number | null }) => api.linkAccount(id, hsID),
    onSuccess: invalidate,
  })

  const role = useMutation({
    mutationFn: ({ id, r }: { id: number; r: Role }) => api.setAccountRole(id, r),
    onSuccess: invalidate,
  })

  if (users.error) return <ErrorNote error={users.error} />

  return (
    <div className="space-y-6">
      <header>
        <h1 className="text-xl font-semibold">People</h1>
        <p className="text-sm text-muted-foreground">
          Headscale users own devices. Headboard accounts are OIDC identities with a role. They have
          to be linked, and a user created with the CLI has no OIDC identity to match.
        </p>
      </header>

      {(accounts.error || create.error || link.error || role.error) && (
        <ErrorNote error={accounts.error ?? create.error ?? link.error ?? role.error} />
      )}

      <Section
        title="Headboard accounts"
        actions={<span className="text-xs text-muted-foreground">who can sign in</span>}
      >
        {accounts.isPending ? (
          <p className="text-sm text-muted-foreground">Loading accounts…</p>
        ) : accounts.data?.accounts.length ? (
          <div className="overflow-x-auto rounded-lg border border-border">
            <table className="w-full min-w-max text-sm">
              <thead className="bg-surface-1 text-left text-xs uppercase tracking-wide text-muted-foreground">
                <tr>
                  <th className="px-3 py-2 font-medium">Identity</th>
                  <th className="px-3 py-2 font-medium">Role</th>
                  <th className="px-3 py-2 font-medium">Linked Headscale user</th>
                </tr>
              </thead>
              <tbody>
                {accounts.data.accounts.map((a) => (
                  <tr key={a.id} className="border-t border-border">
                    <td className="px-3 py-2">
                      <div className="font-medium">{a.displayName || a.email}</div>
                      <div className="text-xs text-muted-foreground">{a.email}</div>
                    </td>
                    <td className="px-3 py-2">
                      <select
                        value={a.role}
                        disabled={a.id === me.user.id}
                        onChange={(e) => role.mutate({ id: a.id, r: e.target.value as Role })}
                        className="rounded-md border border-border bg-surface-1 px-2 py-1 text-sm disabled:opacity-50"
                      >
                        {roles.map((r) => (
                          <option key={r} value={r}>
                            {r}
                          </option>
                        ))}
                      </select>
                    </td>
                    <td className="px-3 py-2">
                      <select
                        value={a.headscaleUserId ?? ''}
                        onChange={(e) =>
                          link.mutate({
                            id: a.id,
                            hsID: e.target.value === '' ? null : Number(e.target.value),
                          })
                        }
                        className="rounded-md border border-border bg-surface-1 px-2 py-1 text-sm"
                      >
                        <option value="">— not linked —</option>
                        {users.data?.users.map((u) => (
                          <option key={u.id} value={u.id}>
                            {u.name}
                          </option>
                        ))}
                      </select>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : (
          <Empty title="No accounts yet" hint="The first person to sign in becomes the owner." />
        )}
      </Section>

      <Section title="Headscale users">
        <div className="overflow-x-auto rounded-lg border border-border">
          <table className="w-full min-w-max text-sm">
            <thead className="bg-surface-1 text-left text-xs uppercase tracking-wide text-muted-foreground">
              <tr>
                <th className="px-3 py-2 font-medium">User</th>
                <th className="px-3 py-2 font-medium">Devices</th>
                <th className="px-3 py-2 font-medium">OIDC identity</th>
              </tr>
            </thead>
            <tbody>
              {users.data?.users.map((u) => (
                <tr key={u.id} className="border-t border-border">
                  <td className="px-3 py-2">
                    <div className="font-medium">{u.name}</div>
                    {u.email && <div className="text-xs text-muted-foreground">{u.email}</div>}
                  </td>
                  <td className="px-3 py-2">{u.devices}</td>
                  <td className="px-3 py-2">
                    {u.providerId ? (
                      <Status ok label="matched automatically" />
                    ) : (
                      <Status ok={false} warn label="none — must be linked by hand" />
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>

        <div className="flex items-center gap-2">
          <Input value={newUser} onChange={setNewUser} placeholder="new username" className="w-56" />
          <Button disabled={newUser === ''} onClick={() => create.mutate(newUser)}>
            Create user
          </Button>
        </div>
      </Section>
    </div>
  )
}

export function Keys({ me }: { me: Me }) {
  const qc = useQueryClient()
  const admin = me.capabilities.includes('manage:keys')

  const [minted, setMinted] = useState<{ command: string; warning: string } | null>(null)
  const [apiKey, setApiKey] = useState<{ key: string; warning: string } | null>(null)
  const [user, setUser] = useState('')
  const [reusable, setReusable] = useState(false)
  const [ephemeral, setEphemeral] = useState(false)

  const keys = useQuery({ queryKey: ['preauth-keys'], queryFn: api.preAuthKeys })
  const users = useQuery({ queryKey: ['tailnet-users'], queryFn: api.tailnetUsers, enabled: admin })
  const headscaleKeys = useQuery({ queryKey: ['api-keys'], queryFn: api.apiKeys, enabled: admin })

  const create = useMutation({
    mutationFn: () =>
      api.createPreAuthKey({
        user: admin && user !== '' ? user : undefined,
        reusable,
        ephemeral,
        expiresIn: '24h',
      }),
    onSuccess: (res) => {
      setMinted({ command: res.command, warning: res.warning })
      void qc.invalidateQueries({ queryKey: ['preauth-keys'] })
    },
  })

  const createApi = useMutation({
    mutationFn: () => api.createApiKey('2160h'),
    onSuccess: (res) => {
      setApiKey(res)
      void qc.invalidateQueries({ queryKey: ['api-keys'] })
    },
  })

  return (
    <div className="space-y-6">
      <header>
        <h1 className="text-xl font-semibold">Keys</h1>
        <p className="text-sm text-muted-foreground">
          Pre-auth keys enrol devices. Secrets are shown once — Headscale stores only a hash.
        </p>
      </header>

      {(keys.error || headscaleKeys.error || create.error || createApi.error) && (
        <ErrorNote
          error={keys.error ?? headscaleKeys.error ?? create.error ?? createApi.error}
        />
      )}

      <Section title="Enrol a device">
        <div className="flex flex-wrap items-center gap-2">
          {admin && (
            <select
              value={user}
              onChange={(e) => setUser(e.target.value)}
              className="rounded-md border border-border bg-surface-1 px-2 py-1.5 text-sm"
            >
              <option value="">— choose a user —</option>
              {users.data?.users.map((u) => (
                <option key={u.id} value={u.name}>
                  {u.name}
                </option>
              ))}
            </select>
          )}
          <Toggle checked={reusable} onChange={setReusable} label="reusable" />
          <Toggle checked={ephemeral} onChange={setEphemeral} label="ephemeral" />
          <Button variant="primary" disabled={create.isPending} onClick={() => create.mutate()}>
            {create.isPending ? 'Creating…' : 'Create key'}
          </Button>
        </div>

        {minted && (
          <div className="space-y-2 rounded-md border border-warn/40 bg-warn/10 p-3">
            <p className="text-sm">{minted.warning}</p>
            <div className="flex items-center gap-2">
              <code className="flex-1 overflow-x-auto whitespace-nowrap rounded bg-surface-2 px-2 py-1.5 font-mono text-xs">
                {minted.command}
              </code>
              <Mono value={minted.command} className="shrink-0 border border-border px-2 py-1.5" />
            </div>
            <Button variant="ghost" onClick={() => setMinted(null)}>
              I&apos;ve copied it
            </Button>
          </div>
        )}
      </Section>

      <Section title="Pre-auth keys">
        {keys.isPending ? (
          <p className="text-sm text-muted-foreground">Loading keys…</p>
        ) : keys.data?.keys.length ? (
          <div className="overflow-x-auto rounded-lg border border-border">
            <table className="w-full min-w-max text-sm">
              <thead className="bg-surface-1 text-left text-xs uppercase tracking-wide text-muted-foreground">
                <tr>
                  <th className="px-3 py-2 font-medium">User</th>
                  <th className="px-3 py-2 font-medium">Flags</th>
                  <th className="px-3 py-2 font-medium">Tags</th>
                  <th className="px-3 py-2 font-medium">Expires</th>
                </tr>
              </thead>
              <tbody>
                {keys.data.keys.map((k) => (
                  <tr key={k.id} className="border-t border-border">
                    <td className="px-3 py-2">{k.user}</td>
                    <td className="px-3 py-2">
                      <div className="flex gap-1">
                        {k.reusable && <Badge>reusable</Badge>}
                        {k.ephemeral && <Badge>ephemeral</Badge>}
                        {k.used && <Badge tone="warn">used</Badge>}
                      </div>
                    </td>
                    <td className="px-3 py-2">
                      <div className="flex gap-1">
                        {(k.tags ?? []).map((t) => (
                          <Badge key={t} tone="accent">
                            {t}
                          </Badge>
                        ))}
                      </div>
                    </td>
                    <td className="px-3 py-2 text-muted-foreground">
                      {k.expiry ? new Date(k.expiry).toLocaleString() : '—'}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : (
          <Empty title="No pre-auth keys" />
        )}
      </Section>

      {admin && (
        <Section title="Headscale API keys">
          <div className="rounded-md border border-danger/30 bg-danger/5 px-3 py-2 text-sm">
            These are all-access admin credentials for the whole tailnet. There is no read-only
            scope on Headscale v0.29.
          </div>

          {apiKey && (
            <div className="space-y-2 rounded-md border border-warn/40 bg-warn/10 p-3">
              <p className="text-sm">{apiKey.warning}</p>
              <code className="block overflow-x-auto whitespace-nowrap rounded bg-surface-2 px-2 py-1.5 font-mono text-xs">
                {apiKey.key}
              </code>
              <Button variant="ghost" onClick={() => setApiKey(null)}>
                I&apos;ve copied it
              </Button>
            </div>
          )}

          <ul className="space-y-1">
            {headscaleKeys.data?.keys.map((k) => (
              <li
                key={k.id}
                className="flex items-center justify-between gap-3 rounded-md border border-border bg-surface-1 px-3 py-2"
              >
                <code className="font-mono text-xs">{k.prefix}…</code>
                <span className="text-xs text-muted-foreground">
                  {k.expiration ? `expires ${new Date(k.expiration).toLocaleDateString()}` : 'no expiry'}
                </span>
              </li>
            ))}
          </ul>

          <Button onClick={() => createApi.mutate()} disabled={createApi.isPending}>
            Mint an API key
          </Button>
        </Section>
      )}
    </div>
  )
}

function Toggle({
  checked,
  onChange,
  label,
}: {
  checked: boolean
  onChange: (v: boolean) => void
  label: string
}) {
  return (
    <label className="flex items-center gap-1.5 text-sm">
      <input
        type="checkbox"
        checked={checked}
        onChange={(e) => onChange(e.target.checked)}
        className="size-4 rounded border-border"
      />
      {label}
    </label>
  )
}
