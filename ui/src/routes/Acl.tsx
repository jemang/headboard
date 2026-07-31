import { useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, type AclRule, type PatchOp, type Policy, type PolicyPreview } from '../lib/api'
import { Button, Badge, Empty, ErrorNote, Input, Section } from '../components/ui'
import { TokenPicker, validateTokens } from '../components/TokenPicker'
import { TestsTab } from './AclTests'

type Tab = 'rules' | 'groups' | 'tags' | 'hosts' | 'ssh' | 'auto' | 'tests' | 'raw'

const tabs: { id: Tab; label: string }[] = [
  { id: 'rules', label: 'Rules' },
  { id: 'groups', label: 'Groups' },
  { id: 'tags', label: 'Tags' },
  { id: 'hosts', label: 'Hosts' },
  { id: 'ssh', label: 'SSH' },
  { id: 'auto', label: 'Auto-approvers' },
  { id: 'tests', label: 'Tests' },
  { id: 'raw', label: 'Raw HuJSON' },
]

export function Acl() {
  const qc = useQueryClient()
  const [tab, setTab] = useState<Tab>('rules')
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
    },
  })

  if (policy.isPending) return <p className="text-sm text-muted-foreground">Loading policy…</p>
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

    if (pointer.startsWith('/acls/')) setTab('rules')
    else if (pointer.startsWith('/ssh/')) setTab('ssh')
    else setTab('raw')
  }

  return (
    <div className="space-y-5">
      <header className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold">Access control</h1>
          <p className="text-sm text-muted-foreground">
            Edited as a form, stored as your HuJSON. Comments and layout outside the fields you
            change are left alone.
          </p>
        </div>
        <div className="flex items-center gap-2 text-xs text-muted-foreground">
          <span>sha</span>
          <code className="font-mono">{p.sha256.slice(0, 12)}</code>
        </div>
      </header>

      {p.parseError && (
        <div className="rounded-md border border-warn/40 bg-warn/10 px-3 py-2 text-sm">
          The stored policy does not parse, so the form is unavailable. Fix it in the raw editor.
          <div className="mt-1 font-mono text-xs">{p.parseError}</div>
        </div>
      )}

      <nav className="flex flex-wrap gap-1 border-b border-border">
        {tabs.map((t) => (
          <button
            key={t.id}
            type="button"
            onClick={() => setTab(t.id)}
            className={
              tab === t.id
                ? 'border-b-2 border-accent-500 px-3 py-2 text-sm font-medium'
                : 'border-b-2 border-transparent px-3 py-2 text-sm text-muted-foreground hover:text-foreground'
            }
          >
            {t.label}
          </button>
        ))}
      </nav>

      {tab === 'rules' && <RulesTab policy={p} queue={queue} highlight={highlight} />}
      {tab === 'groups' && <MapListTab policy={p} section="groups" queue={queue} slot="src" />}
      {tab === 'tags' && <MapListTab policy={p} section="tagOwners" queue={queue} slot="src" />}
      {tab === 'hosts' && <HostsTab policy={p} queue={queue} />}
      {tab === 'ssh' && <SshTab policy={p} queue={queue} />}
      {tab === 'auto' && <AutoApproversTab policy={p} />}
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
        />
      )}

      {change && (
        <div className="sticky bottom-0 space-y-3 border-t border-border bg-surface-0/95 py-4 backdrop-blur">
          <div className="flex flex-wrap items-center gap-2">
            <Badge tone="accent">
              {rawDraft !== null ? 'raw edit' : `${pending.length} pending change${pending.length === 1 ? '' : 's'}`}
            </Badge>
            <Input value={note} onChange={setNote} placeholder="Why? (saved with the revision)" className="flex-1" />
            <Button
              onClick={() => preview.mutate({ sha256: p.sha256, ...change })}
              disabled={preview.isPending}
            >
              {preview.isPending ? 'Checking…' : 'Preview diff'}
            </Button>
            <Button
              variant="ghost"
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
          {save.error && <ErrorNote error={save.error} />}

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

      <Button variant="primary" onClick={onSave} disabled={!preview.valid || saving}>
        {saving ? 'Saving…' : 'Save policy'}
      </Button>
    </div>
  )
}

function RulesTab({
  policy,
  queue,
  highlight,
}: {
  policy: Policy
  queue: (op: PatchOp) => void
  highlight: string | null
}) {
  const rules = policy.schema?.acls ?? []

  if (rules.length === 0) {
    return (
      <Empty
        title="No rules yet"
        hint="Without any rules every device can reach every other device."
      />
    )
  }

  return (
    <Section
      title={`${rules.length} rule${rules.length === 1 ? '' : 's'}`}
      actions={
        <Button
          onClick={() =>
            queue({
              op: 'add',
              path: '/acls/-',
              value: { action: 'accept', src: [], dst: [] },
            })
          }
        >
          Add rule
        </Button>
      }
    >
      <ol className="space-y-2">
        {rules.map((rule, i) => (
          <RuleRow
            key={i}
            index={i}
            rule={rule}
            policy={policy}
            queue={queue}
            highlighted={highlight === `/acls/${i}`}
          />
        ))}
      </ol>
    </Section>
  )
}

function RuleRow({
  index,
  rule,
  policy,
  queue,
  highlighted,
}: {
  index: number
  rule: AclRule
  policy: Policy
  queue: (op: PatchOp) => void
  highlighted?: boolean
}) {
  const [src, setSrc] = useState(rule.src ?? [])
  const [dst, setDst] = useState(rule.dst ?? [])

  const issues = useMemo(
    () => [
      ...validateTokens(src, policy.tokens, 'src'),
      ...validateTokens(dst, policy.tokens, 'dst'),
    ],
    [src, dst, policy.tokens],
  )

  const dirty = JSON.stringify(src) !== JSON.stringify(rule.src) ||
    JSON.stringify(dst) !== JSON.stringify(rule.dst)

  return (
    <li
      className={
        highlighted
          ? 'rounded-lg border border-accent-500 bg-surface-1 p-3 ring-1 ring-accent-500'
          : 'rounded-lg border border-border bg-surface-1 p-3'
      }
    >
      <div className="flex flex-wrap items-start gap-2">
        <span className="mt-1.5 w-8 shrink-0 font-mono text-xs text-muted-foreground">#{index}</span>
        <Badge tone="accent">{rule.action}</Badge>

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
            slot="dst"
            placeholder="tag:prod:443, autogroup:self:*…"
          />
        </div>

        <div className="mt-5 flex gap-1">
          <Button
            disabled={!dirty || issues.length > 0}
            onClick={() =>
              queue({ op: 'replace', path: `/acls/${index}`, value: { ...rule, src, dst } })
            }
          >
            Stage
          </Button>
          <Button variant="ghost" onClick={() => queue({ op: 'remove', path: `/acls/${index}` })}>
            Delete
          </Button>
        </div>
      </div>

      {issues.length > 0 && (
        <ul className="mt-2 space-y-0.5 pl-10 text-xs text-danger">
          {issues.map((issue) => (
            <li key={issue.token}>
              <span className="font-mono">{issue.token}</span> — {issue.reason}
            </li>
          ))}
        </ul>
      )}
    </li>
  )
}

/** groups and tagOwners share a shape: a name mapped to a list of owners. */
function MapListTab({
  policy,
  section,
  queue,
  slot,
}: {
  policy: Policy
  section: 'groups' | 'tagOwners'
  queue: (op: PatchOp) => void
  slot: 'src'
}) {
  const entries = Object.entries(
    (section === 'groups' ? policy.schema?.groups : policy.schema?.tagOwners) ?? {},
  )
  const [name, setName] = useState('')

  const prefix = section === 'groups' ? 'group:' : 'tag:'

  return (
    <Section title={section}>
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

  return (
    <li className="flex flex-wrap items-center gap-2 rounded-lg border border-border bg-surface-1 p-2.5">
      <code className="w-48 shrink-0 font-mono text-xs">{name}</code>
      <TokenPicker values={values} onChange={setValues} tokens={policy.tokens} slot={slot} />
      <Button disabled={!dirty} onClick={() => onStage(values)}>
        Stage
      </Button>
      <Button variant="ghost" onClick={onDelete}>
        Delete
      </Button>
    </li>
  )
}

function HostsTab({ policy, queue }: { policy: Policy; queue: (op: PatchOp) => void }) {
  const entries = Object.entries(policy.schema?.hosts ?? {})
  const [name, setName] = useState('')
  const [cidr, setCidr] = useState('')

  return (
    <Section title="hosts">
      {entries.length === 0 && <Empty title="No hosts defined" hint="A host is a name for a CIDR." />}

      <ul className="space-y-2">
        {entries.map(([host, value]) => (
          <li
            key={host}
            className="flex items-center gap-2 rounded-lg border border-border bg-surface-1 p-2.5"
          >
            <code className="w-48 font-mono text-xs">{host}</code>
            <code className="flex-1 font-mono text-xs text-muted-foreground">{value}</code>
            <Button variant="ghost" onClick={() => queue({ op: 'remove', path: `/hosts/${escape(host)}` })}>
              Delete
            </Button>
          </li>
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

function SshTab({ policy, queue }: { policy: Policy; queue: (op: PatchOp) => void }) {
  const rules = policy.schema?.ssh ?? []

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
          <li key={i} className="rounded-lg border border-border bg-surface-1 p-3">
            <div className="flex flex-wrap items-center gap-2">
              <Badge tone={r.action === 'check' ? 'warn' : 'accent'}>{r.action}</Badge>
              <code className="font-mono text-xs">{(r.src ?? []).join(', ')}</code>
              <span className="text-muted-foreground">→</span>
              <code className="font-mono text-xs">{(r.dst ?? []).join(', ')}</code>
              <span className="text-xs text-muted-foreground">as</span>
              <code className="font-mono text-xs">{(r.users ?? []).join(', ')}</code>
              <Button variant="ghost" className="ml-auto" onClick={() => queue({ op: 'remove', path: `/ssh/${i}` })}>
                Delete
              </Button>
            </div>
          </li>
        ))}
      </ul>
    </Section>
  )
}

function AutoApproversTab({ policy }: { policy: Policy }) {
  const auto = policy.schema?.autoApprovers

  if (!auto || (!auto.routes && !auto.exitNode)) {
    return (
      <Empty
        title="No auto-approvers"
        hint="Every advertised route needs an admin to approve it by hand."
      />
    )
  }

  return (
    <div className="space-y-4">
      <Section title="routes">
        <ul className="space-y-1">
          {Object.entries(auto.routes ?? {}).map(([cidr, owners]) => (
            <li key={cidr} className="flex items-center gap-3 rounded-md border border-border bg-surface-1 px-3 py-2">
              <code className="w-48 font-mono text-xs">{cidr}</code>
              <code className="font-mono text-xs text-muted-foreground">{owners.join(', ')}</code>
            </li>
          ))}
        </ul>
      </Section>

      <Section title="exit nodes">
        <code className="font-mono text-xs">{(auto.exitNode ?? []).join(', ') || '—'}</code>
      </Section>
    </div>
  )
}

function RawTab({
  text,
  onChange,
  dirty,
  onReset,
}: {
  text: string
  onChange: (v: string) => void
  dirty: boolean
  onReset: () => void
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
