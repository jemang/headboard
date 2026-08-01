import { useEffect, useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, type Device, type EffectiveRule, type Me } from '../lib/api'
import { Badge, Button, Confirm, Drawer, Empty, ErrorNote, Input, Mono, Section, Status } from '../components/ui'
import { Cell, Row, SpanRow, Table } from '../components/Table'
import { Loading, Skeleton, SkeletonRows } from '../components/Skeleton'
import { useToast } from '../components/Toast'
import { devicePulse } from '../lib/devicePulse'
import { approveRoutes, revokeRoutes, routeSummary } from '../lib/deviceRouting'
import { Laptop, Search, Trash2, Pencil, TimerReset, Check, X } from 'lucide-react'

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
  const pulse = devicePulse(devices.data?.devices ?? [])

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

  if (devices.error) return <ErrorNote error={devices.error} />

  return (
    <div className="space-y-6">
      <header className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <p className="text-eyebrow font-semibold uppercase text-muted-foreground">Tailnet inventory</p>
          <h1 className="mt-1 text-display font-semibold">{admin ? 'Devices' : 'My devices'}</h1>
          <p className="text-sm text-muted-foreground">
            {admin
              ? 'Every machine in the tailnet.'
              : 'Your machines, and the rules that actually apply to them.'}
          </p>
        </div>
        <div className="flex items-center gap-2">
          <div className="relative">
            <Search
              aria-hidden
              className="pointer-events-none absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground"
              strokeWidth={1.5}
            />
            <Input
              value={filter}
              onChange={setFilter}
              placeholder="Filter…"
              className="w-44 pl-8 sm:w-56"
            />
          </div>
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

      {devices.data && <NetworkPulse total={pulse.total} online={pulse.online} offline={pulse.offline} expired={pulse.expired} />}

      <Table
        columns={['Device', 'Addresses', 'Owner', 'Status', 'Routes']}
      >
        {devices.isPending ? (
          <SpanRow columns={5}>
            <Loading label="Loading devices">
              <SkeletonRows rows={4} cols={5} />
            </Loading>
          </SpanRow>
        ) : rows.length === 0 ? (
          <SpanRow columns={5}>
            <Empty
              icon={Laptop}
              title="No devices"
              hint={
                filter || onlineOnly
                  ? 'Nothing matches the current filter.'
                  : 'Enrol one from the Keys page to see it here.'
              }
            />
          </SpanRow>
        ) : (
          rows.map((d) => (
            <Row key={d.id} onClick={() => setSelected(d.id)} label={`Open ${d.name}`}>
              <Cell>
                <div className="font-medium">{d.name}</div>
                {(d.tags ?? []).length > 0 && (
                  <div className="mt-1 flex flex-wrap gap-1">
                    {d.tags?.map((t) => (
                      <Badge key={t} tone="accent">
                        {t}
                      </Badge>
                    ))}
                  </div>
                )}
              </Cell>
              <Cell>
                <div className="flex flex-col gap-0.5">
                  {d.ips.map((ip) => (
                    <Mono key={ip} value={ip} />
                  ))}
                </div>
              </Cell>
              <Cell muted>{d.owner ?? <span className="text-xs">tagged</span>}</Cell>
              <Cell>
                <div className="flex flex-col gap-1">
                  <Status ok={d.online} label={d.online ? 'online' : 'offline'} />
                  {d.expired && <Status ok={false} warn label="key expired" />}
                </div>
              </Cell>
              <Cell>
                <div className="flex flex-wrap gap-1">
                  {d.exitNode && <Badge tone="accent">exit node</Badge>}
                  {(d.subnetRoutes ?? []).map((r) => (
                    <Badge key={r}>{r}</Badge>
                  ))}
                  {(d.advertisedRoutes ?? []).filter(
                    (r) => !(d.approvedRoutes ?? []).includes(r),
                  ).length > 0 && <Badge tone="warn">awaiting approval</Badge>}
                </div>
              </Cell>
            </Row>
          ))
        )}
      </Table>

      <DeviceDrawer id={selected} me={me} onClose={() => setSelected(null)} />
    </div>
  )
}

function NetworkPulse({
  total,
  online,
  offline,
  expired,
}: {
  total: number
  online: number
  offline: number
  expired: number
}) {
  const onlineWidth = total === 0 ? 0 : (online / total) * 100

  return (
    <section aria-label="Tailnet status" className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
      <div className="rounded-xl border border-border bg-surface-1 p-4 shadow-sm sm:col-span-2 xl:col-span-2">
        <div className="flex items-center justify-between gap-3">
          <span className="text-eyebrow font-semibold uppercase text-muted-foreground">Network pulse</span>
          <Status ok={online > 0} label={`${online} online`} />
        </div>
        <div className="mt-3 flex items-end gap-2">
          <strong className="text-3xl font-semibold tracking-tight">{total}</strong>
          <span className="mb-1 text-sm text-muted-foreground">device{total === 1 ? '' : 's'} in this tailnet</span>
        </div>
        <div className="mt-4 h-2 overflow-hidden rounded-full bg-surface-2" aria-hidden>
          <div className="h-full rounded-full bg-accent-500 transition-[width] duration-300" style={{ width: `${onlineWidth}%` }} />
        </div>
      </div>
      <div className="rounded-xl border border-border bg-surface-1 p-4 shadow-sm">
        <span className="text-eyebrow font-semibold uppercase text-muted-foreground">Offline</span>
        <strong className="mt-3 block text-3xl font-semibold tracking-tight">{offline}</strong>
        <span className="text-sm text-muted-foreground">shown in the table</span>
      </div>
      <div className={expired > 0 ? 'rounded-xl border border-warn/45 bg-warn/10 p-4 shadow-sm' : 'rounded-xl border border-border bg-surface-1 p-4 shadow-sm'}>
        <span className={expired > 0 ? 'text-eyebrow font-semibold uppercase text-warn' : 'text-eyebrow font-semibold uppercase text-muted-foreground'}>Attention</span>
        <strong className={expired > 0 ? 'mt-3 block text-3xl font-semibold tracking-tight text-warn' : 'mt-3 block text-3xl font-semibold tracking-tight'}>{expired}</strong>
        <span className="text-sm text-muted-foreground">expired key{expired === 1 ? '' : 's'}</span>
      </div>
    </section>
  )
}

function DeviceDrawer({ id, me, onClose }: { id: number | null; me: Me; onClose: () => void }) {
  const qc = useQueryClient()
  const toast = useToast()
  const [confirmDelete, setConfirmDelete] = useState(false)
  const [exitAction, setExitAction] = useState<'approve' | 'revoke' | null>(null)
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
    onSuccess: (d) => {
      setRenaming(null)
      invalidate()
      toast.ok(`Renamed to ${d.name}`)
    },
    onError: toast.error,
  })

  const routes = useMutation({
    mutationFn: ({ next }: { next: string[]; notice: string }) => api.approveRoutes(id as number, next),
    onSuccess: (_, { notice }) => {
      invalidate()
      toast.ok(notice)
    },
    onError: toast.error,
  })

  const expire = useMutation({
    mutationFn: () => api.expireDevice(id as number),
    onSuccess: () => {
      invalidate()
      toast.ok('Key expired — the device must sign in again')
    },
    onError: toast.error,
  })

  const remove = useMutation({
    mutationFn: () => api.deleteDevice(id as number),
    onSuccess: () => {
      setConfirmDelete(false)
      onClose()
      invalidate()
      toast.ok('Device removed')
    },
    onError: toast.error,
  })

  const d = device.data
  const canManage = me.capabilities.includes('manage:devices')
  const routing = d ? routeSummary(d.advertisedRoutes ?? [], d.approvedRoutes ?? []) : null

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
          <dl className="grid grid-cols-2 gap-3 text-sm">
            <Field label="Owner">{d.owner ?? 'tagged device'}</Field>
            <Field label="Hostname"><code className="font-mono text-xs">{d.hostname}</code></Field>
            <Field label="Node ID"><code className="font-mono text-xs">{d.id}</code></Field>
            <Field label="Status">
              <Status ok={d.online} label={d.online ? 'online' : 'offline'} />
            </Field>
            <Field label="Addresses">
              <span className="flex flex-wrap gap-1">
                {d.ips.map((ip) => <Mono key={ip} value={ip} />)}
              </span>
            </Field>
            <Field label="Tags">
              {(d.tags ?? []).length > 0 ? (
                <span className="flex flex-wrap gap-1">
                  {d.tags?.map((tag) => <Badge key={tag} tone="accent">{tag}</Badge>)}
                </span>
              ) : '—'}
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
                  <Button icon={Pencil} onClick={() => setRenaming(d.name)}>
                    Rename
                  </Button>
                ) : (
                  <div className="flex w-full gap-2">
                    <Input value={renaming} onChange={setRenaming} autoFocus className="flex-1" />
                    <Button variant="primary" icon={Check} onClick={() => rename.mutate(renaming)}>
                      Save
                    </Button>
                    <Button variant="ghost" icon={X} onClick={() => setRenaming(null)}>
                      Cancel
                    </Button>
                  </div>
                )}

                {canManage && (
                  <Button icon={TimerReset} onClick={() => expire.mutate()}>
                    Expire key
                  </Button>
                )}

                <Button variant="danger" icon={Trash2} onClick={() => setConfirmDelete(true)}>
                  Remove device
                </Button>
              </div>
            </Section>
          )}

          {routing && canManage && (routing.exit.state !== 'none' || routing.subnets.length > 0) && (
            <Section title="Networking & routing">
              <div className="space-y-4">
                {routing.exit.state !== 'none' && (
                  <div className={routing.exit.state === 'incomplete' ? 'rounded-lg border border-warn/40 bg-warn/10 p-3' : 'rounded-lg border border-border bg-surface-1 p-3'}>
                    <div className="flex flex-wrap items-center justify-between gap-3">
                      <div>
                        <h3 className="text-sm font-medium">Exit node</h3>
                        {routing.exit.state === 'incomplete' ? (
                          <p className="mt-1 text-xs text-warn">This device must advertise both IPv4 and IPv6 default routes before it can be approved as an exit node.</p>
                        ) : (
                          <p className="mt-1 text-xs text-muted-foreground">Lets tailnet users send general internet traffic through this device.</p>
                        )}
                      </div>
                      {routing.exit.state !== 'incomplete' && (
                        <Status
                          ok={routing.exit.state === 'approved'}
                          warn={routing.exit.state !== 'approved'}
                          label={routing.exit.state === 'approved' ? 'approved' : 'pending approval'}
                        />
                      )}
                    </div>
                    <div className="mt-3 flex flex-wrap items-center justify-between gap-2">
                      <span className="font-mono text-xs text-muted-foreground">{routing.exit.routes.join(' · ')}</span>
                      {routing.exit.state !== 'incomplete' && (
                        <Button
                          disabled={routes.isPending}
                          onClick={() => setExitAction(routing.exit.state === 'approved' ? 'revoke' : 'approve')}
                        >
                          {routing.exit.state === 'approved' ? 'Revoke exit node' : 'Approve exit node'}
                        </Button>
                      )}
                    </div>
                  </div>
                )}

                {routing.subnets.length > 0 && (
                  <div>
                    <h3 className="text-sm font-medium">Subnet routes</h3>
                    <p className="mt-1 text-xs text-muted-foreground">Allow tailnet devices to reach these private networks through this device.</p>
                    <ul className="mt-2 space-y-1">
                      {routing.subnets.map(({ route, approved }) => (
                        <li key={route} className="flex items-center justify-between gap-3 rounded-md border border-border bg-surface-1 px-3 py-2">
                          <code className="font-mono text-xs">{route}</code>
                          <Status ok={approved} warn={!approved} label={approved ? 'approved' : 'pending'} />
                          <Button
                            disabled={routes.isPending}
                            onClick={() => routes.mutate({
                              next: approved
                                ? revokeRoutes(d.approvedRoutes ?? [], [route])
                                : approveRoutes(d.approvedRoutes ?? [], [route]),
                              notice: approved ? `Revoked ${route}` : `Approved ${route}`,
                            })}
                          >
                            {approved ? 'Revoke' : 'Approve'}
                          </Button>
                        </li>
                      ))}
                    </ul>
                  </div>
                )}
              </div>
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

            {rules.isPending && (
              <Loading label="Computing effective rules">
                <div className="space-y-2">
                  <Skeleton className="h-9 w-full" />
                  <Skeleton className="h-9 w-4/5" />
                </div>
              </Loading>
            )}
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
      <Confirm
        open={exitAction !== null}
        title={`${exitAction === 'approve' ? 'Approve' : 'Revoke'} exit node for ${d?.name ?? 'this device'}?`}
        body={
          exitAction === 'approve'
            ? <>Tailnet users will be able to send general internet traffic through <span className="font-mono">{d?.name}</span>.</>
            : <>Tailnet users will no longer be able to use <span className="font-mono">{d?.name}</span> as an exit node.</>
        }
        confirmLabel={exitAction === 'approve' ? 'Approve exit node' : 'Revoke exit node'}
        onConfirm={() => {
          if (!d || !routing || !exitAction) return

          routes.mutate({
            next: exitAction === 'approve'
              ? approveRoutes(d.approvedRoutes ?? [], routing.exit.routes)
              : revokeRoutes(d.approvedRoutes ?? [], routing.exit.routes),
            notice: exitAction === 'approve' ? 'Exit node approved' : 'Exit node revoked',
          })
          setExitAction(null)
        }}
        onCancel={() => setExitAction(null)}
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
