import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Plus, Trash2 } from 'lucide-react'
import { api, type SshKey } from '../lib/api'
import { Button, Confirm, Empty, ErrorNote, Input, Mono, Section } from './ui'
import { useToast } from './toastContext'
import { Loading, SkeletonRows } from './Skeleton'

export function SshKeysSection({ userId, userName }: { userId: number; userName: string }) {
  const qc = useQueryClient()
  const toast = useToast()
  const keys = useQuery({ queryKey: ['ssh-keys', userId], queryFn: () => api.sshKeys(userId) })
  const [publicKey, setPublicKey] = useState('')
  const [label, setLabel] = useState('')
  const [removing, setRemoving] = useState<SshKey | null>(null)

  const refresh = () => {
    void qc.invalidateQueries({ queryKey: ['ssh-keys', userId] })
    void qc.invalidateQueries({ queryKey: ['ssh-access'] })
  }

  const add = useMutation({
    mutationFn: () => api.addSshKey(userId, publicKey, label),
    onSuccess: (k) => {
      setPublicKey('')
      setLabel('')
      refresh()
      toast.ok(`Added ${k.label || k.fingerprint} to ${userName}`)
    },
    onError: toast.error,
  })

  const remove = useMutation({
    mutationFn: (id: number) => api.deleteSshKey(id),
    onSuccess: () => {
      refresh()
      toast.ok('Key removed; servers drop it on their next sync')
      setRemoving(null)
    },
    onError: toast.error,
  })

  return (
    <Section title="SSH keys">
      <div className="space-y-3">
        {keys.isPending && (
          <Loading label={`Loading ${userName}'s SSH keys`}>
            <SkeletonRows rows={1} cols={3} />
          </Loading>
        )}
        {keys.error ? <ErrorNote error={keys.error} /> : null}
        {keys.data?.keys.length === 0 && (
          <Empty title="No SSH keys" hint="Keys added here are written to every server where this person is assigned." />
        )}
        <ul className="divide-y divide-border rounded-lg border border-border">
          {keys.data?.keys.map((k) => (
            <li key={k.id} className="flex items-center gap-3 px-3 py-2 text-sm">
              <span className="w-24 shrink-0 font-mono text-xs text-muted-foreground">{k.type}</span>
              <span className="min-w-0 flex-1 truncate">{k.label || '—'}</span>
              <Mono value={k.fingerprint} />
              <button
                type="button"
                aria-label={`Remove key ${k.label || k.fingerprint}`}
                className="rounded p-1 text-muted-foreground hover:bg-surface-2 hover:text-foreground"
                onClick={() => setRemoving(k)}
              >
                <Trash2 className="size-4" aria-hidden />
              </button>
            </li>
          ))}
        </ul>
        <div className="grid gap-2 sm:grid-cols-[1fr_10rem_auto]">
          <textarea
            value={publicKey}
            onChange={(e) => setPublicKey(e.target.value)}
            placeholder="Paste a public key (ssh-ed25519 AAAA…)"
            aria-label="Paste a public key"
            rows={2}
            className="rounded-lg border border-border bg-surface-1 px-2 py-1.5 font-mono text-xs"
          />
          <Input value={label} onChange={setLabel} placeholder="Label, e.g. laptop" />
          <Button variant="primary" icon={Plus} disabled={publicKey.trim() === '' || add.isPending} onClick={() => add.mutate()}>
            Add key
          </Button>
        </div>
      </div>
      <Confirm
        open={removing !== null}
        title="Remove this key?"
        body={`${userName} will no longer be able to log in with ${removing?.label || removing?.fingerprint} once servers sync (about 30 seconds).`}
        confirmLabel="Remove key"
        onConfirm={() => removing && remove.mutate(removing.id)}
        onCancel={() => setRemoving(null)}
        busy={remove.isPending}
      />
    </Section>
  )
}
