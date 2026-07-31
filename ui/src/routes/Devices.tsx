import { useEffect, useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, type Device, type EffectiveRule, type Me } from '../lib/api'
import { Badge, Button, Confirm, Drawer, Empty, ErrorNote, Input, Mono, Section, Status } from '../components/ui'

export function Devices({
  me,
  focus,
  onFocused,
}: {
  me: Me
  /** A device the palette asked to open, or null. */
  focus?: number | null
  onFocused?: () => void
}) {
  const [selected, setSelected] = useState<number | null>(null)
  const [filter, setFilter] = useState('')
  const [onlineOnly, setOnlineOnly] = useState(false)

  // Cleared as soon as it is consumed, so closing the drawer and pressing ⌘K
  // for the same machine opens it again rather than doing nothing.
  useEffect(() => {
    if (focus == null) return

    setSelected(focus)
    onFocused?.()
  }, [focus, onFocused])

  const devices = useQuery({ queryKey: ['devices'], queryFn: api.devices })

  const admin = me.capabilities.includes('view:all')

  const rows = useMemo(() => {
    const all = devices.data?.devices ?? []
    const q = filter.toLowerCase()

    return all.filter((d) => {
      if (onlineOnly && !d.online) return false
      if (q === '') return true

      return (
        d.name.toLowerCase().includes(q) ||
        (d.owner ?? '').toLowerCase().includes(q) ||
        (d.tags ?? []).some((t) => t.toLowerCase().includes(q)) ||
        d.ips.some((ip) => ip.includes(q))
      )
    })
  }, [devices.data, filter, onlineOnly])

  if (devices.isPending) return <p className="text-sm text-muted-foreground">Loading devices…</p>
  if (devices.error) return <ErrorNote error={devices.error} />

  return (
    <div className="space-y-4">
      <header className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold">{admin ? 'Devices' : 'My devices'}</h1>
          <p className="text-sm text-muted-foreground">
            {admin
              ? 'Every machine in the tailnet.'
              : 'Your machines, and the rules that actually apply to them.'}
          </p>
        </div>
        <div className="flex items-center gap-2">
          <Input value={filter} onChange={setFilter} placeholder="Filter…" className="w-56" />
          <Button variant={onlineOnly ? 'primary' : 'default'} onClick={() => setOnlineOnly((v) => !v)}>
            Online only
          </Button>
        </div>
      </header>

      {!me.linked && !admin && (
        <div className="rounded-md border border-warn/40 bg-warn/10 px-3 py-2 text-sm">
          Your account is not linked to a Headscale user yet, so there is nothing to show. An
          administrator has to link it.
        </div>
      )}

      {rows.length === 0 ? (
        <Empty title="No devices" hint={filter ? 'Nothing matches that filter.' : undefined} />
      ) : (
        <div className="overflow-x-auto rounded-lg border border-border">
          <table className="w-full min-w-max text-sm">
            <thead className="bg-surface-1 text-left text-xs uppercase tracking-wide text-muted-foreground">
              <tr>
                <th className="px-3 py-2 font-medium">Device</th>
                <th className="px-3 py-2 font-medium">Addresses</th>
                <th className="px-3 py-2 font-medium">Owner</th>
                <th className="px-3 py-2 font-medium">Status</th>
                <th className="px-3 py-2 font-medium">Routes</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((d) => (
                <tr
                  key={d.id}
                  onClick={() => setSelected(d.id)}
                  className="cursor-pointer border-t border-border hover:bg-surface-1"
                >
                  <td className="px-3 py-2">
                    <div className="font-medium">{d.name}</div>
                    {(d.tags ?? []).length > 0 && (
                      <div className="mt-0.5 flex gap-1">
                        {d.tags?.map((t) => (
                          <Badge key={t} tone="accent">
                            {t}
                          </Badge>
                        ))}
                      </div>
                    )}
                  </td>
                  <td className="px-3 py-2">
                    <div className="flex flex-col gap-0.5">
                      {d.ips.map((ip) => (
                        <Mono key={ip} value={ip} />
                      ))}
                    </div>
                  </td>
                  <td className="px-3 py-2 text-muted-foreground">
                    {d.owner ?? <span className="text-xs">tagged</span>}
                  </td>
                  <td className="px-3 py-2">
                    <div className="flex flex-col gap-1">
                      <Status ok={d.online} label={d.online ? 'online' : 'offline'} />
                      {d.expired && <Status ok={false} warn label="key expired" />}
                    </div>
                  </td>
                  <td className="px-3 py-2">
                    <div className="flex flex-wrap gap-1">
                      {d.exitNode && <Badge tone="accent">exit node</Badge>}
                      {(d.subnetRoutes ?? []).map((r) => (
                        <Badge key={r}>{r}</Badge>
                      ))}
                      {(d.advertisedRoutes ?? []).filter(
                        (r) => !(d.approvedRoutes ?? []).includes(r),
                      ).length > 0 && <Badge tone="warn">awaiting approval</Badge>}
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      <DeviceDrawer id={selected} me={me} onClose={() => setSelected(null)} />
    </div>
  )
}

function DeviceDrawer({ id, me, onClose }: { id: number | null; me: Me; onClose: () => void }) {
  const qc = useQueryClient()
  const [confirmDelete, setConfirmDelete] = useState(false)
  const [renaming, setRenaming] = useState<string | null>(null)
  const [tab, setTab] = useState<'inbound' | 'outbound' | 'peers'>('inbound')

  const device = useQuery({
    queryKey: ['device', id],
    queryFn: () => api.device(id as number),
    enabled: id !== null,
  })

  const rules = useQuery({
    queryKey: ['device-rules', id],
    queryFn: () => api.deviceRules(id as number),
    enabled: id !== null,
  })

  const invalidate = () => {
    void qc.invalidateQueries({ queryKey: ['devices'] })
    void qc.invalidateQueries({ queryKey: ['device', id] })
    void qc.invalidateQueries({ queryKey: ['device-rules', id] })
  }

  const rename = useMutation({
    mutationFn: (name: string) => api.renameDevice(id as number, name),
    onSuccess: () => {
      setRenaming(null)
      invalidate()
    },
  })

  const routes = useMutation({
    mutationFn: (next: string[]) => api.approveRoutes(id as number, next),
    onSuccess: invalidate,
  })

  const expire = useMutation({
    mutationFn: () => api.expireDevice(id as number),
    onSuccess: invalidate,
  })

  const remove = useMutation({
    mutationFn: () => api.deleteDevice(id as number),
    onSuccess: () => {
      setConfirmDelete(false)
      onClose()
      invalidate()
    },
  })

  const d = device.data
  const canManage = me.capabilities.includes('manage:devices')

  return (
    <Drawer
      open={id !== null}
      onClose={onClose}
      title={d?.name ?? 'Device'}
      subtitle={d && <span className="font-mono text-xs">{d.ips.join('  ')}</span>}
    >
      {device.error && <ErrorNote error={device.error} />}
      {d && (
        <div className="space-y-6">
          {(rename.error || routes.error || expire.error || remove.error) && (
            <ErrorNote error={rename.error ?? routes.error ?? expire.error ?? remove.error} />
          )}

          <dl className="grid grid-cols-2 gap-3 text-sm">
            <Field label="Owner">{d.owner ?? 'tagged device'}</Field>
            <Field label="Status">
              <Status ok={d.online} label={d.online ? 'online' : 'offline'} />
            </Field>
            <Field label="Last seen">{d.lastSeen ? new Date(d.lastSeen).toLocaleString() : '—'}</Field>
            <Field label="Key expiry">
              {d.expiry ? new Date(d.expiry).toLocaleString() : 'never expires'}
            </Field>
          </dl>

          {(d.mine || canManage) && (
            <Section title="Actions">
              <div className="flex flex-wrap gap-2">
                {renaming === null ? (
                  <Button onClick={() => setRenaming(d.name)}>Rename</Button>
                ) : (
                  <div className="flex w-full gap-2">
                    <Input value={renaming} onChange={setRenaming} autoFocus className="flex-1" />
                    <Button variant="primary" onClick={() => rename.mutate(renaming)}>
                      Save
                    </Button>
                    <Button variant="ghost" onClick={() => setRenaming(null)}>
                      Cancel
                    </Button>
                  </div>
                )}

                {canManage && (
                  <Button onClick={() => expire.mutate()}>Expire key</Button>
                )}

                <Button variant="danger" onClick={() => setConfirmDelete(true)}>
                  Remove device
                </Button>
              </div>
            </Section>
          )}

          {(d.advertisedRoutes ?? []).length > 0 && canManage && (
            <Section title="Routes">
              <ul className="space-y-1">
                {(d.advertisedRoutes ?? []).map((r) => {
                  const approved = (d.approvedRoutes ?? []).includes(r)

                  return (
                    <li
                      key={r}
                      className="flex items-center justify-between gap-3 rounded-md border border-border bg-surface-1 px-3 py-2"
                    >
                      <code className="font-mono text-xs">{r}</code>
                      <Status ok={approved} warn={!approved} label={approved ? 'approved' : 'pending'} />
                      <Button
                        onClick={() =>
                          routes.mutate(
                            approved
                              ? (d.approvedRoutes ?? []).filter((x) => x !== r)
                              : [...(d.approvedRoutes ?? []), r],
                          )
                        }
                      >
                        {approved ? 'Revoke' : 'Approve'}
                      </Button>
                    </li>
                  )
                })}
              </ul>
            </Section>
          )}

          <Section title="Effective rules">
            <p className="-mt-1 text-xs text-muted-foreground">
              Computed by Headscale&apos;s own policy engine, not an approximation.
            </p>

            <nav className="flex gap-1 border-b border-border">
              {(
                [
                  ['inbound', 'Who can reach this'],
                  ['outbound', 'What this can reach'],
                  ['peers', 'Peers it can see'],
                ] as const
              ).map(([k, label]) => (
                <button
                  key={k}
                  type="button"
                  onClick={() => setTab(k)}
                  className={
                    tab === k
                      ? 'border-b-2 border-accent-500 px-2.5 py-1.5 text-sm font-medium'
                      : 'border-b-2 border-transparent px-2.5 py-1.5 text-sm text-muted-foreground hover:text-foreground'
                  }
                >
                  {label}
                </button>
              ))}
            </nav>

            {rules.isPending && <p className="text-sm text-muted-foreground">Computing…</p>}
            {rules.error && <ErrorNote error={rules.error} />}

            {rules.data && tab === 'inbound' && <RuleList rules={rules.data.inbound} empty="Nothing can reach this device." />}
            {rules.data && tab === 'outbound' && <RuleList rules={rules.data.outbound} empty="This device cannot reach anything." />}
            {rules.data && tab === 'peers' && (
              <ul className="space-y-1">
                {rules.data.peers.length === 0 && (
                  <Empty title="No peers" hint="This device sees nothing else on the tailnet." />
                )}
                {rules.data.peers.map((p) => (
                  <li
                    key={p.id}
                    className="flex items-center justify-between gap-3 rounded-md border border-border bg-surface-1 px-3 py-2"
                  >
                    <span className="font-medium">{p.givenName}</span>
                    <span className="font-mono text-xs text-muted-foreground">{p.ips.join(' ')}</span>
                    <Status ok={p.online} label={p.online ? 'online' : 'offline'} />
                  </li>
                ))}
              </ul>
            )}
          </Section>
        </div>
      )}

      <Confirm
        open={confirmDelete}
        title={`Remove ${d?.name ?? 'this device'}?`}
        body={
          <>
            <span className="font-mono">{d?.name}</span> will be removed from the tailnet and will
            have to register again. Its addresses may be reassigned.
          </>
        }
        confirmLabel="Remove device"
        onConfirm={() => remove.mutate()}
        onCancel={() => setConfirmDelete(false)}
      />
    </Drawer>
  )
}

function RuleList({ rules, empty }: { rules: EffectiveRule[]; empty: string }) {
  if (rules.length === 0) return <Empty title={empty} />

  return (
    <ul className="space-y-1.5">
      {rules.map((r, i) => (
        <li key={i} className="rounded-md border border-border bg-surface-1 px-3 py-2 text-sm">
          <div className="flex flex-wrap items-center gap-2">
            <span className="flex flex-wrap gap-1">
              {r.sources.map((s) => (
                <span key={s.raw} title={s.raw} className="rounded bg-surface-2 px-1.5 py-0.5 text-xs">
                  {s.label}
                </span>
              ))}
            </span>
            <span className="text-muted-foreground">→</span>
            <span className="flex flex-wrap gap-1">
              {r.dests.map((dst) => (
                <span
                  key={dst.raw + dst.ports}
                  title={dst.raw}
                  className="rounded bg-accent-500/15 px-1.5 py-0.5 text-xs text-accent-700 dark:text-accent-400"
                >
                  {dst.label}
                  <span className="opacity-70">:{dst.ports}</span>
                </span>
              ))}
            </span>
          </div>
        </li>
      ))}
    </ul>
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

export type { Device }
