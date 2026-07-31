import { useState } from 'react'
import { useMutation } from '@tanstack/react-query'
import { api, type Me } from '../lib/api'
import { Button, ErrorNote, Section, Status } from '../components/ui'

/**
 * Your own account. Small on purpose — its reason to exist is the password
 * form: the first owner's password is generated and printed to a log, which is
 * fine for getting in once and no way to leave a deployment.
 */
export function Account({ me }: { me: Me }) {
  const [current, setCurrent] = useState('')
  const [next, setNext] = useState('')
  const [confirm, setConfirm] = useState('')
  const [done, setDone] = useState(false)

  const change = useMutation({
    mutationFn: () => api.changePassword(current, next),
    onSuccess: () => {
      setCurrent('')
      setNext('')
      setConfirm('')
      setDone(true)
    },
  })

  const mismatch = confirm !== '' && next !== confirm
  const tooShort = next !== '' && next.length < 12
  const ready = current !== '' && next !== '' && !mismatch && !tooShort

  return (
    <div className="space-y-6">
      <header>
        <h1 className="text-xl font-semibold">Account</h1>
        <p className="text-sm text-muted-foreground">{me.user.email}</p>
      </header>

      <Section title="Role">
        <div className="flex items-center gap-3 text-sm">
          <span className="rounded bg-surface-2 px-1.5 py-0.5 font-mono text-xs">
            {me.user.role}
          </span>
          <Status
            ok={me.linked}
            warn={!me.linked}
            label={
              me.linked
                ? 'linked to a Headscale user'
                : 'not linked to a Headscale user — an admin has to link it'
            }
          />
        </div>
      </Section>

      <Section title="Password">
        {me.local ? (
          <form
            className="max-w-sm space-y-3"
            onSubmit={(e) => {
              e.preventDefault()
              change.mutate()
            }}
          >
            {change.error && <ErrorNote error={change.error} />}

            {done && (
              <div className="rounded-md border border-ok/40 bg-ok/10 px-3 py-2 text-sm">
                Password changed.
              </div>
            )}

            <Field
              label="Current password"
              value={current}
              onChange={setCurrent}
              autoComplete="current-password"
            />
            <Field
              label="New password"
              value={next}
              onChange={setNext}
              autoComplete="new-password"
              hint="At least 12 characters."
              problem={tooShort ? 'Too short — at least 12 characters.' : undefined}
            />
            <Field
              label="Confirm new password"
              value={confirm}
              onChange={setConfirm}
              autoComplete="new-password"
              problem={mismatch ? 'These do not match.' : undefined}
            />

            <Button type="submit" variant="primary" disabled={!ready || change.isPending}>
              {change.isPending ? 'Changing…' : 'Change password'}
            </Button>
          </form>
        ) : (
          <p className="text-sm text-muted-foreground">
            This account signs in through your identity provider, so its password lives there.
          </p>
        )}
      </Section>
    </div>
  )
}

function Field({
  label,
  value,
  onChange,
  autoComplete,
  hint,
  problem,
}: {
  label: string
  value: string
  onChange: (v: string) => void
  autoComplete: string
  hint?: string
  problem?: string
}) {
  return (
    <label className="block space-y-1">
      <span className="text-xs text-muted-foreground">{label}</span>
      <input
        type="password"
        autoComplete={autoComplete}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className="w-full rounded-md border border-border bg-surface-1 px-2.5 py-1.5 text-sm"
      />
      {problem ? (
        <span className="block text-xs text-danger">{problem}</span>
      ) : (
        hint && <span className="block text-xs text-muted-foreground">{hint}</span>
      )}
    </label>
  )
}
