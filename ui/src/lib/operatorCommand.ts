import { shellArg } from './serverRegistrationCommand'

export type OperatorCommandKind =
  | 'advertise-subnet'
  | 'accept-routes'
  | 'advertise-exit'
  | 'use-exit'
  | 'enable-ssh'
  | 'status'
  | 'netcheck'
  | 'ping'

export function operatorCommand(kind: OperatorCommandKind, value = '') {
  const arg = value.trim()

  if ((kind === 'advertise-subnet' || kind === 'use-exit' || kind === 'ping') && !arg) return ''
  if (kind === 'advertise-subnet') return `tailscale set --advertise-routes=${shellArg(arg)}`
  if (kind === 'accept-routes') return 'tailscale set --accept-routes'
  if (kind === 'advertise-exit') return 'tailscale set --advertise-exit-node'
  if (kind === 'use-exit') return `tailscale set --exit-node=${shellArg(arg)}`
  if (kind === 'enable-ssh') return 'tailscale set --ssh'
  if (kind === 'status') return 'tailscale status'
  if (kind === 'netcheck') return 'tailscale netcheck'

  return `tailscale ping ${shellArg(arg)}`
}
