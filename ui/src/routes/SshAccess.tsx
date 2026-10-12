import { useEffect, useMemo, useState } from 'react'
import { unixUserProblem } from '../lib/formHints'
import { cn } from '../lib/cn'
import { useRouter } from '../lib/routerContext'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { CircleAlert, Info, KeyRound, Link2, Link2Off, Plus, RotateCw, Server, Trash2 } from 'lucide-react'
import { api, type SshAccess as SshAccessData, type SshAgent, type SshServer } from '../lib/api'
import { Badge, Button, Confirm, Empty, ErrorNote, Input, Menu, Mono, Section, Status, IconButton } from '../components/ui'
import { TagSelector } from '../components/TagSelector'
import { useToast } from '../components/toastContext'
import { CommandCard } from './Help'
import { headboardUrl, sshAgentInstallCommand, sshNewMachineCommand, tokenFor, userTokens, visibleRows, insecureInstallUrl, revokeRefusal, saveCleanup, loadCleanup, sshAgentOneLiner, withRow, idsFromTokens, pickableTokens } from '../lib/sshAccess'

type User = { id: number; name: string; keys?: number }

/** The /ssh route. The server enforces manage:ssh regardless; this keeps other
 *  roles from a page of polling 403s and a form they cannot use. */
export function SshRoute({ capabilities }: { capabilities: string[] }) {
  if (!capabilities.includes('manage:ssh')) {
    return <p className="text-sm text-muted-foreground">You do not have access to SSH access.</p>
  }

  return <SshAccess />
}

export function SshAccess() {
  const qc = useQueryClient()
  const toast = useToast()
  const access = useQuery({ queryKey: ['ssh-access'], queryFn: api.sshAccess, refetchInterval: 15000 })
  const target = useRouter().search.get('server')
  const [highlight, setHighlight] = useState<string | null>(null)

  // Arrived from "is suspended" in the needs list: bring that server into view
  // and mark it briefly once the list has loaded.
  const loaded = access.isSuccess
  useEffect(() => {
    if (!target || !loaded) return

    document.getElementById(`server-${target}`)?.scrollIntoView({ block: 'start' })
    setHighlight(target)
    const t = setTimeout(() => setHighlight(null), 2000)

    return () => clearTimeout(t)
  }, [target, loaded])
  const users = useQuery({ queryKey: ['tailnet-users'], queryFn: api.tailnetUsers })

  const refresh = () => {
    void qc.invalidateQueries({ queryKey: ['ssh-access'] })
    void qc.invalidateQueries({ queryKey: ['ssh-agents'] })
  }
  const onError = toast.error

  const setRow = useMutation({
    mutationFn: (v: { host: string; unixUser: string; ids: number[]; before?: SshAccessData }) =>
      api.setSshRow(v.host, v.unixUser, v.ids),
    // One at a time: two PUTs in flight can land out of order, and the server
    // keeps whichever came last.
    scope: { id: 'ssh-row' },
    onError: (err, v) => {
      if (v.before) qc.setQueryData(['ssh-access'], v.before)
      onError(err)
    },
    onSettled: refresh,
  })
  // The selector builds each save from the cached rows, so the cache takes the
  // change synchronously, before the request: a second pick must include the
  // first. An in-flight refetch is dropped rather than allowed to overwrite it.
  const saveRow = (host: string, unixUser: string, ids: number[]) => {
    const before = qc.getQueryData<SshAccessData>(['ssh-access'])
    if (before) qc.setQueryData(['ssh-access'], withRow(before, host, unixUser, ids, people))
    void qc.cancelQueries({ queryKey: ['ssh-access'] }, { revert: false })
    setRow.mutate({ host, unixUser, ids, before })
  }
  const approve = useMutation({
    mutationFn: (v: { host: string; agentId: number; ref: string }) => api.approveSshTarget(v.host, v.agentId, v.ref),
    onSuccess: () => { refresh(); toast.ok('Binding approved') },
    onError,
  })
  const [unbinding, setUnbinding] = useState<string | null>(null)
  const [removingOrphan, setRemovingOrphan] = useState<string | null>(null)
  const unbind = useMutation({
    mutationFn: (host: string) => api.unbindSshTarget(host),
    onSuccess: () => { refresh(); toast.ok('Unbound; its keys are cleared on the next sync') },
    onError,
    onSettled: () => setUnbinding(null),
  })
  const removeOrphan = useMutation({
    mutationFn: (host: string) => api.removeSshOrphan(host),
    onSuccess: refresh,
    onError,
    onSettled: () => setRemovingOrphan(null),
  })

  // The access response carries key counts; the users list is the fallback
  // while it loads, and for a server that predates the people field.
  const people: User[] = access.data?.people ?? users.data?.users.map((u) => ({ id: u.id, name: u.name })) ?? []

  return (
    <div className="space-y-6">
      <header>
        <h1 className="text-display font-semibold lg:text-page">SSH access</h1>
        <p className="max-w-3xl text-sm text-muted-foreground">
          Choose who may log in as which unix user on each server. Servers come from the Hosts tab in Access control;
          an agent on the Proxmox host or server writes the keys. Changes reach servers within about 30 seconds.
        </p>
      </header>

      {access.error ? <ErrorNote error={access.error} /> : null}

      {target && access.data && !access.data.servers.some((s) => s.name === target) && (
        <div role="status" className="rounded-lg border border-border bg-surface-1 shadow-card px-4 py-3 text-sm">
          That server is no longer in the policy.
        </div>
      )}

      {access.data?.servers.length === 0 && (
        <Empty icon={KeyRound} title="No servers yet" hint="Add a single-IP entry in Access control → Hosts first." />
      )}

      <div className="grid gap-4">
        {access.data?.servers.map((s) => (
          <ServerCard
            key={s.name}
            server={s}
            users={people}
            onSetRow={(unixUser, ids) => saveRow(s.name, unixUser, ids)}
            onApprove={(agentId, ref) => approve.mutate({ host: s.name, agentId, ref })}
            onUnbind={() => setUnbinding(s.name)}
            highlighted={s.name === highlight}
          />
        ))}
      </div>

      {access.data && access.data.orphans.length > 0 && (
        <Section title="No longer in the policy">
          <ul className="space-y-2 text-sm">
            {access.data.orphans.map((o) => (
              <li key={o.hostName} className="flex flex-wrap items-center gap-2">
                <Mono value={o.hostName} />
                <span className="text-muted-foreground">{o.unixUsers.join(', ') || 'binding only'}</span>
                <IconButton icon={Trash2} variant="danger" label={`Remove ${o.hostName}`} onClick={() => setRemovingOrphan(o.hostName)} />
              </li>
            ))}
          </ul>
        </Section>
      )}

      <AgentsSection />

      <Confirm
        open={unbinding !== null}
        title={`Unbind ${unbinding}?`}
        body="Every key Headboard wrote on this server is removed on the agent's next sync. Binding it again needs a new approval."
        confirmLabel="Unbind"
        onConfirm={() => unbinding && unbind.mutate(unbinding)}
        onCancel={() => setUnbinding(null)}
        busy={unbind.isPending}
      />
      <Confirm
        open={removingOrphan !== null}
        title={`Remove ${removingOrphan}?`}
        body="Deletes its unix-user rows and binding from Headboard. Keys already written stay until the agent clears them."
        confirmLabel="Remove"
        onConfirm={() => removingOrphan && removeOrphan.mutate(removingOrphan)}
        onCancel={() => setRemovingOrphan(null)}
        busy={removeOrphan.isPending}
      />
    </div>
  )
}

export function ServerCard({
  server,
  users,
  onSetRow,
  onApprove,
  onUnbind,
  highlighted = false,
}: {
  server: SshServer
  users: User[]
  onSetRow: (unixUser: string, ids: number[]) => void
  onApprove: (agentId: number, ref: string) => void
  onUnbind: () => void
  /** Arrived here from the needs list for this server. */
  highlighted?: boolean
}) {
  const tokens = useMemo(() => userTokens(users), [users])
  const [newUnixUser, setNewUnixUser] = useState('')
  const [pending, setPending] = useState<string[]>([])
  const [removing, setRemoving] = useState<string | null>(null)
  const rows = visibleRows(server.rows, pending)
  const removingRow = rows.find((r) => r.unixUser === removing)
  const unixProblem = unixUserProblem(newUnixUser, rows.map((r) => r.unixUser))
  const b = server.binding
  const ambiguous = server.suggestions.some((s) => s.ambiguous)

  return (
    <article
      id={`server-${server.name}`}
      className={cn('scroll-mt-24 rounded-xl border border-border bg-surface-1 p-4 shadow-card transition-shadow sm:p-6', highlighted && 'ring-2 ring-attention')}
    >
      <div className="flex items-start gap-3 sm:gap-4">
        <span
          aria-hidden
          className={cn(
            'grid size-10 shrink-0 place-items-center rounded-full sm:size-12',
            !b ? 'bg-surface-2 text-muted-foreground' : b.state === 'active' ? 'bg-accent-500/12 text-accent-700 dark:text-accent-400' : 'bg-danger/10 text-danger',
          )}
        >
          <Server className="size-5" strokeWidth={1.6} />
        </span>
        <div className="flex min-w-0 flex-1 flex-wrap items-start justify-between gap-x-3 gap-y-1">
          <div className="min-w-0">
            <h2 className="break-all text-title">{server.name}</h2>
            <Mono value={server.ip} />
          </div>
          <div className="flex flex-wrap items-center gap-2 text-sm sm:min-h-9">
            {b ? (
              <>
                <Status ok={b.state === 'active'} bad={b.state !== 'active'} label={b.state === 'active' ? 'active' : 'suspended'} />
                <span className="text-muted-foreground">{b.agentName} / CT {b.targetRef}</span>
                {b.state === 'suspended' && b.reapprovable && (
                  <Button icon={Link2} onClick={() => onApprove(b.agentId, b.targetRef)}>Re-approve</Button>
                )}
              </>
            ) : ambiguous ? (
              <span className="text-danger">
                Two targets report this IP:{' '}
                {server.suggestions.map((s) => `${s.agentName} / CT ${s.targetRef} (${s.hostname})`).join(', ')}. Fix the
                duplicate before approving.
              </span>
            ) : server.suggestions.length === 1 ? (
              <>
                <span className="text-muted-foreground">
                  {server.suggestions[0].agentName} / CT {server.suggestions[0].targetRef} ({server.suggestions[0].hostname})
                </span>
                <Button variant="primary" icon={Link2} onClick={() => onApprove(server.suggestions[0].agentId, server.suggestions[0].targetRef)}>
                  Approve
                </Button>
              </>
            ) : (
              <span className="text-muted-foreground">No agent has reported this IP yet.</span>
            )}
          </div>
        </div>
        {b && <Menu label={`More actions for ${server.name}`} items={[{ label: 'Unbind', icon: Link2Off, danger: true, onSelect: onUnbind }]} />}
      </div>

      {b?.state === 'suspended' && (
        <p className="mt-4 flex gap-2.5 rounded-lg bg-surface-2 px-3 py-2.5 text-sm text-muted-foreground">
          <Info aria-hidden className="mt-0.5 size-4 shrink-0" strokeWidth={1.6} />
          <span>
            The approved IP was {b.approvedIp}. Nothing is written or cleared until someone with SSH access re-approves.
            {!b.reapprovable && ` Waiting for ${b.agentName} to report CT ${b.targetRef} at ${server.ip}; or unbind it.`}
          </span>
        </p>
      )}

      <div className="mt-4 space-y-3">
        {rows.length === 0 ? (
          <p className="rounded-lg border border-dashed border-border px-4 py-4 text-center text-sm text-muted-foreground">
            No unix users yet. Add one below, then choose who may log in as it.
          </p>
        ) : (
          <div className="divide-y divide-border rounded-lg border border-border">
            {rows.map((r) => (
              <div key={r.unixUser} className="grid gap-2 px-3 py-3 sm:grid-cols-[10rem_1fr] sm:items-start">
                <div className="flex items-center gap-2 sm:pt-0.5">
                  <Mono value={r.unixUser} />
                  {r.unixUser === 'root' && <Badge tone="danger">root login</Badge>}
                  {/* An unsaved row only lives in this card; a saved one takes away access, so it asks. */}
                  <IconButton
                    icon={Trash2}
                    variant="danger"
                    label={`Remove ${r.unixUser} from ${server.name}`}
                    className="ml-auto"
                    onClick={() => (r.pending ? setPending(pending.filter((u) => u !== r.unixUser)) : setRemoving(r.unixUser))}
                  />
                </div>
                <div>
                  <TagSelector
                    tags={pickableTokens(tokens, users)}
                    value={r.users.map((u) => tokenFor(tokens, u.id))}
                    onChange={(next) => onSetRow(r.unixUser, idsFromTokens(tokens, next))}
                    placeholder="Add a person…"
                    searchLabel={`Find people for ${r.unixUser}`}
                    emptyHint="Nobody has an SSH key yet. Add one from People, on the person's sheet."
                  />
                  <div className="mt-1 flex flex-wrap gap-2 text-xs text-muted-foreground">
                    {r.pending && <span>Not saved yet. Add a person to keep this row.</span>}
                    {r.users.filter((u) => u.keys === 0 && !u.missing).map((u) => (
                      <span key={u.id}>{u.name} has no keys</span>
                    ))}
                    {r.users.filter((u) => u.missing).map((u) => (
                      <span key={u.id} className="text-danger">user #{u.id} no longer exists</span>
                    ))}
                    {r.warnings.map((w, i) => (
                      <span key={`${i}-${w}`} className="text-danger">{w}</span>
                    ))}
                  </div>
                </div>
              </div>
            ))}
          </div>
        )}

        <div className="flex gap-2">
          <Input value={newUnixUser} onChange={setNewUnixUser} placeholder="unix user, e.g. deploy" className="min-w-0 flex-1" />
          <Button
            icon={Plus}
            disabled={newUnixUser === '' || unixProblem !== null}
            onClick={() => {
              setPending([...pending, newUnixUser])
              setNewUnixUser('')
            }}
          >
            Unix user
          </Button>
        </div>
        {unixProblem && <p className="text-xs text-danger">{unixProblem}</p>}
      </div>

      {server.pendingClears.length > 0 &&
        (server.binding?.state === 'suspended' ? (
          <p className="mt-3 flex items-start gap-2 text-sm text-warn">
            <CircleAlert aria-hidden className="mt-0.5 size-4 shrink-0" strokeWidth={1.6} />
            Not removed yet, server suspended: {server.pendingClears.join(', ')}. Re-approve or unbind to remove.
          </p>
        ) : (
          <p className="mt-3 text-xs text-muted-foreground">Clearing: {server.pendingClears.join(', ')}</p>
        ))}
      <Confirm
        open={removingRow !== undefined}
        title={`Remove ${removing} from ${server.name}?`}
        body={
          removingRow && removingRow.users.length > 0
            ? `${removingRow.users.map((u) => u.name).join(', ')} will no longer be able to log in as ${removing}. Their keys are cleared on the agent's next sync.`
            : `Nobody will be able to log in as ${removing}.`
        }
        confirmLabel="Remove"
        onConfirm={() => {
          // Also forget it locally: a row added in this card stays in `pending`
          // and would come back as an unsaved row once the server drops it.
          if (removing) {
            onSetRow(removing, [])
            setPending(pending.filter((u) => u !== removing))
          }
          setRemoving(null)
        }}
        onCancel={() => setRemoving(null)}
      />
    </article>
  )
}

export function AgentsSection() {
  const qc = useQueryClient()
  const toast = useToast()
  const agents = useQuery({ queryKey: ['ssh-agents'], queryFn: api.sshAgents, refetchInterval: 15000 })
  const reg = useQuery({ queryKey: ['registration-info'], queryFn: api.registrationInfo })
  const [name, setName] = useState('')
  const [token, setToken] = useState<{ agent: string; token: string } | null>(null)
  const [revoking, setRevoking] = useState<SshAgent | null>(null)
  const [rotating, setRotating] = useState<SshAgent | null>(null)
  const [refused, setRefused] = useState<{ agent: SshAgent; message: string } | null>(null)
  const [cleanup, setCleanup] = useState<string[]>(loadCleanup)

  const keepCleanup = (cmds: string[]) => {
    saveCleanup(cmds)
    setCleanup(cmds)
  }

  const refresh = () => void qc.invalidateQueries({ queryKey: ['ssh-agents'] })

  const create = useMutation({
    mutationFn: () => api.createSshAgent(name),
    onSuccess: (r) => { setToken({ agent: r.agent.name, token: r.token }); setName(''); refresh() },
    onError: toast.error,
  })
  const rotate = useMutation({
    mutationFn: (a: SshAgent) => api.rotateSshAgent(a.id).then((r) => ({ agent: a.name, token: r.token })),
    onSuccess: (t) => { setToken(t); setRotating(null) },
    onError: (err) => { setRotating(null); toast.error(err) },
  })
  const revoke = useMutation({
    mutationFn: (v: { agent: SshAgent; force: boolean }) => api.revokeSshAgent(v.agent.id, v.force),
    onSuccess: (r, v) => {
      toast.ok(`Revoked ${v.agent.name}`)
      if (r.cleanup.length > 0) keepCleanup(r.cleanup)
      setRevoking(null)
      setRefused(null)
      refresh()
      // Its bindings are gone with it; the server cards above must not keep
      // offering Unbind or Approve against an agent that no longer exists.
      void qc.invalidateQueries({ queryKey: ['ssh-access'] })
    },
    onError: (err, v) => {
      setRevoking(null)
      const message = revokeRefusal(err)
      if (message) setRefused({ agent: v.agent, message })
      else toast.error(err)
    },
  })

  const url = headboardUrl()

  return (
    <Section title="Agents">
      <div className="space-y-4">
        <p className="text-sm text-muted-foreground">
          One agent per Proxmox host (it writes into every container) or per plain server. Adding a normal tailnet
          device still needs only <code className="font-mono">tailscale up</code>.
        </p>

        <div className="flex gap-2">
          <Input value={name} onChange={setName} placeholder="agent name, e.g. pve1" />
          <Button variant="primary" icon={Plus} disabled={name.trim() === '' || create.isPending} onClick={() => create.mutate()}>
            Add agent
          </Button>
        </div>

        {token && (
          <div className="space-y-3 rounded-lg border border-accent-500/40 bg-accent-500/5 p-3">
            <p className="text-sm">
              Token for <strong>{token.agent}</strong> — shown once. The installer asks for it; it is not part of the command.
            </p>
            {/* Labelled, or the copy tooltip would show the token itself. */}
            <Mono value={token.token} label="agent token" />
            {insecureInstallUrl(url) && (
              <p className="text-xs text-danger">
                Headboard is reached over {new URL(url).protocol.replace(':', '')}, not https. The installer and agent
                refuse that; publish Headboard behind https before running these commands.
              </p>
            )}
            <CommandCard
              title="Install on the Proxmox host or server"
              description="One command, token included. Run it where the agent should live."
              command={sshAgentOneLiner(url, token.token)}
              note="Run as root in bash. It turns off history for that shell so the token is not saved to .bash_history; rotate the token if it was saved anyway."
            />
            {reg.data?.headscalePublicUrl && (
              <CommandCard
                title="New machine"
                description="Joins the tailnet (an admin approves the registration), then installs the agent."
                command={sshNewMachineCommand(reg.data.headscalePublicUrl, url)}
                note="Run as root on the Proxmox host or server."
              />
            )}
            <CommandCard
              title="Without the token in the command"
              description="Installs the agent and asks for the token on the terminal."
              command={sshAgentInstallCommand(url)}
              note="Run as root. Headboard must be reached over https."
            />
            <Button variant="ghost" onClick={() => setToken(null)}>I saved the token</Button>
          </div>
        )}

        {agents.error ? <ErrorNote error={agents.error} /> : null}

        <ul className="divide-y divide-border rounded-lg border border-border">
          {agents.data?.agents.map((a) => (
            <li key={a.id} className="flex flex-wrap items-center gap-x-3 gap-y-2 px-3 py-2.5 text-sm">
              <span className="font-medium">{a.name}</span>
              <Status ok={a.online} label={a.online ? 'online' : a.mode ? 'offline' : 'never connected'} />
              {a.mode && <Badge tone="neutral">{a.mode}</Badge>}
              {a.version && !a.versionMatch && <Badge tone="danger">agent {a.version} ≠ Headboard</Badge>}
              <span className="text-muted-foreground">
                {a.lastSeenAt ? `seen ${new Date(a.lastSeenAt).toLocaleString()}` : 'never seen'} · {a.discovered.length} targets ·{' '}
                {a.bindings} bound{a.pendingClears > 0 ? ` · ${a.pendingClears} clears pending` : ''}
              </span>
              <span className="flex w-full items-center gap-2 sm:ml-auto sm:w-auto">
                <Button variant="ghost" icon={RotateCw} onClick={() => setRotating(a)} className="flex-1 justify-center sm:flex-none">Rotate token</Button>
                <IconButton icon={Trash2} variant="danger" label={`Revoke ${a.name}`} onClick={() => setRevoking(a)} />
              </span>
              {a.warnings.length > 0 && (
                <ul className="w-full space-y-0.5 text-xs text-danger">
                  {a.warnings.map((w, i) => (
                    <li key={`${i}-${w.ref}-${w.unix_user ?? ''}`}>
                      {w.ref === 'self' ? 'this host' : `CT ${w.ref}`}
                      {w.unix_user ? ` ${w.unix_user}` : ''}: {w.message}
                    </li>
                  ))}
                </ul>
              )}
            </li>
          ))}
        </ul>

        {refused && (
          <div className="space-y-2 rounded-lg border border-danger/40 bg-danger/5 p-3 text-sm">
            <p>
              <strong>{refused.agent.name}</strong> was not revoked: {refused.message}.
            </p>
            <p className="text-xs text-muted-foreground">
              Forcing leaves the keys it wrote on the servers; you will get commands to remove them by hand.
            </p>
            <div className="flex gap-2">
              <Button variant="danger-solid" icon={Trash2} disabled={revoke.isPending} onClick={() => revoke.mutate({ agent: refused.agent, force: true })}>
                Force revoke
              </Button>
              <Button variant="ghost" onClick={() => setRefused(null)}>Cancel</Button>
            </div>
          </div>
        )}

        {cleanup.length > 0 && (
          <div className="space-y-2">
            <p className="text-sm">Run these to remove keys the revoked agent had written:</p>
            {cleanup.map((c) => <Mono key={c} value={c} />)}
            <Button variant="ghost" onClick={() => keepCleanup([])}>I ran these</Button>
          </div>
        )}
      </div>

      <Confirm
        open={revoking !== null}
        title={`Revoke ${revoking?.name}?`}
        body="The agent stops syncing. Headboard refuses while it still has bound servers or keys waiting to be cleared."
        confirmLabel="Revoke"
        onConfirm={() => revoking && revoke.mutate({ agent: revoking, force: false })}
        onCancel={() => setRevoking(null)}
        busy={revoke.isPending}
      />
      <Confirm
        open={rotating !== null}
        title={`Rotate the token for ${rotating?.name}?`}
        body="The current token stops working at once. The agent stays offline until you reinstall it with the new token."
        confirmLabel="Rotate token"
        onConfirm={() => rotating && rotate.mutate(rotating)}
        onCancel={() => setRotating(null)}
        busy={rotate.isPending}
      />
    </Section>
  )
}
