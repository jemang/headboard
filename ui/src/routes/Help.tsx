import { useState, type ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api } from '../lib/api'
import { ErrorNote, Input, Mono, Section } from '../components/ui'
import { Link } from '../lib/router'
import { operatorCommand } from '../lib/operatorCommand'
import { serverRegistrationCommand } from '../lib/serverRegistrationCommand'

export function Help() {
  const [tag, setTag] = useState('tag:server')
  const [cidrs, setCidrs] = useState('192.168.1.0/24')
  const [exitNode, setExitNode] = useState('')
  const [pingTarget, setPingTarget] = useState('')
  const registrationInfo = useQuery({ queryKey: ['registration-info'], queryFn: api.registrationInfo })
  const url = registrationInfo.data?.headscalePublicUrl

  return (
    <div className="space-y-6">
      <header>
        <p className="text-eyebrow font-semibold uppercase text-muted-foreground">Operator reference</p>
        <h1 className="mt-1 text-display font-semibold">Help</h1>
        <p className="max-w-3xl text-sm text-muted-foreground">
          Copy a command, run it on the target device, then use Headboard to approve its registration and routes.
          These commands do not contain a key or automatically connect a device.
        </p>
      </header>

      <Section title="Enrol and assign" actions={<span className="text-xs text-muted-foreground">manual approval required</span>}>
        {registrationInfo.error && <ErrorNote error={registrationInfo.error} />}
        <div className="grid gap-3 xl:grid-cols-2">
          <CommandCard
            title="Enrol a device"
            description="Start a standard device registration against this Headscale control server."
            command={url ? serverRegistrationCommand(url, 'standard', '') : ''}
            pending={!registrationInfo.error && !url}
            note={<><code>hskey-authreq-…</code> is printed on the device. Paste it into <Link to="/keys" className="text-accent-700 underline underline-offset-2 dark:text-accent-400">Keys</Link> to approve the request.</>}
          />
          <CommandCard
            title="Enrol a tagged server"
            description="Attach a service tag when the server starts registration."
            command={url && tag.trim() ? serverRegistrationCommand(url, 'tagged', tag.trim()) : ''}
            pending={!registrationInfo.error && !url}
            emptyHint="Enter at least one tag to generate the command."
            note={<>Every tag must be allowed by your policy’s <code>tagOwners</code>, and the request still needs approval in <Link to="/keys" className="text-accent-700 underline underline-offset-2 dark:text-accent-400">Keys</Link>.</>}
          >
            <Field label="Tags"><Input value={tag} onChange={setTag} placeholder="tag:server,tag:prod" /></Field>
          </CommandCard>
        </div>
      </Section>

      <Section title="Route traffic" actions={<span className="text-xs text-muted-foreground">review before approving</span>}>
        <div className="grid gap-3 xl:grid-cols-2">
          <CommandCard
            title="Advertise subnet routes"
            description="Expose private networks behind an already enrolled Linux router."
            command={operatorCommand('advertise-subnet', cidrs)}
            emptyHint="Enter one or more CIDR routes to generate the command."
            note={<>Enable IP forwarding on the router, then explicitly approve the advertised routes from its <Link to="/devices" className="text-accent-700 underline underline-offset-2 dark:text-accent-400">device details</Link>.</>}
          >
            <Field label="CIDR routes"><Input value={cidrs} onChange={setCidrs} placeholder="10.0.0.0/24,192.168.1.0/24" /></Field>
          </CommandCard>
          <CommandCard
            title="Accept approved routes"
            description="Let this device use subnet routes that another device has already advertised and Headboard has approved."
            command={operatorCommand('accept-routes')}
            note="This does not advertise a route or grant access. The tailnet policy still determines which traffic is permitted."
          />
          <CommandCard
            title="Advertise an exit node"
            description="Offer this device as a gateway for other devices’ internet traffic."
            command={operatorCommand('advertise-exit')}
            note={<>Enable IP forwarding, then explicitly approve the exit node in its <Link to="/devices" className="text-accent-700 underline underline-offset-2 dark:text-accent-400">device details</Link> before anyone can use it.</>}
          />
          <CommandCard
            title="Use an exit node"
            description="Send this device’s internet traffic through an approved exit node."
            command={operatorCommand('use-exit', exitNode)}
            emptyHint="Enter the approved exit node hostname to generate the command."
            note="The chosen exit node must be approved, and policy must allow this device to use internet routing."
          >
            <Field label="Exit node hostname"><Input value={exitNode} onChange={setExitNode} placeholder="gateway" /></Field>
          </CommandCard>
        </div>
        <p className="text-xs text-muted-foreground">
          Platform-specific IP-forwarding instructions are in the{' '}
          <a className="text-accent-700 underline underline-offset-2 dark:text-accent-400" href="https://headscale.net/stable/ref/routes/" target="_blank" rel="noreferrer">Headscale routing guide</a>.
        </p>
      </Section>

      <Section title="Secure access and diagnose">
        <div className="grid gap-3 xl:grid-cols-2">
          <CommandCard
            title="Enable Tailscale SSH"
            description="Run a Tailscale SSH server on this device."
            command={operatorCommand('enable-ssh')}
            note="Your SSH policy remains authoritative: enabling the server alone does not grant anyone access."
          />
          <CommandCard title="Inspect connection status" description="List peers, addresses, and the current device state." command={operatorCommand('status')} note="Run this first when a device does not appear or cannot reach a peer." />
          <CommandCard title="Check network paths" description="Check direct-connect and relay conditions for this device." command={operatorCommand('netcheck')} note="Use the output to investigate firewall, NAT, or relay-only connectivity." />
          <CommandCard
            title="Ping a tailnet device"
            description="Test Tailscale reachability to a device by its hostname."
            command={operatorCommand('ping', pingTarget)}
            emptyHint="Enter a device hostname to generate the command."
            note="A successful ping only proves reachability; application access is still governed by policy."
          >
            <Field label="Device hostname"><Input value={pingTarget} onChange={setPingTarget} placeholder="db-1" /></Field>
          </CommandCard>
        </div>
      </Section>
    </div>
  )
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return <label className="block space-y-1"><span className="text-xs text-muted-foreground">{label}</span>{children}</label>
}

function CommandCard({
  title,
  description,
  command,
  note,
  children,
  emptyHint = 'A value is required to generate this command.',
  pending = false,
}: {
  title: string
  description: string
  command: string
  note: ReactNode
  children?: ReactNode
  emptyHint?: string
  pending?: boolean
}) {
  return (
    <article className="space-y-3 rounded-lg border border-border bg-surface-0 p-3">
      <div>
        <h2 className="text-sm font-semibold">{title}</h2>
        <p className="mt-0.5 text-sm text-muted-foreground">{description}</p>
      </div>
      {children}
      {command ? (
        <div className="flex items-center gap-2 rounded-md bg-surface-2 px-2 py-1.5">
          <code className="min-w-0 flex-1 overflow-x-auto whitespace-nowrap font-mono text-xs">{command}</code>
          <Mono compact label={`${title.toLowerCase()} command`} value={command} className="shrink-0 border border-border px-2 py-1.5" />
        </div>
      ) : <p className="text-xs text-muted-foreground">{pending ? 'Loading the Headscale address…' : emptyHint}</p>}
      <p className="text-xs leading-5 text-muted-foreground">{note}</p>
    </article>
  )
}
