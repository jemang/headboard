import { useEffect, useMemo, useState } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import {
  api,
  type Assertion,
  type Device,
  type PatchOp,
  type Policy,
  type PolicyDraft,
  type Simulation,
} from '../lib/api'
import { Badge, Button, Empty, ErrorNote, Input, Section, Status } from '../components/ui'
import { FlaskConical, Play, Plus, Radar, Trash2 } from 'lucide-react'
import { TokenPicker, validateTokens } from '../components/TokenPicker'

/**
 * Tests and the simulator sit together because they answer the same question
 * from opposite ends: the tests block states what must be true and fails the
 * save when it is not, while the simulator asks about one connection you have
 * not written an assertion for yet.
 */
export function TestsTab({
  policy,
  draft,
  queue,
  onJump,
}: {
  policy: Policy
  draft: PolicyDraft | null
  queue: (op: PatchOp) => void
  onJump: (pointer: string) => void
}) {
  return (
    <div className="space-y-8">
      <TestsRunner policy={policy} draft={draft} queue={queue} />
      <Simulator policy={policy} draft={draft} onJump={onJump} />
    </div>
  )
}

/** Clear a result whenever the pending change it was computed against moves. */
function useStaleReset(draft: PolicyDraft | null, reset: () => void) {
  const key = draft ? JSON.stringify(draft) : ''

  useEffect(() => {
    reset()
  }, [key, reset])
}

/** The key a result and its declared assertion agree on. */
function assertionKey(a: Pick<Assertion, 'section' | 'index' | 'kind' | 'dst' | 'user'>) {
  return `${a.section}/${a.index}/${a.kind}/${a.dst}/${a.user ?? ''}`
}

/**
 * Expand the document's test entries the same way the server does, so the list
 * is visible before anything is run and results can be merged into it.
 */
function declaredAssertions(policy: Policy): Assertion[] {
  const out: Assertion[] = []

  const push = (a: Omit<Assertion, 'passed' | 'pointer'>) =>
    out.push({ ...a, pointer: `/${a.section}/${a.index}`, passed: false })

  ;(policy.schema?.tests ?? []).forEach((t, index) => {
    for (const dst of t.accept ?? [])
      push({ section: 'tests', index, kind: 'accept', src: t.src, dst, proto: t.proto })
    for (const dst of t.deny ?? [])
      push({ section: 'tests', index, kind: 'deny', src: t.src, dst, proto: t.proto })
  })
  ;(policy.schema?.sshTests ?? []).forEach((t, index) => {
    const kinds = [
      ['accept', t.accept],
      ['deny', t.deny],
      ['check', t.check],
    ] as const

    for (const [kind, users] of kinds)
      for (const user of users ?? [])
        for (const dst of t.dst ?? []) push({ section: 'sshTests', index, kind, src: t.src, dst, user })
  })

  return out
}

function TestsRunner({
  policy,
  draft,
  queue,
}: {
  policy: Policy
  draft: PolicyDraft | null
  queue: (op: PatchOp) => void
}) {
  // Grouped by entry, because an entry is what a Delete removes and what the
  // pointer names — a flat list would offer to delete the same entry once per
  // destination it asserts.
  const entries = useMemo(() => {
    const byEntry = new Map<string, Assertion[]>()

    for (const a of declaredAssertions(policy)) {
      const group = byEntry.get(a.pointer)

      if (group) group.push(a)
      else byEntry.set(a.pointer, [a])
    }

    return [...byEntry.entries()]
  }, [policy])

  const run = useMutation({
    mutationFn: () => api.runPolicyTests(draft ? { sha256: policy.sha256, ...draft } : undefined),
  })

  // Results describe the exact document they ran against, so staging or
  // discarding a change invalidates them. Leaving a green run on screen after
  // the policy underneath moved is the one failure mode this panel cannot have.
  useStaleReset(draft, run.reset)

  // Results are merged onto the declared list rather than replacing it, so a
  // run against a draft still lines up with the entries on screen.
  const results = useMemo(() => {
    const byKey = new Map<string, Assertion>()
    for (const a of run.data?.assertions ?? []) byKey.set(assertionKey(a), a)

    return byKey
  }, [run.data])

  const failed = (run.data?.assertions ?? []).filter((a) => !a.passed).length

  return (
    <Section
      title="Assertions"
      actions={
        <div className="flex items-center gap-2">
          {draft && <Badge tone="accent">testing your pending change</Badge>}
          <Button variant="primary" icon={Play} disabled={run.isPending} onClick={() => run.mutate()}>
            {run.isPending ? 'Running…' : 'Run tests'}
          </Button>
        </div>
      }
    >
      <p className="text-sm text-muted-foreground">
        Headscale runs this block on every write and <strong>refuses to save</strong> a policy whose
        tests fail, so these are the write boundary rather than documentation. Each destination is
        evaluated on its own, which is why a failure names the assertion instead of the block.
      </p>

      {run.error && <ErrorNote error={run.error} />}

      {run.data && (
        <div className="flex items-center gap-4">
          {run.data.ran ? (
            <Status
              ok={run.data.allPassed}
              bad={!run.data.allPassed}
              label={
                run.data.allPassed
                  ? `${run.data.assertions.length} assertion${run.data.assertions.length === 1 ? '' : 's'} passed`
                  : `${failed} of ${run.data.assertions.length} failed`
              }
            />
          ) : (
            <Status ok={false} label="this policy asserts nothing" />
          )}
        </div>
      )}

      {entries.length === 0 ? (
        <Empty
          icon={FlaskConical}
          title="No tests yet"
          hint="A test pins down something the policy must keep true — that ops can still SSH to production, or that contractors still cannot."
        />
      ) : (
        <ul className="space-y-2">
          {entries.map(([pointer, group]) => (
            <li key={pointer} className="rounded-lg border border-border bg-surface-1 p-3">
              <div className="flex flex-wrap items-center gap-2">
                <span className="font-mono text-xs text-muted-foreground">{pointer}</span>
                <span className="font-mono text-sm">{group[0].src}</span>
                {group[0].proto && <Badge>{group[0].proto}</Badge>}

                <Button
                  variant="ghost"
                  icon={Trash2}
                  className="ml-auto"
                  onClick={() => queue({ op: 'remove', path: pointer })}
                >
                  Delete
                </Button>
              </div>

              <ul className="mt-2 space-y-1">
                {group.map((a) => {
                  const result = results.get(assertionKey(a))

                  return (
                    <li key={assertionKey(a)} className="flex flex-wrap items-center gap-x-3 gap-y-1">
                      {result ? (
                        <Status
                          ok={result.passed}
                          bad={!result.passed}
                          label={result.passed ? 'pass' : 'fail'}
                        />
                      ) : (
                        <Status ok={false} label="not run" />
                      )}

                      <Badge tone={a.kind === 'deny' ? 'warn' : 'accent'}>{a.kind}</Badge>

                      <span className="font-mono text-sm">
                        {a.user && <span className="text-muted-foreground">{a.user} @ </span>}
                        {a.dst}
                      </span>

                      {result?.error && (
                        <pre className="w-full overflow-x-auto whitespace-pre-wrap font-mono text-xs text-danger">
                          {result.error}
                        </pre>
                      )}
                    </li>
                  )
                })}
              </ul>
            </li>
          ))}
        </ul>
      )}

      <AddTest policy={policy} queue={queue} />
    </Section>
  )
}

function AddTest({ policy, queue }: { policy: Policy; queue: (op: PatchOp) => void }) {
  const [src, setSrc] = useState<string[]>([])
  const [dst, setDst] = useState('')
  const [kind, setKind] = useState<'accept' | 'deny'>('accept')

  const issues = validateTokens(src, policy.tokens, 'src')
  const ready = src.length === 1 && dst.trim() !== '' && issues.length === 0

  return (
    <div className="space-y-2 rounded-lg border border-dashed border-border p-3">
      <div className="flex flex-wrap items-end gap-2">
        <div className="min-w-56 flex-1">
          <span className="text-xs text-muted-foreground">from (one source)</span>
          <TokenPicker
            values={src}
            onChange={(v) => setSrc(v.slice(-1))}
            tokens={policy.tokens}
            slot="src"
            placeholder="alice@, group:ops…"
          />
        </div>

        <select
          value={kind}
          onChange={(e) => setKind(e.target.value as 'accept' | 'deny')}
          className="rounded-md border border-border bg-surface-1 px-2 py-1.5 text-sm"
        >
          <option value="accept">must reach</option>
          <option value="deny">must not reach</option>
        </select>

        <Input
          value={dst}
          onChange={setDst}
          placeholder="tag:prod:22"
          className="min-w-48 flex-1"
        />

        <Button
          icon={Plus}
          disabled={!ready}
          onClick={() => {
            queue({
              op: 'add',
              path: '/tests/-',
              value: { src: src[0], [kind]: [dst.trim()] },
            })
            setSrc([])
            setDst('')
          }}
        >
          Add assertion
        </Button>
      </div>

      <p className="text-xs text-muted-foreground">
        A destination is one host and one port — <code>tag:prod:22</code>. Ranges, CIDRs and
        <code> autogroup:internet</code> have no single yes-or-no answer, so Headscale rejects them
        here.
      </p>

      {issues.length > 0 && (
        <ul className="space-y-0.5 text-xs text-danger">
          {issues.map((issue) => (
            <li key={issue.token}>
              <span className="font-mono">{issue.token}</span> — {issue.reason}
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

function Simulator({
  policy,
  draft,
  onJump,
}: {
  policy: Policy
  draft: PolicyDraft | null
  onJump: (pointer: string) => void
}) {
  const devices = useQuery({ queryKey: ['devices'], queryFn: api.devices })

  const [src, setSrc] = useState(0)
  const [dst, setDst] = useState(0)
  const [port, setPort] = useState('22')

  const simulate = useMutation({
    mutationFn: () =>
      api.simulate({
        src,
        dst,
        port: Number(port),
        ...(draft ? { sha256: policy.sha256, ...draft } : {}),
      }),
  })

  useStaleReset(draft, simulate.reset)

  const list = devices.data?.devices ?? []
  const ready = src !== 0 && dst !== 0 && Number(port) > 0

  return (
    <Section title="Reachability simulator">
      <p className="text-sm text-muted-foreground">
        Evaluated against the destination's own filter, so rules written with{' '}
        <code>autogroup:self</code> count — they are compiled per node and never appear in the
        tailnet-wide rule set.
      </p>

      <div className="flex flex-wrap items-end gap-2">
        <DevicePicker label="from" value={src} onChange={setSrc} devices={list} />
        <span className="pb-2 text-muted-foreground">→</span>
        <DevicePicker label="to" value={dst} onChange={setDst} devices={list} />

        <div>
          <span className="text-xs text-muted-foreground">port</span>
          <Input value={port} onChange={setPort} placeholder="22" className="w-24" />
        </div>

        <Button variant="primary" icon={Radar} disabled={!ready || simulate.isPending} onClick={() => simulate.mutate()}>
          {simulate.isPending ? 'Checking…' : 'Check'}
        </Button>

        {draft && <Badge tone="accent">against your pending change</Badge>}
      </div>

      {simulate.error && <ErrorNote error={simulate.error} />}
      {simulate.data && <SimulationResult sim={simulate.data} onJump={onJump} />}
    </Section>
  )
}

function DevicePicker({
  label,
  value,
  onChange,
  devices,
}: {
  label: string
  value: number
  onChange: (v: number) => void
  devices: Device[]
}) {
  return (
    <div>
      <span className="text-xs text-muted-foreground">{label}</span>
      <select
        value={value}
        onChange={(e) => onChange(Number(e.target.value))}
        className="block rounded-md border border-border bg-surface-1 px-2 py-1.5 text-sm"
      >
        <option value={0}>— choose a device —</option>
        {devices.map((d) => (
          <option key={d.id} value={d.id}>
            {d.name}
            {d.owner ? ` (${d.owner})` : d.tags?.length ? ` (${d.tags.join(', ')})` : ''}
          </option>
        ))}
      </select>
    </div>
  )
}

function SimulationResult({ sim, onJump }: { sim: Simulation; onJump: (pointer: string) => void }) {
  return (
    <div
      className={
        sim.allowed
          ? 'space-y-2 rounded-lg border border-ok/40 bg-ok/5 p-3'
          : 'space-y-2 rounded-lg border border-border bg-surface-1 p-3'
      }
    >
      <div className="flex flex-wrap items-center gap-2">
        <Status ok={sim.allowed} label={sim.allowed ? 'allowed' : 'denied'} />
        <span className="font-mono text-sm">
          {sim.source.label}
          <span className="mx-1.5 text-muted-foreground">→</span>
          {sim.dest.label}:{sim.port}
        </span>
      </div>

      {sim.rule && (
        <div className="flex flex-wrap items-baseline gap-2 text-xs">
          <span className="text-muted-foreground">matched rule</span>
          <span className="font-mono">
            {sim.rule.sources.map((s) => s.label).join(', ')} →{' '}
            {sim.rule.dests.map((d) => `${d.label}:${d.ports}`).join(', ')}
          </span>
        </div>
      )}

      {sim.because ? (
        <Button variant="ghost" onClick={() => onJump(sim.because!.pointer)}>
          Show the rule that allows it ({sim.because.pointer})
        </Button>
      ) : (
        !sim.allowed && (
          <p className="text-sm text-muted-foreground">
            No rule permits this. Nothing to point at — a denial is the absence of a rule, not a
            rule of its own.
          </p>
        )
      )}
    </div>
  )
}
