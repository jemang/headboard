import { useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, type Grant, type PatchOp, type Policy, type PolicyPreview, type SshRule } from '../lib/api'
import { Button, Badge, Empty, ErrorNote, Input, Section } from '../components/ui'
import { TokenPicker, validateTokens } from '../components/TokenPicker'
import { TestsTab } from './AclTests'
import { useToast } from '../components/Toast'
import { Loading, Skeleton } from '../components/Skeleton'
import { grantsWithPendingChanges, schemaWithPendingChanges } from '../lib/policyDraft'
import { grantStageOp, jumpTarget, policyStateNotice, type PolicyTab } from '../lib/grantEditor'
import { Link } from '../lib/router'
import { Eye, Plus, Save, Trash2, Undo2 } from 'lucide-react'

const tabs: { id: PolicyTab; label: string }[] = [
  { id: 'grants', label: 'Grants' },
  { id: 'groups', label: 'Groups' },
  { id: 'tags', label: 'Tags' },
  { id: 'hosts', label: 'Hosts' },
  { id: 'ssh', label: 'SSH' },
  { id: 'auto', label: 'Routing approvals' },
  { id: 'tests', label: 'Tests' },
  { id: 'raw', label: 'Raw HuJSON' },
]

export function Acl() {
  const qc = useQueryClient()
  const toast = useToast()
  const [tab, setTab] = useState<PolicyTab>('grants')
  const [pending, setPending] = useState<PatchOp[]>([])
  const [rawDraft, setRawDraft] = useState<string | null>(null)
  const [highlight, setHighlight] = useState<string | null>(null)
  const [note, setNote] = useState('')

  const policy = useQuery({ queryKey: ['policy'], queryFn: api.policy })

  const preview = useMutation({
    mutationFn: (body: { sha256: string; ops?: PatchOp[]; hujson?: string }) =>
      api.previewPolicy(body),
  })

  const save = useMutation({
    mutationFn: (body: { sha256: string; ops?: PatchOp[]; hujson?: string; note?: string }) =>
      api.savePolicy(body),
    onSuccess: (next) => {
      qc.setQueryData(['policy'], next)
      setPending([])
      setRawDraft(null)
      setNote('')
      preview.reset()
      toast.ok('Policy saved')
    },
    onError: toast.error,
  })

  if (policy.isPending) {
    return (
      <Loading label="Loading policy">
        <div className="space-y-4">
          <Skeleton className="h-8 w-56" />
          <Skeleton className="h-9 w-full" />
          <Skeleton className="h-24 w-full" />
          <Skeleton className="h-24 w-full" />
        </div>
      </Loading>
    )
  }
  if (policy.error) return <ErrorNote error={policy.error} />

  const p = policy.data

  const change = rawDraft !== null ? { hujson: rawDraft } : pending.length > 0 ? { ops: pending } : null

  const queue = (op: PatchOp) => {
    setPending((prev) => [...prev, op])
    preview.reset()
  }

  // The simulator answers with a JSON pointer at the entry responsible. Opening
  // the tab and marking the row is what turns that answer into an edit.
  const jump = (pointer: string) => {
    setHighlight(pointer)

    setTab(jumpTarget(pointer))
  }

  return (
    <div className="space-y-6">
      <header className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <p className="text-eyebrow font-semibold uppercase text-muted-foreground">Policy workbench</p>
          <h1 className="mt-1 text-display font-semibold">Access control</h1>
          <p className="text-sm text-muted-foreground">
            Edited as a form, stored as your HuJSON. Comments and layout outside the fields you
            change are left alone.
          </p>
        </div>
        <div className="rounded-lg border border-border bg-surface-1 px-3 py-2 text-xs text-muted-foreground">
          <span className="text-eyebrow font-semibold uppercase">Revision </span>
          <code className="ml-1 font-mono">{p.sha256.slice(0, 12)}</code>
        </div>
      </header>

      {p.parseError && (
        <div className="rounded-md border border-warn/40 bg-warn/10 px-3 py-2 text-sm">
          The stored policy does not parse, so the form is unavailable. Fix it in the raw editor.
          <div className="mt-1 font-mono text-xs">{p.parseError}</div>
        </div>
      )}

      <PolicyStateNotice policy={p} onRaw={() => setTab('raw')} />

      <nav className="flex gap-1 overflow-x-auto border-b border-border pb-px" aria-label="Policy sections">
        {tabs.map((t) => (
          <button
            key={t.id}
            type="button"
            onClick={() => setTab(t.id)}
            className={
              tab === t.id
                ? 'border-b-2 border-accent-500 px-3 py-2 text-sm font-semibold text-foreground transition-colors'
                : 'border-b-2 border-transparent px-3 py-2 text-sm text-muted-foreground transition-colors hover:border-border hover:text-foreground'
            }
          >
            {t.label}
          </button>
        ))}
      </nav>

      {tab === 'grants' && <GrantsTab policy={p} pending={pending} queue={queue} highlight={highlight} />}
      {tab === 'groups' && <MapListTab policy={p} pending={pending} section="groups" queue={queue} slot="src" />}
      {tab === 'tags' && <MapListTab policy={p} pending={pending} section="tagOwners" queue={queue} slot="src" />}
      {tab === 'hosts' && <HostsTab policy={p} pending={pending} queue={queue} />}
      {tab === 'ssh' && <SshTab policy={p} pending={pending} queue={queue} />}
      {tab === 'auto' && <AutoApproversTab policy={p} pending={pending} queue={queue} />}
      {tab === 'tests' && (
        <TestsTab policy={p} draft={change} queue={queue} onJump={jump} />
      )}
      {tab === 'raw' && (
        <RawTab
          text={rawDraft ?? p.hujson}
          onChange={(v) => {
            setRawDraft(v)
            setPending([])
            preview.reset()
          }}
          dirty={rawDraft !== null}
          onReset={() => setRawDraft(null)}
          location={tab === 'raw' ? highlight : null}
        />
      )}

      {change && (
        <div className="sticky bottom-4 z-10 space-y-3 rounded-xl border border-accent-500/35 bg-surface-1/95 p-3 shadow-raised backdrop-blur sm:p-4">
          <div className="flex flex-wrap items-center gap-2">
            <Badge tone="accent">
              {rawDraft !== null ? 'raw edit' : `${pending.length} pending change${pending.length === 1 ? '' : 's'}`}
            </Badge>
            <Input value={note} onChange={setNote} placeholder="Why? (saved with the revision)" className="flex-1" />
            <Button
              icon={Eye}
              onClick={() => preview.mutate({ sha256: p.sha256, ...change })}
              disabled={preview.isPending}
            >
              {preview.isPending ? 'Checking…' : 'Preview diff'}
            </Button>
            <Button
              variant="ghost"
              icon={Undo2}
              onClick={() => {
                setPending([])
                setRawDraft(null)
                preview.reset()
              }}
            >
              Discard
            </Button>
          </div>

          {preview.error && <ErrorNote error={preview.error} />}

          {preview.data && (
            <PreviewPanel
              preview={preview.data}
              saving={save.isPending}
              onSave={() => save.mutate({ sha256: p.sha256, note, ...change })}
            />
          )}
        </div>
      )}
    </div>
  )
}

/**
 * Nothing is saved without showing the real diff first. An ACL mistake locks
 * people out of machines, so "are you sure" is not enough — the confirm has to
 * show what is actually about to change.
 */
function PreviewPanel({
  preview,
  onSave,
  saving,
}: {
  preview: PolicyPreview
  onSave: () => void
  saving: boolean
}) {
  if (preview.diff.identical) {
    return <p className="text-sm text-muted-foreground">That change would not alter the policy.</p>
  }

  return (
    <div className="space-y-2">
      <div className="flex items-center gap-3 text-sm">
        <span className="text-ok">+{preview.diff.added}</span>
        <span className="text-danger">−{preview.diff.removed}</span>
        {preview.valid ? (
          <span className="text-ok">Headscale accepts this policy</span>
        ) : (
          <span className="text-danger">Headscale rejects this policy</span>
        )}
      </div>

      {!preview.valid && preview.error && (
        <pre className="overflow-x-auto rounded-md border border-danger/40 bg-danger/10 p-3 font-mono text-xs text-danger">
          {preview.error}
        </pre>
      )}

      <div className="max-h-72 overflow-auto rounded-md border border-border bg-surface-1">
        <pre className="min-w-max p-3 font-mono text-xs leading-relaxed">
          {preview.diff.hunks.map((h, i) => (
            <div key={i}>
              <div className="text-muted-foreground">
                @@ -{h.oldStart},{h.oldLines} +{h.newStart},{h.newLines} @@
              </div>
              {h.lines.map((l, j) => (
                <div
                  key={j}
                  className={
                    l.startsWith('+')
                      ? 'bg-ok/10 text-ok'
                      : l.startsWith('-')
                        ? 'bg-danger/10 text-danger'
                        : 'text-muted-foreground'
                  }
                >
                  {l}
                </div>
              ))}
            </div>
          ))}
        </pre>
      </div>

      <Button variant="primary" icon={Save} onClick={onSave} disabled={!preview.valid || saving}>
        {saving ? 'Saving…' : 'Save policy'}
      </Button>
    </div>
  )
}

function PolicyStateNotice({ policy, onRaw }: { policy: Policy; onRaw: () => void }) {
  const notice = policyStateNotice(policy.sections ?? [], policy.schema ?? {})

  return (
    <div className={notice.tone === 'warn' ? 'rounded-lg border border-warn/40 bg-warn/10 p-3 text-sm' : 'rounded-lg border border-accent-500/25 bg-accent-500/5 p-3 text-sm'}>
      <p>{notice.message}</p>
      {notice.rawPointer && <Button variant="ghost" className="mt-1" onClick={onRaw}>Open Raw HuJSON ({notice.rawPointer})</Button>}
    </div>
  )
}

function GrantsTab({
  policy,
  pending,
  queue,
  highlight,
}: {
  policy: Policy
  pending: PatchOp[]
  queue: (op: PatchOp) => void
  highlight: string | null
}) {
  const grants = grantsWithPendingChanges(policy.schema?.grants ?? [], pending)
  const hasGrantsSection = Object.hasOwn(schemaWithPendingChanges(policy.schema, pending), 'grants')

  return (
    <Section
      title={`${grants.length} grant${grants.length === 1 ? '' : 's'}`}
      actions={
        <Button
          icon={Plus}
          onClick={() => queue(grantStageOp('new', { src: [], dst: [], ip: ['*'] }, hasGrantsSection))}
        >
          Add grant
        </Button>
      }
    >
      {grants.length === 0 && (
        <Empty
          title="No grants yet"
          hint="Add a grant to define who can reach a device, subnet, or service."
        />
      )}
      <ol className="space-y-2">
        {grants.map((grant, i) => (
          <GrantRow
            key={i}
            index={i}
            grant={grant}
            policy={policy}
            queue={queue}
            highlighted={highlight === `/grants/${i}`}
          />
        ))}
      </ol>
    </Section>
  )
}

function GrantRow({
  index,
  grant,
  policy,
  queue,
  highlighted,
}: {
  index: number
  grant: Grant
  policy: Policy
  queue: (op: PatchOp) => void
  highlighted?: boolean
}) {
  const [src, setSrc] = useState(grant.src ?? [])
  const [dst, setDst] = useState(grant.dst ?? [])
  const [ip, setIP] = useState(grant.ip ?? [])
  const [via, setVia] = useState(grant.via ?? [])

  const issues = useMemo(
    () => [
      ...validateTokens(src, policy.tokens, 'src'),
      ...validateTokens(dst, policy.tokens, 'grant-dst'),
      ...validateTokens(ip, policy.tokens, 'ip'),
      ...validateTokens(via, policy.tokens, 'via'),
    ],
    [src, dst, ip, via, policy.tokens],
  )

  const next = { src, dst, ip, ...(via.length ? { via } : {}) }
  const dirty = JSON.stringify(next) !== JSON.stringify({ src: grant.src ?? [], dst: grant.dst ?? [], ip: grant.ip ?? [], ...(grant.via?.length ? { via: grant.via } : {}) })
  const blocking = issues.some((issue) => issue.severity === 'blocking')
  const routeGuidance = via.length > 0 || dst.some((value) => /[.:]/.test(value))

  return (
    <li
      className={
        highlighted
          ? 'rounded-lg border border-accent-500 bg-surface-1 p-3 ring-1 ring-accent-500'
          : 'rounded-lg border border-border bg-surface-1 p-3'
      }
    >
      {grant.app !== undefined ? (
        <div className="space-y-1">
          <Badge tone="warn">Raw HuJSON only</Badge>
          <p className="text-sm text-muted-foreground">This grant contains application capabilities. It stays in place and can only be edited in Raw HuJSON.</p>
        </div>
      ) : <div className="flex flex-wrap items-start gap-2">
        <span className="mt-1.5 w-8 shrink-0 font-mono text-xs text-muted-foreground">#{index}</span>

        <div className="flex min-w-64 flex-1 flex-col gap-1">
          <span className="text-xs text-muted-foreground">from</span>
          <TokenPicker
            values={src}
            onChange={setSrc}
            tokens={policy.tokens}
            slot="src"
            placeholder="alice@, group:eng, tag:prod…"
          />
        </div>

        <span className="mt-6 text-muted-foreground">→</span>

        <div className="flex min-w-64 flex-1 flex-col gap-1">
          <span className="text-xs text-muted-foreground">to</span>
          <TokenPicker
            values={dst}
            onChange={setDst}
            tokens={policy.tokens}
            slot="grant-dst"
            placeholder="tag:prod, 13.0.0.0/24…"
          />
        </div>

        <div className="flex min-w-40 flex-1 flex-col gap-1">
          <span className="text-xs text-muted-foreground">ports</span>
          <TokenPicker values={ip} onChange={setIP} tokens={policy.tokens} slot="ip" placeholder="*, tcp:443…" />
        </div>

        <div className="flex min-w-40 flex-1 flex-col gap-1">
          <span className="text-xs text-muted-foreground">via (optional)</span>
          <TokenPicker values={via} onChange={setVia} tokens={policy.tokens} slot="via" placeholder="tag:router…" />
        </div>

        <div className="mt-5 flex gap-1">
          <Button
            disabled={!dirty || src.length === 0 || dst.length === 0 || ip.length === 0 || blocking}
            onClick={() => queue(grantStageOp(index, next, true))}
          >
            Stage
          </Button>
          <Button
            variant="ghost"
            icon={Trash2}
            onClick={() => queue({ op: 'remove', path: `/grants/${index}` })}
          >
            Delete
          </Button>
        </div>
      </div>}

      {issues.length > 0 && (
        <ul className="mt-2 space-y-0.5 pl-10 text-xs text-danger">
          {issues.map((issue) => (
            <li key={issue.token} className={issue.severity === 'warning' ? 'text-warn' : ''}>
              <span className="font-mono">{issue.token}</span> — {issue.reason}
            </li>
          ))}
        </ul>
      )}
      {routeGuidance && grant.app === undefined && <p className="mt-2 text-xs text-muted-foreground">Policy permission is separate from routing: advertise and approve the route, enable forwarding, and have clients accept routes. See <Link to="/devices" className="text-accent-700 underline underline-offset-2 dark:text-accent-400">Devices</Link> and Routing approvals.</p>}
    </li>
  )
}

/** groups and tagOwners share a shape: a name mapped to a list of owners. */
function MapListTab({
  policy,
  pending,
  section,
  queue,
  slot,
}: {
  policy: Policy
  pending: PatchOp[]
  section: 'groups' | 'tagOwners'
  queue: (op: PatchOp) => void
  slot: 'src'
}) {
  const draft = schemaWithPendingChanges(policy.schema, pending)
  const entries = Object.entries((section === 'groups' ? draft.groups : draft.tagOwners) ?? {})
  const [name, setName] = useState('')

  const prefix = section === 'groups' ? 'group:' : 'tag:'

  return (
    <Section
      title={section}
      actions={section === 'tagOwners' ? (
        <span className="text-xs text-muted-foreground">
          who may claim a tag — devices get tagged from Devices, not here
        </span>
      ) : undefined}
    >
      {entries.length === 0 && <Empty title={`No ${section} defined`} />}

      <ul className="space-y-2">
        {entries.map(([key, members]) => (
          <MapListRow
            key={key}
            name={key}
            members={members}
            policy={policy}
            slot={slot}
            onStage={(next) => queue({ op: 'replace', path: `/${section}/${escape(key)}`, value: next })}
            onDelete={() => queue({ op: 'remove', path: `/${section}/${escape(key)}` })}
          />
        ))}
      </ul>

      <div className="flex items-center gap-2">
        <Input value={name} onChange={setName} placeholder={`${prefix}name`} className="w-64" />
        <Button
          disabled={!name.startsWith(prefix) || name.length <= prefix.length}
          onClick={() => {
            queue({ op: 'add', path: `/${section}/${escape(name)}`, value: [] })
            setName('')
          }}
        >
          Add
        </Button>
      </div>
    </Section>
  )
}

function MapListRow({
  name,
  members,
  policy,
  slot,
  onStage,
  onDelete,
}: {
  name: string
  members: string[]
  policy: Policy
  slot: 'src'
  onStage: (next: string[]) => void
  onDelete: () => void
}) {
  const [values, setValues] = useState(members)
  const dirty = JSON.stringify(values) !== JSON.stringify(members)
  const issues = validateTokens(values, policy.tokens, slot)

  return (
    <li className="flex flex-wrap items-center gap-2 rounded-lg border border-border bg-surface-1 p-2.5">
      <code className="w-48 shrink-0 font-mono text-xs">{name}</code>
      <TokenPicker values={values} onChange={setValues} tokens={policy.tokens} slot={slot} />
      <Button disabled={!dirty || issues.length > 0} onClick={() => onStage(values)}>
        Stage
      </Button>
      <Button variant="ghost" onClick={onDelete}>
        Delete
      </Button>
      {issues.length > 0 && (
        <ul className="w-full space-y-0.5 text-xs text-danger">
          {issues.map((issue) => <li key={issue.token}><span className="font-mono">{issue.token}</span> — {issue.reason}</li>)}
        </ul>
      )}
    </li>
  )
}

function HostsTab({ policy, pending, queue }: { policy: Policy; pending: PatchOp[]; queue: (op: PatchOp) => void }) {
  const entries = Object.entries(schemaWithPendingChanges(policy.schema, pending).hosts ?? {})
  const [name, setName] = useState('')
  const [cidr, setCidr] = useState('')

  return (
    <Section title="hosts">
      {entries.length === 0 && <Empty title="No hosts defined" hint="A host is a name for a CIDR." />}

      <ul className="space-y-2">
        {entries.map(([host, value]) => (
          <HostRow
            key={host}
            host={host}
            value={value}
            queue={queue}
          />
        ))}
      </ul>

      <div className="flex items-center gap-2">
        <Input value={name} onChange={setName} placeholder="office-lan" className="w-48" />
        <Input value={cidr} onChange={setCidr} placeholder="10.0.0.0/24" className="w-48" />
        <Button
          disabled={name === '' || cidr === ''}
          onClick={() => {
            queue({ op: 'add', path: `/hosts/${escape(name)}`, value: cidr })
            setName('')
            setCidr('')
          }}
        >
          Add
        </Button>
      </div>
    </Section>
  )
}

function HostRow({ host, value, queue }: { host: string; value: string; queue: (op: PatchOp) => void }) {
  const [cidr, setCidr] = useState(value)

  return (
    <li className="flex flex-wrap items-center gap-2 rounded-lg border border-border bg-surface-1 p-2.5">
      <code className="w-48 font-mono text-xs">{host}</code>
      <Input value={cidr} onChange={setCidr} className="min-w-48 flex-1 font-mono text-xs" />
      <Button
        disabled={cidr.trim() === '' || cidr === value}
        onClick={() => queue({ op: 'replace', path: `/hosts/${escape(host)}`, value: cidr.trim() })}
      >
        Stage
      </Button>
      <Button variant="ghost" onClick={() => queue({ op: 'remove', path: `/hosts/${escape(host)}` })}>
        Delete
      </Button>
    </li>
  )
}

function SshTab({ policy, pending, queue }: { policy: Policy; pending: PatchOp[]; queue: (op: PatchOp) => void }) {
  const rules = schemaWithPendingChanges(policy.schema, pending).ssh ?? []

  return (
    <Section
      title="ssh"
      actions={
        <Button
          onClick={() =>
            queue({
              op: 'add',
              path: '/ssh/-',
              value: { action: 'accept', src: [], dst: [], users: ['root'] },
            })
          }
        >
          Add SSH rule
        </Button>
      }
    >
      {rules.length === 0 && <Empty title="No SSH rules" hint="Tailscale SSH is denied by default." />}

      <ul className="space-y-2">
        {rules.map((r, i) => (
          <SshRow key={i} index={i} rule={r} policy={policy} queue={queue} />
        ))}
      </ul>
    </Section>
  )
}

function SshRow({
  index,
  rule,
  policy,
  queue,
}: {
  index: number
  rule: SshRule
  policy: Policy
  queue: (op: PatchOp) => void
}) {
  const [action, setAction] = useState(rule.action)
  const [src, setSrc] = useState(rule.src ?? [])
  const [dst, setDst] = useState(rule.dst ?? [])
  const [users, setUsers] = useState((rule.users ?? []).join(', '))
  const [checkPeriod, setCheckPeriod] = useState(rule.checkPeriod ?? '')
  const nextUsers = users.split(',').map((user) => user.trim()).filter(Boolean)
  const issues = [...validateTokens(src, policy.tokens, 'src'), ...validateTokens(dst, policy.tokens, 'src')]
  const next = { ...rule, action, src, dst, users: nextUsers, ...(checkPeriod ? { checkPeriod } : { checkPeriod: undefined }) }
  const dirty = JSON.stringify(next) !== JSON.stringify(rule)

  return (
    <li className="space-y-3 rounded-lg border border-border bg-surface-1 p-3">
      <div className="flex flex-wrap items-end gap-2">
        <div>
          <span className="text-xs text-muted-foreground">action</span>
          <select
            value={action}
            onChange={(event) => setAction(event.target.value)}
            className="block rounded-md border border-border bg-surface-1 px-2 py-1.5 text-sm"
          >
            <option value="accept">accept</option>
            <option value="check">check</option>
          </select>
        </div>
        <div className="min-w-56 flex-1">
          <span className="text-xs text-muted-foreground">from</span>
          <TokenPicker values={src} onChange={setSrc} tokens={policy.tokens} slot="src" placeholder="group:eng…" />
        </div>
        <div className="min-w-56 flex-1">
          <span className="text-xs text-muted-foreground">to</span>
          <TokenPicker values={dst} onChange={setDst} tokens={policy.tokens} slot="src" placeholder="tag:prod…" />
        </div>
        <div className="min-w-40 flex-1">
          <span className="text-xs text-muted-foreground">users</span>
          <Input value={users} onChange={setUsers} placeholder="root, autogroup:nonroot" className="font-mono text-xs" />
        </div>
        {action === 'check' && (
          <div className="w-32">
            <span className="text-xs text-muted-foreground">check period</span>
            <Input value={checkPeriod} onChange={setCheckPeriod} placeholder="12h" className="font-mono text-xs" />
          </div>
        )}
        <Button disabled={!dirty || src.length === 0 || dst.length === 0 || nextUsers.length === 0 || issues.length > 0} onClick={() => queue({ op: 'replace', path: `/ssh/${index}`, value: next })}>
          Stage
        </Button>
        <Button variant="ghost" icon={Trash2} onClick={() => queue({ op: 'remove', path: `/ssh/${index}` })}>
          Delete
        </Button>
      </div>
      {issues.length > 0 && (
        <ul className="space-y-0.5 text-xs text-danger">
          {issues.map((issue) => <li key={issue.token}><span className="font-mono">{issue.token}</span> — {issue.reason}</li>)}
        </ul>
      )}
    </li>
  )
}

function AutoApproversTab({
  policy,
  pending,
  queue,
}: {
  policy: Policy
  pending: PatchOp[]
  queue: (op: PatchOp) => void
}) {
  const auto = schemaWithPendingChanges(policy.schema, pending).autoApprovers

  return (
    <div className="space-y-4">
      <p className="text-sm text-muted-foreground">These entries approve route advertisements automatically. They do not grant traffic; use a Grant for access permission.</p>
      <AutoApproverMap
        title="routes"
        entries={auto?.routes ?? {}}
        policy={policy}
        placeholder="10.0.0.0/24"
        onAdd={(name) => queueAutoApproverMap(queue, auto, 'routes', name)}
        onStage={(name, owners) => queue({ op: 'replace', path: `/autoApprovers/routes/${escape(name)}`, value: owners })}
        onDelete={(name) => queue({ op: 'remove', path: `/autoApprovers/routes/${escape(name)}` })}
      />

      <ExitNodeApprovers policy={policy} owners={auto?.exitNode ?? []} auto={auto} queue={queue} />
    </div>
  )
}

function AutoApproverMap({
  title,
  entries,
  policy,
  placeholder,
  onAdd,
  onStage,
  onDelete,
}: {
  title: string
  entries: Record<string, string[]>
  policy: Policy
  placeholder: string
  onAdd: (name: string) => void
  onStage: (name: string, owners: string[]) => void
  onDelete: (name: string) => void
}) {
  const [name, setName] = useState('')

  return (
    <Section title={title}>
      {Object.keys(entries).length === 0 && (
        <Empty title={`No ${title} auto-approvers`} hint="Headscale will require manual approval." />
      )}
      <ul className="space-y-2">
        {Object.entries(entries).map(([key, owners]) => (
          <MapListRow
            key={key}
            name={key}
            members={owners}
            policy={policy}
            slot="src"
            onStage={(next) => onStage(key, next)}
            onDelete={() => onDelete(key)}
          />
        ))}
      </ul>
      <div className="flex items-center gap-2">
        <Input value={name} onChange={setName} placeholder={placeholder} className="w-64 font-mono text-xs" />
        <Button disabled={name.trim() === ''} onClick={() => { onAdd(name.trim()); setName('') }}>
          Add
        </Button>
      </div>
    </Section>
  )
}

function ExitNodeApprovers({
  policy,
  owners,
  auto,
  queue,
}: {
  policy: Policy
  owners: string[]
  auto: NonNullable<Policy['schema']>['autoApprovers']
  queue: (op: PatchOp) => void
}) {
  const [values, setValues] = useState(owners)
  const issues = validateTokens(values, policy.tokens, 'src')
  const dirty = JSON.stringify(values) !== JSON.stringify(owners)

  return (
    <Section title="exit nodes">
      <div className="flex flex-wrap items-end gap-2">
        <div className="min-w-64 flex-1">
          <span className="text-xs text-muted-foreground">approved for</span>
          <TokenPicker values={values} onChange={setValues} tokens={policy.tokens} slot="src" placeholder="group:ops…" />
        </div>
        <Button
          disabled={!dirty || issues.length > 0}
          onClick={() => {
            if (!auto) queue({ op: 'add', path: '/autoApprovers', value: { exitNode: values } })
            else if (!auto.exitNode) queue({ op: 'add', path: '/autoApprovers/exitNode', value: values })
            else queue({ op: 'replace', path: '/autoApprovers/exitNode', value: values })
          }}
        >
          Stage
        </Button>
      </div>
      {issues.length > 0 && (
        <ul className="mt-2 space-y-0.5 text-xs text-danger">
          {issues.map((issue) => <li key={issue.token}><span className="font-mono">{issue.token}</span> — {issue.reason}</li>)}
        </ul>
      )}
    </Section>
  )
}

function queueAutoApproverMap(
  queue: (op: PatchOp) => void,
  auto: NonNullable<Policy['schema']>['autoApprovers'],
  section: 'routes',
  name: string,
) {
  if (!auto) queue({ op: 'add', path: '/autoApprovers', value: { [section]: { [name]: [] } } })
  else if (!auto[section]) queue({ op: 'add', path: `/autoApprovers/${section}`, value: { [name]: [] } })
  else queue({ op: 'add', path: `/autoApprovers/${section}/${escape(name)}`, value: [] })
}

function RawTab({
  text,
  onChange,
  dirty,
  onReset,
  location,
}: {
  text: string
  onChange: (v: string) => void
  dirty: boolean
  onReset: () => void
  location: string | null
}) {
  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between">
        <p className="text-sm text-muted-foreground">
          The document as stored. Editing here replaces the whole file.
        </p>
        {dirty && (
          <Button variant="ghost" onClick={onReset}>
            Revert
          </Button>
        )}
      </div>
      {location && <p className="text-xs text-muted-foreground">Policy location: <code>{location}</code></p>}
      <textarea
        value={text}
        onChange={(e) => onChange(e.target.value)}
        spellCheck={false}
        rows={28}
        className="w-full rounded-md border border-border bg-surface-1 p-3 font-mono text-xs leading-relaxed outline-none focus:border-accent-500"
      />
    </div>
  )
}

/** RFC 6901 escaping for pointer segments. */
function escape(token: string): string {
  return token.replaceAll('~', '~0').replaceAll('/', '~1')
}
