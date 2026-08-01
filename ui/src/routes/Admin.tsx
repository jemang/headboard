import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, type Device, type Me, type Role, type TailnetUser } from '../lib/api'
import { Badge, Button, Empty, ErrorNote, Input, Mono, Section, Status } from '../components/ui'
import { Cell, Row, SpanRow, Table } from '../components/Table'
import { Loading, SkeletonRows } from '../components/Skeleton'
import { useToast } from '../components/Toast'
import { KeyRound, Plus, ShieldCheck, UserPlus, Users } from 'lucide-react'
import { expiryFromDays, isExpired } from '../lib/apiKeyControls'

const roles: Role[] = ['owner', 'admin', 'network-admin', 'auditor', 'member']

/** Headscale users and Headboard accounts, side by side — because linking them
 *  is the one thing that has to happen for a member to see anything. */
export function People({ me }: { me: Me }) {
  const qc = useQueryClient()
  const toast = useToast()
  const users = useQuery({ queryKey: ['tailnet-users'], queryFn: api.tailnetUsers })
  const accounts = useQuery({ queryKey: ['accounts'], queryFn: api.accounts })

  const [newUser, setNewUser] = useState('')

  const invalidate = () => {
    void qc.invalidateQueries({ queryKey: ['tailnet-users'] })
    void qc.invalidateQueries({ queryKey: ['accounts'] })
  }

  const create = useMutation({
    mutationFn: (name: string) => api.createTailnetUser({ name }),
    onSuccess: (u) => {
      setNewUser('')
      invalidate()
      toast.ok(`Created ${u.name}`)
    },
    onError: toast.error,
  })

  const link = useMutation({
    mutationFn: ({ id, hsID }: { id: number; hsID: number | null }) => api.linkAccount(id, hsID),
    onSuccess: (a) => {
      invalidate()
      toast.ok(a.headscaleUserId ? 'Account linked' : 'Account unlinked')
    },
    onError: toast.error,
  })

  const role = useMutation({
    mutationFn: ({ id, r }: { id: number; r: Role }) => api.setAccountRole(id, r),
    onSuccess: (a) => {
      invalidate()
      toast.ok(`${a.email} is now ${a.role}`)
    },
    onError: toast.error,
  })

  if (users.error) return <ErrorNote error={users.error} />

  return (
    <div className="space-y-6">
      <header>
        <p className="text-eyebrow font-semibold uppercase text-muted-foreground">Identity management</p>
        <h1 className="mt-1 text-display font-semibold">People</h1>
        <p className="text-sm text-muted-foreground">
          Headscale users own devices. Headboard accounts are OIDC identities with a role. They have
          to be linked, and a user created with the CLI has no OIDC identity to match.
        </p>
      </header>

      {accounts.error && <ErrorNote error={accounts.error} />}

      <Section
        title="Headboard accounts"
        actions={<span className="text-xs text-muted-foreground">who can sign in</span>}
      >
        <Table columns={['Identity', 'Role', 'Linked Headscale user']}>
          {accounts.isPending ? (
            <SpanRow columns={3}>
              <Loading label="Loading accounts">
                <SkeletonRows rows={2} cols={3} />
              </Loading>
            </SpanRow>
          ) : accounts.data?.accounts.length ? (
            accounts.data.accounts.map((a) => (
                  <Row key={a.id}>
                    <Cell>
                      <div className="font-medium">{a.displayName || a.email}</div>
                      <div className="text-xs text-muted-foreground">{a.email}</div>
                    </Cell>
                    <Cell>
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
                    </Cell>
                    <Cell>
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
                    </Cell>
                  </Row>
            ))
          ) : (
            <SpanRow columns={3}>
              <Empty
                icon={Users}
                title="No accounts yet"
                hint="The first person to sign in becomes the owner."
              />
            </SpanRow>
          )}
        </Table>
      </Section>

      <Section title="Headscale users" actions={<span className="text-xs text-muted-foreground">who owns devices</span>}>
        <Table columns={['User', { label: 'Devices', align: 'right' }, 'OIDC identity']}>
          {users.isPending ? (
            <SpanRow columns={3}>
              <Loading label="Loading users">
                <SkeletonRows rows={3} cols={3} />
              </Loading>
            </SpanRow>
          ) : (
            users.data?.users.map((u) => (
              <Row key={u.id}>
                <Cell>
                  <div className="font-medium">{u.name}</div>
                  {u.email && <div className="text-xs text-muted-foreground">{u.email}</div>}
                </Cell>
                <Cell align="right">{u.devices}</Cell>
                <Cell>
                  {u.providerId ? (
                    <Status ok label="matched automatically" />
                  ) : (
                    <Status ok={false} warn label="none — must be linked by hand" />
                  )}
                </Cell>
              </Row>
            ))
          )}
        </Table>

        <div className="flex items-center gap-2">
          <Input value={newUser} onChange={setNewUser} placeholder="new username" className="w-56" />
          <Button icon={UserPlus} disabled={newUser === ''} onClick={() => create.mutate(newUser)}>
            Create user
          </Button>
        </div>
      </Section>
    </div>
  )
}

export function Keys({ me }: { me: Me }) {
  const qc = useQueryClient()
  const toast = useToast()
  const admin = me.capabilities.includes('manage:keys')

  const [minted, setMinted] = useState<{
    command: string
    warning: string
    loginServer: string
    loginServerProblem?: string
  } | null>(null)
  const [apiKey, setApiKey] = useState<{ key: string; warning: string } | null>(null)
  const [user, setUser] = useState('')
  const [reusable, setReusable] = useState(false)
  const [ephemeral, setEphemeral] = useState(false)
  const [apiKeyLifetime, setApiKeyLifetime] = useState('90')
  const [customApiKeyDays, setCustomApiKeyDays] = useState('')
  const [confirmApiKey, setConfirmApiKey] = useState(false)

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
      setMinted({
        command: res.command,
        warning: res.warning,
        loginServer: res.loginServer,
        loginServerProblem: res.loginServerProblem,
      })
      void qc.invalidateQueries({ queryKey: ['preauth-keys'] })
    },
    onError: toast.error,
  })

  const apiKeyDays = apiKeyLifetime === 'custom' ? customApiKeyDays : apiKeyLifetime
  const apiKeyExpiry = expiryFromDays(apiKeyDays)

  const createApi = useMutation({
    mutationFn: () => api.createApiKey(apiKeyExpiry),
    onSuccess: (res) => {
      setApiKey(res)
      setConfirmApiKey(false)
      void qc.invalidateQueries({ queryKey: ['api-keys'] })
    },
    onError: toast.error,
  })

  const revokeApi = useMutation({
    mutationFn: (prefix: string) => api.expireApiKey(prefix),
    onSuccess: () => {
      toast.ok('API key revoked')
      void qc.invalidateQueries({ queryKey: ['api-keys'] })
    },
    onError: toast.error,
  })

  return (
    <div className="space-y-6">
      <header>
        <p className="text-eyebrow font-semibold uppercase text-muted-foreground">Secure enrolment</p>
        <h1 className="mt-1 text-display font-semibold">Keys</h1>
        <p className="text-sm text-muted-foreground">
          Pre-auth keys enrol devices. Secrets are shown once — Headscale stores only a hash.
        </p>
      </header>

      {(keys.error || headscaleKeys.error) && (
        <ErrorNote error={keys.error ?? headscaleKeys.error} />
      )}

      <Section title="Enrol a device" actions={<span className="text-xs text-muted-foreground">Create a pre-auth key</span>}>
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
          <Button
            variant="primary"
            icon={Plus}
            disabled={create.isPending}
            onClick={() => create.mutate()}
          >
            {create.isPending ? 'Creating…' : 'Create key'}
          </Button>
        </div>

        {minted && (
          <div className="space-y-2 rounded-lg border border-warn/40 bg-warn/10 p-3">
            <p className="text-sm">{minted.warning}</p>
            <div className="flex items-center gap-2">
              <code className="flex-1 overflow-x-auto whitespace-nowrap rounded bg-surface-2 px-2 py-1.5 font-mono text-xs">
                {minted.command}
              </code>
              <Mono
                compact
                label="the enrolment command"
                value={minted.command}
                className="shrink-0 border border-border px-2 py-1.5"
              />
            </div>
            <p className="text-xs text-muted-foreground">
              Enrols against <span className="font-mono">{minted.loginServer}</span>
            </p>

            {minted.loginServerProblem && (
              <div className="rounded-md border border-danger/40 bg-danger/10 px-3 py-2 text-xs">
                That address {minted.loginServerProblem}, so this command will not work on another
                machine. Set <code>HEADSCALE_PUBLIC_URL</code> to the address devices use to reach
                Headscale.
              </div>
            )}

            <Button variant="ghost" onClick={() => setMinted(null)}>
              I&apos;ve copied it
            </Button>
          </div>
        )}
      </Section>

      {admin && <ApproveRegistration users={users.data?.users ?? []} />}

      <Section title="Pre-auth keys">
        <Table columns={['User', 'Flags', 'Tags', 'Expires']}>
          {keys.isPending ? (
            <SpanRow columns={4}>
              <Loading label="Loading keys">
                <SkeletonRows rows={2} cols={4} />
              </Loading>
            </SpanRow>
          ) : keys.data?.keys.length ? (
            keys.data.keys.map((k) => (
              <Row key={k.id}>
                <Cell>{k.user}</Cell>
                <Cell>
                  <div className="flex gap-1">
                    {k.reusable && <Badge>reusable</Badge>}
                    {k.ephemeral && <Badge>ephemeral</Badge>}
                    {k.used && <Badge tone="warn">used</Badge>}
                  </div>
                </Cell>
                <Cell>
                  <div className="flex gap-1">
                    {(k.tags ?? []).map((t) => (
                      <Badge key={t} tone="accent">
                        {t}
                      </Badge>
                    ))}
                  </div>
                </Cell>
                <Cell muted>{k.expiry ? new Date(k.expiry).toLocaleString() : '—'}</Cell>
              </Row>
            ))
          ) : (
            <SpanRow columns={4}>
              <Empty
                icon={KeyRound}
                title="No pre-auth keys"
                hint="Create one above to enrol a device."
              />
            </SpanRow>
          )}
        </Table>
      </Section>

      {admin && (
        <Section title="Headscale API keys">
          <div className="rounded-lg border border-danger/30 bg-danger/5 px-3 py-2 text-sm">
            Use an API key only for trusted server-to-server automation. It grants full administrator
            access to this tailnet, cannot enrol a device, and has no read-only scope on Headscale v0.29.
          </div>

          <div className="flex flex-wrap items-end gap-2">
            <label className="block">
              <span className="text-xs text-muted-foreground">key lifetime</span>
              <select
                value={apiKeyLifetime}
                onChange={(event) => {
                  setApiKeyLifetime(event.target.value)
                  setConfirmApiKey(false)
                }}
                className="mt-1 block rounded-md border border-border bg-surface-1 px-2 py-1.5 text-sm"
              >
                <option value="30">30 days</option>
                <option value="90">90 days</option>
                <option value="180">180 days</option>
                <option value="365">365 days</option>
                <option value="custom">Custom…</option>
              </select>
            </label>
            {apiKeyLifetime === 'custom' && (
              <label className="block">
                <span className="text-xs text-muted-foreground">days</span>
                <Input
                  value={customApiKeyDays}
                  onChange={(value) => {
                    setCustomApiKeyDays(value)
                    setConfirmApiKey(false)
                  }}
                  placeholder="e.g. 45"
                  className="mt-1 w-28"
                />
              </label>
            )}
            <Button
              icon={KeyRound}
              disabled={!apiKeyExpiry || createApi.isPending}
              onClick={() => setConfirmApiKey(true)}
            >
              Continue
            </Button>
          </div>

          {apiKeyLifetime === 'custom' && customApiKeyDays !== '' && !apiKeyExpiry && (
            <p className="text-xs text-danger">Enter a positive whole number of days.</p>
          )}

          {confirmApiKey && apiKeyExpiry && (
            <div className="flex flex-wrap items-center gap-2 rounded-lg border border-warn/40 bg-warn/10 p-3 text-sm">
              <p className="flex-1">
                This will create a full-access API key that expires in <strong>{apiKeyDays} days</strong>.
                It is shown once and cannot be retrieved again.
              </p>
              <Button variant="primary" icon={KeyRound} disabled={createApi.isPending} onClick={() => createApi.mutate()}>
                {createApi.isPending ? 'Minting…' : 'Mint API key'}
              </Button>
              <Button variant="ghost" onClick={() => setConfirmApiKey(false)}>Cancel</Button>
            </div>
          )}

          {apiKey && (
            <div className="space-y-2 rounded-md border border-warn/40 bg-warn/10 p-3">
              <p className="text-sm">{apiKey.warning}</p>
              <div className="flex items-center gap-2">
                <code className="flex-1 overflow-x-auto whitespace-nowrap rounded bg-surface-2 px-2 py-1.5 font-mono text-xs">
                  {apiKey.key}
                </code>
                <Mono
                  compact
                  label="the API key"
                  value={apiKey.key}
                  className="shrink-0 border border-border px-2 py-1.5"
                />
              </div>
              <Button variant="ghost" onClick={() => setApiKey(null)}>
                I&apos;ve copied it
              </Button>
            </div>
          )}

          <ul className="space-y-1">
            {headscaleKeys.data?.keys.map((k) => {
              const expired = isExpired(k.expiration)

              return (
                <li
                  key={k.id}
                  className="flex flex-wrap items-center justify-between gap-3 rounded-md border border-border bg-surface-1 px-3 py-2"
                >
                  <code className="font-mono text-xs">{k.prefix}…</code>
                  <div className="ml-auto flex items-center gap-2">
                    {k.protected ? (
                      <Badge tone="accent">used by Headboard</Badge>
                    ) : expired ? (
                      <Badge tone="warn">expired</Badge>
                    ) : (
                      <span className="text-xs text-muted-foreground">
                        {k.expiration ? `expires ${new Date(k.expiration).toLocaleDateString()}` : 'no expiry'}
                      </span>
                    )}
                    {!expired && !k.protected && (
                      <Button
                        variant="ghost"
                        disabled={revokeApi.isPending}
                        onClick={() => {
                          if (window.confirm(`Revoke API key ${k.prefix}? Any automation using it will stop immediately.`)) {
                            revokeApi.mutate(k.prefix)
                          }
                        }}
                      >
                        Revoke
                      </Button>
                    )}
                  </div>
                </li>
              )
            })}
          </ul>
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

/**
 * Approve a device that started registering itself.
 *
 * A form rather than an inbox, because Headscale v0.29.3 offers approve, reject
 * and register by auth id and nothing that enumerates what is pending. The id
 * travels out of band: the Tailscale client prints it on the device, and the
 * person enrolling passes it on. Headscale main's /api/v2 is expected to expose
 * the queue, at which point this becomes a list.
 */
function ApproveRegistration({ users }: { users: TailnetUser[] }) {
  const qc = useQueryClient()
  const [authId, setAuthId] = useState('')
  const [user, setUser] = useState('')
  const [approved, setApproved] = useState<Device | null>(null)

  const invalidate = () => {
    void qc.invalidateQueries({ queryKey: ['devices'] })
    setAuthId('')
  }

  const approve = useMutation({
    mutationFn: () => api.approveRegistration(authId.trim(), user || undefined),
    onSuccess: (res) => {
      setApproved(res.device ?? null)
      invalidate()
    },
  })

  const reject = useMutation({
    mutationFn: () => api.rejectRegistration(authId.trim()),
    onSuccess: () => {
      setApproved(null)
      invalidate()
    },
  })

  const ready = authId.trim().length > 13

  return (
    <Section title="Approve a device">
      <p className="text-sm text-muted-foreground">
        When someone runs <code>tailscale up</code> without a pre-auth key, the device prints a URL
        containing an id like <code>hskey-authreq-…</code>. Paste it here to let the device in.
      </p>

      {(approve.error || reject.error) && <ErrorNote error={approve.error ?? reject.error} />}

      {approved && (
        <div className="rounded-md border border-ok/40 bg-ok/10 px-3 py-2 text-sm">
          Approved <span className="font-medium">{approved.name}</span> — it is in Devices now.
        </div>
      )}

      <div className="flex flex-wrap items-end gap-2">
        <div className="min-w-72 flex-1">
          <span className="text-xs text-muted-foreground">Registration id</span>
          <Input value={authId} onChange={setAuthId} placeholder="hskey-authreq-…" />
        </div>

        <div>
          <span className="text-xs text-muted-foreground">Assign to</span>
          <select
            value={user}
            onChange={(e) => setUser(e.target.value)}
            className="block rounded-md border border-border bg-surface-1 px-2 py-1.5 text-sm"
          >
            <option value="">— as requested —</option>
            {users.map((u) => (
              <option key={u.id} value={u.name}>
                {u.name}
              </option>
            ))}
          </select>
        </div>

        <Button
          variant="primary"
          icon={ShieldCheck}
          disabled={!ready || approve.isPending}
          onClick={() => approve.mutate()}
        >
          {approve.isPending ? 'Approving…' : 'Approve'}
        </Button>

        <Button variant="ghost" disabled={!ready || reject.isPending} onClick={() => reject.mutate()}>
          Reject
        </Button>
      </div>
    </Section>
  )
}
