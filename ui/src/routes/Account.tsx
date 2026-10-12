import { useState } from 'react'
import { useMutation } from '@tanstack/react-query'
import { api, type Me } from '../lib/api'
import { Button, ErrorNote, Section, Status } from '../components/ui'
import { useToast } from '../components/toastContext'
import { KeyRound, LockKeyhole } from 'lucide-react'
import { PinField } from '../components/Sudo'
import { pinProblem } from '../lib/sudo'

/**
 * Your own account. Small on purpose — its reason to exist is the password
 * form: the first owner's password is generated and printed to a log, which is
 * fine for getting in once and no way to leave a deployment.
 */
export function Account({ me }: { me: Me }) {
  const toast = useToast()
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
      toast.ok('Password changed')
    },
  })

  const mismatch = confirm !== '' && next !== confirm
  const tooShort = next !== '' && next.length < 12
  const ready = current !== '' && next !== '' && !mismatch && !tooShort

  return (
    <div className="space-y-6">
      <header>
        <h1 className="text-display font-semibold lg:text-page">Account</h1>
        <p className="text-sm text-muted-foreground">{me.user.email}</p>
      </header>

      <Section title="Role">
        <div className="flex flex-wrap items-start gap-3 text-sm">
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

            <Button type="submit" variant="primary" icon={KeyRound} disabled={!ready || change.isPending}>
              {change.isPending ? 'Changing…' : 'Change password'}
            </Button>
          </form>
        ) : (
          <p className="text-sm text-muted-foreground">
            This account signs in through your identity provider, so its password lives there.
          </p>
        )}
      </Section>

      {me.pinSet && <PinSection />}
    </div>
  )
}

/** Change the sudo PIN. The server asks for the current one first (sudo), which opens the PIN prompt. */
function PinSection() {
  const toast = useToast()
  const [pin, setPin] = useState('')
  const [confirm, setConfirm] = useState('')
  const change = useMutation({
    mutationFn: () => api.setPin(pin),
    onSuccess: () => {
      setPin('')
      setConfirm('')
      toast.ok('PIN changed')
    },
  })
  const problem = confirm.length === 6 ? pinProblem(pin, confirm) : pin.length === 6 ? pinProblem(pin, pin) : null

  return (
    <Section title="Sudo PIN">
      <form
        onSubmit={(e) => {
          e.preventDefault()
          change.mutate()
        }}
        className="max-w-md space-y-4"
      >
        <p className="text-sm text-muted-foreground">
          Dangerous actions ask for this PIN, then stay unlocked for 5 minutes. Changing it asks for the current one first.
        </p>
        <PinField label="New PIN" value={pin} onChange={setPin} />
        <PinField label="Same PIN again" value={confirm} onChange={setConfirm} />
        {problem && <p className="text-sm text-danger">{problem}</p>}
        {change.error && <ErrorNote error={change.error} />}
        <Button type="submit" variant="primary" icon={LockKeyhole} disabled={pin.length !== 6 || confirm.length !== 6 || problem !== null || change.isPending}>
          {change.isPending ? 'Saving…' : 'Change PIN'}
        </Button>
        <p className="text-xs text-muted-foreground">
          Forgot it? Someone with a shell on the Headboard container can clear it:{' '}
          <code className="break-all">docker exec &lt;container&gt; headboard reset-pin --email you@example.com</code>
        </p>
      </form>
    </Section>
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
        className="h-10 w-full rounded-lg border border-border bg-surface-1 px-3 text-sm md:h-9"
      />
      {problem ? (
        <span className="block text-xs text-danger">{problem}</span>
      ) : (
        hint && <span className="block text-xs text-muted-foreground">{hint}</span>
      )}
    </label>
  )
}
