/**
 * Messages for inputs whose button stays disabled until they are valid, so a
 * disabled button always comes with the reason beside it. Each returns null
 * while there is nothing to say (empty, or fine).
 */

const PREFIX = 'hskey-authreq-'

export function registrationIdHint(id: string): string | null {
  const v = id.trim()
  if (v === '') return null
  if (!v.startsWith(PREFIX)) return `Registration ids start with ${PREFIX} — copy the whole id the device printed.`
  if (v.length <= PREFIX.length + 4) return 'That id looks incomplete — copy the whole id the device printed.'

  return null
}

const octet = '(25[0-5]|2[0-4]\\d|1\\d\\d|[1-9]?\\d)'
const IPV4 = new RegExp(`^${octet}(\\.${octet}){3}(\\/(3[0-2]|[12]?\\d))?$`)
const IPV6 = /^[0-9a-f:]+(\/(12[0-8]|1[01]\d|[1-9]?\d))?$/i

export function hostEntryProblem(name: string, address: string, existing: string[] = []): string | null {
  const n = name.trim()
  const a = address.trim()
  if (n === '' || a === '') return null
  if (!/^[a-z0-9][a-z0-9.-]*$/.test(n)) return 'Use a lowercase name: letters, digits, dots and hyphens.'
  if (existing.includes(n)) return `${n} is already in hosts.`
  if (!IPV4.test(a) && !(a.includes(':') && IPV6.test(a))) return 'The address must be an IP or a CIDR, e.g. 192.0.2.10 or 10.0.0.0/24.'

  return null
}

export function unixUserProblem(name: string, existing: string[]): string | null {
  if (name === '') return null
  if (!/^[a-z_][a-z0-9_-]{0,31}$/.test(name)) return 'Unix users are lowercase letters, digits, - and _, starting with a letter or _.'
  if (existing.includes(name)) return `${name} is already listed on this server.`

  return null
}
