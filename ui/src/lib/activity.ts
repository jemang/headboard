import type { AuditCategory, AuditEntry } from './api'

export interface Described {
  /** Who did it: an email, "Agent pve1", "Command line", or "Someone". */
  actor: string
  /** The rest of the sentence after the actor, lower case. */
  text: string
  /** A short second line: what changed, from → to, a reason. */
  detail?: string
  category: Exclude<AuditCategory, ''>
  /** warn marks what an admin scanning for trouble should notice. */
  tone: 'default' | 'warn' | 'danger'
}

type Obj = Record<string, unknown>

const obj = (v: unknown): Obj => (v && typeof v === 'object' && !Array.isArray(v) ? (v as Obj) : {})
const str = (v: unknown) => (typeof v === 'string' ? v : '')
const list = (v: unknown) => (Array.isArray(v) ? v.map(String) : [])
const arrow = (a: string, b: string) => `${a || 'none'} → ${b || 'none'}`

function actorOf(label: string) {
  if (label === '') return 'Someone'
  if (label === 'cli') return 'Command line'
  if (label.startsWith('agent:')) return `Agent ${label.slice('agent:'.length)}`

  return label
}

function categoryOf(action: string): Described['category'] {
  if (action.startsWith('device.') || action.startsWith('registration.')) return 'devices'
  if (action.startsWith('policy.')) return 'policy'
  if (action.startsWith('apikey.') || action.startsWith('preauthkey.')) return 'keys'
  if (action.startsWith('ssh_')) return 'ssh'
  if (action.startsWith('auth.') || action.startsWith('sudo.')) return 'signin'

  return 'people'
}

/**
 * Turns one audit row into a sentence a person can scan: "ops@x renamed device
 * web-1 to web-2". The row keeps whatever before/after the action stored, so
 * everything here reads defensively and falls back to the raw action.
 */
export function describeEntry(e: AuditEntry): Described {
  const before = obj(e.before)
  const after = obj(e.after)
  // Rows written before labels existed fall back to a name stored in the
  // change itself, then to the number; "0" is the placeholder for "no id".
  const id = e.targetId && e.targetId !== '0' ? `#${e.targetId}` : ''
  const target = e.targetLabel || str(after.host) || str(before.host) || str(after.name) || str(before.name) || id || e.targetType
  const out = (text: string, detail?: string, tone: Described['tone'] = 'default'): Described => ({
    actor: actorOf(e.actorLabel),
    text,
    detail: detail || undefined,
    category: categoryOf(e.action),
    tone,
  })

  switch (e.action) {
    case 'auth.login':
      return out('signed in', str(after.method) === 'oidc' ? 'with the identity provider' : 'with a password')
    case 'auth.login_failed':
      return out('failed to sign in', str(after.reason), 'warn')
    case 'auth.logout':
      return out('signed out')
    case 'sudo.grant':
      return out('turned on sudo')
    case 'sudo.fail': {
      const n = Number(after.failures) || 0

      return out('entered a wrong PIN', n ? `${n} wrong in a row` : undefined, 'warn')
    }
    case 'sudo.locked':
      return out('locked sudo after too many wrong PINs', undefined, 'danger')

    case 'device.rename':
      return out(`renamed device ${str(before.name) || target} to ${str(after.name)}`)
    case 'device.tags':
      return out(`changed tags on ${target}`, arrow(list(e.before).join(', '), list(e.after).join(', ')))
    case 'device.routes':
      return out(`changed approved routes on ${target}`, arrow(list(e.before).join(', '), list(e.after).join(', ')))
    case 'device.expire':
      return out(`expired the key of ${target}`, undefined, 'warn')
    case 'device.delete':
      return out(`removed device ${target}`, undefined, 'danger')
    case 'registration.approve':
      return out('approved a device registration', e.targetLabel ? `for ${e.targetLabel}` : undefined)
    case 'registration.reject':
      return out('rejected a device registration')

    case 'policy.save': {
      const note = str(after.note)
      const lines = `+${list(after.added).length} −${list(after.removed).length} lines`

      return out('saved the access policy', note ? `${lines} · ${note}` : lines)
    }

    case 'account.link':
      return out(`linked ${target} to a Headscale user`)
    case 'account.role':
      return out(`changed the role of ${target}`, arrow(str(before.role), str(after.role)))
    case 'account.admission':
      return str(e.after) === 'rejected' ? out(`rejected ${target}`, undefined, 'warn') : out(`approved ${target}`)
    case 'account.password':
      return out('changed their password')
    case 'account.pin_set':
      return out('set their sudo PIN')
    case 'account.pin_change':
      return out('changed their sudo PIN')
    case 'account.pin_reset':
      return out(`reset the sudo PIN of ${target}`, undefined, 'warn')
    case 'account.password_reset':
      return out(`reset the password of ${target}`, undefined, 'warn')
    case 'user.create':
      return out(`created Headscale user ${target}`)
    case 'user.rename':
      return out(`renamed Headscale user ${str(before.name) || 'a user'} to ${target}`)
    case 'user.delete':
      return out(`deleted Headscale user ${target}`, undefined, 'danger')

    case 'apikey.create':
      return out(`created API key ${target}`, undefined, 'warn')
    case 'apikey.expire':
      return out(`expired API key ${target}`)
    case 'apikey.delete':
      return out(`deleted API key ${target}`)
    case 'preauthkey.expire':
      return out(`expired pre-auth ${target}`)
    case 'preauthkey.revoke_active':
      return out('revoked every active pre-auth key', undefined, 'warn')

    case 'ssh_key.add':
      return out(`added SSH key ${target}`)
    case 'ssh_key.delete':
      return out(`removed SSH key ${target}`)
    case 'ssh_access.set':
      return out(`changed who can log in as ${str(after.unixUser) || 'a unix user'} on ${target}`)
    case 'ssh_access.remove_orphan':
      return out(`removed ${target} from SSH access`, undefined, 'warn')
    case 'ssh_target.approve':
      return out(`approved ${target} for SSH key sync`)
    case 'ssh_target.unbind':
      return out(`unbound ${target}`, undefined, 'warn')
    case 'ssh_target.suspend':
      return out(`suspended ${list(after.servers).join(', ') || 'a server'}`, undefined, 'warn')
    case 'ssh_agent.create':
      return out(`added agent ${target}`)
    case 'ssh_agent.rotate':
      return out(`replaced the token of agent ${target}`)
    case 'ssh_agent.revoke':
      return out(`revoked agent ${target}`, undefined, 'danger')
    case 'ssh_agent.retire':
      return out('finished removing its keys and retired')

    default:
      return out(`${e.action} ${e.targetLabel || (id ? `${e.targetType} ${id}` : e.targetType)}`.trim())
  }
}

/** Today, Yesterday, or the date, in the viewer's own time zone. */
export function dayLabel(iso: string, now = new Date()) {
  const d = new Date(iso)
  const start = (x: Date) => new Date(x.getFullYear(), x.getMonth(), x.getDate()).getTime()
  const days = Math.round((start(now) - start(d)) / 86_400_000)

  if (days === 0) return 'Today'
  if (days === 1) return 'Yesterday'

  return d.toLocaleDateString(undefined, { weekday: 'short', day: 'numeric', month: 'short', year: d.getFullYear() === now.getFullYear() ? undefined : 'numeric' })
}

const isRecord = (v: unknown): v is Obj => !!v && typeof v === 'object' && !Array.isArray(v)

function flat(v: unknown): string {
  if (v === undefined || v === null) return ''
  if (Array.isArray(v)) return v.map(flat).join(', ')
  if (typeof v === 'object') return JSON.stringify(v)

  return String(v)
}

/**
 * The fields that differ between a before and after record, in key order.
 * Empty when either side is not a record: a create, a delete, or a plain list.
 */
export function changedFields(before: unknown, after: unknown): { key: string; from: string; to: string }[] {
  if (!isRecord(before) || !isRecord(after)) return []

  const keys = [...new Set([...Object.keys(before), ...Object.keys(after)])].sort()

  return keys
    .map((key) => ({ key, from: flat(before[key]), to: flat(after[key]) }))
    .filter((c) => c.from !== c.to)
}
