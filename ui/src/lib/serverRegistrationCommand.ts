export type ServerRegistrationKind = 'standard' | 'tagged' | 'subnet' | 'exit'

export function shellArg(value: string) {
  return `'${value.replaceAll("'", "'\"'\"'")}'`
}

export function serverRegistrationCommand(
  headscalePublicURL: string,
  kind: ServerRegistrationKind,
  value: string,
) {
  const args = ['tailscale up', `--login-server=${shellArg(headscalePublicURL)}`]

  if (kind === 'tagged') args.push(`--advertise-tags=${shellArg(value)}`)
  if (kind === 'subnet') args.push(`--advertise-routes=${shellArg(value)}`)
  if (kind === 'exit') args.push('--advertise-exit-node')

  return args.join(' ')
}
