import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Plus, Trash2 } from 'lucide-react'
import { api, type SshKey } from '../lib/api'
import { Badge, Button, Confirm, Empty, ErrorNote, Input, Section, IconButton } from './ui'
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
        {(keys.data?.keys.length ?? 0) > 0 && (
          <ul className="divide-y divide-border overflow-hidden rounded-xl border border-border bg-surface-1 shadow-card">
            {keys.data?.keys.map((k) => (
              <li key={k.id} className="flex items-center gap-3 px-3 py-2.5 text-sm">
                <div className="min-w-0 flex-1">
                  <p className="flex items-center gap-2">
                    <span className="truncate font-medium">{k.label || 'Unlabelled key'}</span>
                    <Badge>{k.type}</Badge>
                  </p>
                  {/* Fingerprints have no break points; let them wrap inside the row. */}
                  <p className="mt-0.5 break-all font-mono text-xs text-muted-foreground">{k.fingerprint}</p>
                </div>
                <IconButton icon={Trash2} variant="danger" label={`Remove key ${k.label || k.fingerprint}`} onClick={() => setRemoving(k)} />
              </li>
            ))}
          </ul>
        )}
        <div className="space-y-2">
          <textarea
            value={publicKey}
            onChange={(e) => setPublicKey(e.target.value)}
            placeholder="Paste a public key (ssh-ed25519 AAAA…)"
            aria-label="Paste a public key"
            rows={3}
            className="block w-full rounded-lg border border-border bg-surface-1 px-3 py-2 font-mono text-xs placeholder:text-muted-foreground focus:border-accent-500 focus:outline-none"
          />
          <div className="flex flex-col gap-2 sm:flex-row">
            <Input value={label} onChange={setLabel} placeholder="Label, e.g. laptop" className="sm:flex-1" />
            <Button variant="primary" icon={Plus} disabled={publicKey.trim() === '' || add.isPending} onClick={() => add.mutate()}>
              Add key
            </Button>
          </div>
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
