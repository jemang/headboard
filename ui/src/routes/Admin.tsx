import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, type Account, type Device, type Me, type Role, type TailnetUser } from '../lib/api'
import { Badge, Button, Confirm, Drawer, Empty, ErrorNote, Input, Mono, Section, Status } from '../components/ui'
import { Cell, Row, SpanRow, Table } from '../components/Table'
import { Loading, SkeletonRows } from '../components/Skeleton'
import { useToast } from '../components/Toast'
import { Check, KeyRound, Pencil, ShieldCheck, Trash2, UserPlus, Users, X } from 'lucide-react'
import { expiryFromDays, isExpired } from '../lib/apiKeyControls'
import { preAuthHardening } from '../lib/preAuthHardening'
import { serverRegistrationCommand, type ServerRegistrationKind } from '../lib/serverRegistrationCommand'
import { Link } from '../lib/router'
import { devicesForOwner } from '../lib/ownerDevices'
import { duplicateUserNames } from '../lib/duplicateUserNames'
import { TagSelector } from '../components/TagSelector'

const roles: Role[] = ['owner', 'admin', 'network-admin', 'auditor', 'member']

/** Headscale users and Headboard accounts, side by side — because linking them
 *  is the one thing that has to happen for a member to see anything. */
export function People({ me }: { me: Me }) {
  const qc = useQueryClient()
  const toast = useToast()
  const users = useQuery({ queryKey: ['tailnet-users'], queryFn: api.tailnetUsers })
  const accounts = useQuery({ queryKey: ['accounts'], queryFn: api.accounts })
  const devices = useQuery({ queryKey: ['devices'], queryFn: api.devices })

  const [newUser, setNewUser] = useState('')
  const [selectedUserID, setSelectedUserID] = useState<number | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<TailnetUser | null>(null)

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

  const remove = useMutation({
    mutationFn: (id: number) => api.deleteTailnetUser(id),
    onSuccess: () => {
      invalidate()
      toast.ok(`Deleted ${deleteTarget?.name ?? 'user'}`)
      setDeleteTarget(null)
      setSelectedUserID(null)
    },
    onError: toast.error,
  })

  const renameUser = useMutation({
    mutationFn: ({ id, name }: { id: number; name: string }) => api.renameTailnetUser(id, name),
    onSuccess: (u) => {
      invalidate()
      toast.ok(`Renamed to ${u.name}`)
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
  const admission = useMutation({
    mutationFn: ({ id, state }: { id: number; state: 'active' | 'rejected' }) => api.setAccountAdmission(id, state),
    onSuccess: (account) => { invalidate(); toast.ok(`${account.email} is ${account.admission}`) },
    onError: toast.error,
  })

  if (users.error) return <ErrorNote error={users.error} />

  const duplicateNames = duplicateUserNames(users.data?.users ?? [])

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
        <Table columns={['Identity', 'Admission', 'Role', 'Linked Headscale user']}>
          {accounts.isPending ? (
            <SpanRow columns={4}>
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
                      <div className="flex flex-wrap items-center gap-2"><Badge tone={a.admission === 'active' ? 'accent' : a.admission === 'pending' ? 'warn' : 'danger'}>{a.admission}</Badge>
                      {a.admission === 'pending' && a.id !== me.user.id && <><Button variant="ghost" disabled={admission.isPending} onClick={() => admission.mutate({ id: a.id, state: 'active' })}>Approve</Button><Button variant="ghost" disabled={admission.isPending} onClick={() => admission.mutate({ id: a.id, state: 'rejected' })}>Reject</Button></>}</div>
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
            <SpanRow columns={4}>
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
              <Row key={u.id} onClick={() => setSelectedUserID(u.id)} label={`Open ${u.name}`}>
                <Cell>
                  <div className="flex items-center gap-2">
                    <div className="font-medium">{u.name}</div>
                    {duplicateNames.has(u.name.toLowerCase()) && (
                      <Badge tone="danger">duplicate name</Badge>
                    )}
                  </div>
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

      <UserDrawer
        user={users.data?.users.find((u) => u.id === selectedUserID)}
        duplicateName={selectedUserID !== null && duplicateNames.has(
          (users.data?.users.find((u) => u.id === selectedUserID)?.name ?? '').toLowerCase(),
        )}
        linkedAccount={accounts.data?.accounts.find((a) => a.headscaleUserId === selectedUserID)}
        accounts={accounts.data?.accounts ?? []}
        devices={devicesForOwner(devices.data?.devices ?? [], selectedUserID ?? -1)}
        devicesPending={devices.isPending}
        devicesError={devices.error}
        rename={renameUser}
        link={link}
        onDelete={(u) => setDeleteTarget(u)}
        onClose={() => setSelectedUserID(null)}
      />

      <Confirm
        open={deleteTarget !== null}
        title={`Delete ${deleteTarget?.name ?? 'this user'}?`}
        body={
          <>
            <span className="font-mono">{deleteTarget?.name}</span> will be removed from Headscale.
            This only works while they own no devices.
          </>
        }
        confirmLabel="Delete user"
        onConfirm={() => deleteTarget && remove.mutate(deleteTarget.id)}
        onCancel={() => setDeleteTarget(null)}
      />
    </div>
  )
}

function UserDrawer({
  user,
  duplicateName,
  linkedAccount,
  accounts,
  devices,
  devicesPending,
  devicesError,
  rename,
  link,
  onDelete,
  onClose,
}: {
  user: TailnetUser | undefined
  duplicateName: boolean
  linkedAccount: Account | undefined
  accounts: Account[]
  devices: Device[]
  devicesPending: boolean
  devicesError: unknown
  rename: { mutate: (vars: { id: number; name: string }) => void }
  link: { mutate: (vars: { id: number; hsID: number | null }) => void }
  onDelete: (user: TailnetUser) => void
  onClose: () => void
}) {
  const [renaming, setRenaming] = useState<string | null>(null)

  return (
    <Drawer
      open={user !== undefined}
      onClose={onClose}
      title={user?.name ?? 'User'}
      subtitle={user?.email && <span className="text-xs">{user.email}</span>}
    >
      {user && (
        <div className="space-y-6">
          <dl className="grid grid-cols-2 gap-3 text-sm">
            <Field label="Name">
              <span className="flex items-center gap-2">
                {user.name}
                {duplicateName && <Badge tone="danger">duplicate name</Badge>}
              </span>
            </Field>
            <Field label="Display name">{user.displayName || '—'}</Field>
            <Field label="Devices">{user.devices}</Field>
            <Field label="OIDC identity">
              {user.providerId ? (
                <Status ok label="matched automatically" />
              ) : (
                <Status ok={false} warn label="none — must be linked by hand" />
              )}
            </Field>
            <Field label="Linked Headboard account">
              <select
                value={linkedAccount?.id ?? ''}
                onChange={(e) => {
                  if (!user) return

                  if (e.target.value === '') {
                    if (linkedAccount) link.mutate({ id: linkedAccount.id, hsID: null })
                  } else {
                    link.mutate({ id: Number(e.target.value), hsID: user.id })
                  }
                }}
                className="rounded-md border border-border bg-surface-1 px-2 py-1 text-sm"
              >
                <option value="">— not linked —</option>
                {accounts.map((a) => (
                  <option key={a.id} value={a.id}>
                    {a.email} ({a.role})
                  </option>
                ))}
              </select>
            </Field>
          </dl>

          <Section title="Actions">
            <div className="flex flex-wrap gap-2">
              {renaming === null ? (
                <Button icon={Pencil} onClick={() => setRenaming(user.name)}>
                  Rename
                </Button>
              ) : (
                <div className="flex w-full gap-2">
                  <Input value={renaming} onChange={setRenaming} autoFocus className="flex-1" />
                  <Button
                    variant="primary"
                    icon={Check}
                    onClick={() => { rename.mutate({ id: user.id, name: renaming }); setRenaming(null) }}
                  >
                    Save
                  </Button>
                  <Button variant="ghost" icon={X} onClick={() => setRenaming(null)}>
                    Cancel
                  </Button>
                </div>
              )}

              <Button
                variant="danger"
                icon={Trash2}
                disabled={user.devices > 0}
                title={user.devices > 0 ? `${user.name} still owns devices; remove or reassign them first` : undefined}
                onClick={() => onDelete(user)}
              >
                Delete user
              </Button>
            </div>
          </Section>

          <Section title="Devices">
            <OwnerDevices name={user.name} devices={devices} pending={devicesPending} error={devicesError} />
          </Section>
        </div>
      )}
    </Drawer>
  )
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div>
      <dt className="text-xs uppercase tracking-wide text-muted-foreground">{label}</dt>
      <dd className="mt-0.5">{children}</dd>
    </div>
  )
}

function OwnerDevices({
  name,
  devices,
  pending,
  error,
}: {
  name: string
  devices: Device[]
  pending: boolean
  error: unknown
}) {
  if (pending) {
    return (
      <Loading label={`Loading ${name}'s devices`}>
        <SkeletonRows rows={2} cols={3} />
      </Loading>
    )
  }
  if (error) return <ErrorNote error={error} />

  return (
    <div className="space-y-3 text-left">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="text-sm text-muted-foreground">
          Devices owned by <span className="font-medium text-foreground">{name}</span>
        </p>
        <Link to="/devices" className="text-sm font-medium text-accent-700 underline underline-offset-2 dark:text-accent-400">
          View full inventory
        </Link>
      </div>
      {devices.length === 0 ? (
        <Empty icon={Users} title="No devices found" hint="The inventory changed while this view was open." />
      ) : (
        <div className="grid gap-3 md:grid-cols-2">
          {devices.map((device) => (
            <article key={device.id} className="rounded-lg border border-border bg-surface-0 p-3">
              <div className="flex items-start justify-between gap-3">
                <div>
                  <p className="font-medium">{device.name}</p>
                  <div className="mt-1 flex flex-wrap gap-1">
                    {device.tags?.map((tag) => <Badge key={tag} tone="accent">{tag}</Badge>)}
                  </div>
                </div>
                <Status ok={device.online} label={device.online ? 'online' : 'offline'} />
              </div>
              <div className="mt-3 flex flex-wrap gap-1">
                {device.ips.map((ip) => <Mono key={ip} value={ip} />)}
              </div>
            </article>
          ))}
        </div>
      )}
    </div>
  )
}

export function Keys({ me }: { me: Me }) {
  const qc = useQueryClient()
  const toast = useToast()
  const admin = me.capabilities.includes('manage:keys')
  const deviceManager = me.capabilities.includes('manage:devices')

  const [apiKey, setApiKey] = useState<{ key: string; warning: string } | null>(null)
  const [apiKeyLifetime, setApiKeyLifetime] = useState('90')
  const [customApiKeyDays, setCustomApiKeyDays] = useState('')
  const [confirmApiKey, setConfirmApiKey] = useState(false)
  const [confirmRevokeKeys, setConfirmRevokeKeys] = useState(false)

  const keys = useQuery({ queryKey: ['preauth-keys'], queryFn: api.preAuthKeys, enabled: admin })
  const users = useQuery({ queryKey: ['tailnet-users'], queryFn: api.tailnetUsers, enabled: admin })
  const headscaleKeys = useQuery({ queryKey: ['api-keys'], queryFn: api.apiKeys, enabled: admin })

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
  const revokePreAuth = useMutation({
    mutationFn: api.revokeActivePreAuthKeys,
    onSuccess: (result) => {
      setConfirmRevokeKeys(false)
      void qc.invalidateQueries({ queryKey: ['preauth-keys'] })
      result.failed.length ? toast.error(`${result.failed.length} key revocations failed`) : toast.ok(`${result.expired.length} active pre-auth keys revoked`)
    },
    onError: toast.error,
  })
  const hardening = preAuthHardening(keys.data?.keys ?? [])

  return (
    <div className="space-y-6">
      <header>
        <p className="text-eyebrow font-semibold uppercase text-muted-foreground">Secure enrolment</p>
        <h1 className="mt-1 text-display font-semibold">Keys</h1>
        <p className="text-sm text-muted-foreground">
          Every device requires explicit approval. Pre-auth keys are disabled.
        </p>
      </header>

      {(keys.error || headscaleKeys.error) && (
        <ErrorNote error={keys.error ?? headscaleKeys.error} />
      )}

      <EnrolDevice />

      {deviceManager && <ApproveRegistration users={users.data?.users ?? []} />}

      {admin && !hardening.compliant && (
        <Section title="Automatic enrolment risk">
          <div className="space-y-3 rounded-lg border border-danger/40 bg-danger/10 p-3 text-sm">
            <p>{hardening.active.length} active pre-auth key{hardening.active.length === 1 ? '' : 's'} can still enrol a device without approval.</p>
            {confirmRevokeKeys ? (
              <div className="flex flex-wrap gap-2"><Button variant="primary" disabled={revokePreAuth.isPending} onClick={() => revokePreAuth.mutate()}>{revokePreAuth.isPending ? 'Revoking…' : 'Revoke all active keys'}</Button><Button variant="ghost" onClick={() => setConfirmRevokeKeys(false)}>Cancel</Button></div>
            ) : <Button onClick={() => setConfirmRevokeKeys(true)}>Continue to revoke</Button>}
          </div>
        </Section>
      )}

      {admin && <Section title="Pre-auth keys">
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
      </Section>}

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

/**
 * Approve a device that started registering itself.
 *
 * A form rather than an inbox, because Headscale v0.29.3 offers approve, reject
 * and register by auth id and nothing that enumerates what is pending. The id
 * travels out of band: the Tailscale client prints it on the device, and the
 * person enrolling passes it on. Headscale main's /api/v2 is expected to expose
 * the queue, at which point this becomes a list.
 */
function EnrolDevice() {
  const registrationInfo = useQuery({ queryKey: ['registration-info'], queryFn: api.registrationInfo })
  const command = registrationInfo.data?.headscalePublicUrl
    ? serverRegistrationCommand(registrationInfo.data.headscalePublicUrl, 'standard', '')
    : ''

  return (
    <Section title="Enrol a device" actions={<span className="text-xs text-muted-foreground">manual approval required</span>}>
      <p className="text-sm text-muted-foreground">
        Run this on your server. It creates a pending request only; send its <code>hskey-authreq-…</code> ID to an owner, admin, or network admin for approval.
      </p>
      {registrationInfo.error && <ErrorNote error={registrationInfo.error} />}
      {command ? (
        <div className="flex items-center gap-2 rounded-md bg-surface-2 px-2 py-1.5">
          <code className="min-w-0 flex-1 overflow-x-auto whitespace-nowrap font-mono text-xs">{command}</code>
          <Mono compact label="device enrolment command" value={command} className="shrink-0 border border-border px-2 py-1.5" />
        </div>
      ) : !registrationInfo.error && <p className="text-xs text-muted-foreground">Loading the Headscale address…</p>}
    </Section>
  )
}

function ApproveRegistration({ users }: { users: TailnetUser[] }) {
  const qc = useQueryClient()
  const [authId, setAuthId] = useState('')
  const [user, setUser] = useState('')
  const [approved, setApproved] = useState<Device | null>(null)
  const [serverKind, setServerKind] = useState<ServerRegistrationKind>('tagged')
  const [serverValue, setServerValue] = useState('')
  const [selectedTags, setSelectedTags] = useState<string[]>([])
  const registrationInfo = useQuery({ queryKey: ['registration-info'], queryFn: api.registrationInfo })
  const policy = useQuery({ queryKey: ['policy'], queryFn: api.policy })

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
  const needsServerValue = serverKind === 'tagged' || serverKind === 'subnet'
  const commandValue = serverKind === 'tagged' ? selectedTags.join(',') : serverValue.trim()
  const command = registrationInfo.data?.headscalePublicUrl && (!needsServerValue || commandValue)
    ? serverRegistrationCommand(registrationInfo.data.headscalePublicUrl, serverKind, commandValue)
    : ''

  return (
    <Section title="Advanced registration and approval">
      <p className="text-sm text-muted-foreground">
        Generate a tagged-server, subnet-router, or exit-node command. Running it creates a pending
        request; it cannot join until you paste its <code>hskey-authreq-…</code> ID below and approve it.
      </p>

      {(approve.error || reject.error || registrationInfo.error || policy.error) && <ErrorNote error={approve.error ?? reject.error ?? registrationInfo.error ?? policy.error} />}

      <div className="space-y-3 rounded-lg border border-border bg-surface-0 p-3">
        <div className="flex flex-wrap items-end gap-2">
          <label>
            <span className="text-xs text-muted-foreground">Server type</span>
            <select
              value={serverKind}
              onChange={(e) => {
                setServerKind(e.target.value as ServerRegistrationKind)
                setServerValue('')
                setSelectedTags([])
              }}
              className="mt-1 block rounded-md border border-border bg-surface-1 px-2 py-1.5 text-sm"
            >
              <option value="tagged">Tagged server</option>
              <option value="subnet">Subnet router</option>
              <option value="exit">Exit node</option>
            </select>
          </label>
          {serverKind === 'tagged' && (
            <label className="min-w-64 flex-1">
              <span className="text-xs text-muted-foreground">Tags declared in tag owners</span>
              {policy.isPending
                ? <p className="mt-1 text-xs text-muted-foreground">Loading saved tag owners…</p>
                : <TagSelector tags={policy.data?.tokens.tags ?? []} value={selectedTags} onChange={setSelectedTags} />}
            </label>
          )}
          {serverKind === 'subnet' && (
            <label className="min-w-64 flex-1">
              <span className="text-xs text-muted-foreground">Subnet routes</span>
              <Input value={serverValue} onChange={setServerValue} placeholder="10.0.0.0/24,192.168.1.0/24" className="mt-1" />
            </label>
          )}
        </div>

        {command ? (
          <div className="flex items-center gap-2 rounded-md bg-surface-2 px-2 py-1.5">
            <code className="min-w-0 flex-1 overflow-x-auto whitespace-nowrap font-mono text-xs">{command}</code>
            <Mono compact label="server registration command" value={command} className="shrink-0 border border-border px-2 py-1.5" />
          </div>
        ) : (
          <p className="text-xs text-muted-foreground">
            {needsServerValue
              ? serverKind === 'tagged' ? 'Select at least one saved tag to generate the command.' : 'Enter subnet routes to generate the command.'
              : 'Loading the Headscale address…'}
          </p>
        )}
      </div>

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
