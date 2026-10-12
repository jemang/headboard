import { ApiError, type SshAccess, type SshServer } from './api'
import { withBase } from './basePath'
import { serverRegistrationCommand, shellArg } from './serverRegistrationCommand'

/** Chip labels for Headscale users. Names can collide (Headscale's own OIDC
 *  registration creates duplicates), so a shared name gets its id appended. */
export function userTokens(users: { id: number; name: string }[]) {
  const counts = new Map<string, number>()
  for (const u of users) counts.set(u.name.toLowerCase(), (counts.get(u.name.toLowerCase()) ?? 0) + 1)

  const byToken = new Map<string, number>()
  for (const u of users) {
    byToken.set((counts.get(u.name.toLowerCase()) ?? 0) > 1 ? `${u.name}#${u.id}` : u.name, u.id)
  }

  return byToken
}

export function tokenFor(byToken: Map<string, number>, id: number) {
  for (const [token, value] of byToken) if (value === id) return token

  return `#${id}`
}

/** Chip labels back to user ids. A "#9" chip is a user Headscale no longer
 *  has; it is kept, not dropped, so editing a row never removes someone the
 *  admin did not remove. */
export function idsFromTokens(byToken: Map<string, number>, tokens: string[]): number[] {
  const ids: number[] = []
  for (const t of tokens) {
    const id = byToken.get(t) ?? (/^#(\d+)$/.test(t) ? Number(t.slice(1)) : undefined)
    if (id !== undefined) ids.push(id)
  }

  return ids
}

export function headboardUrl() {
  return new URL(withBase('/'), window.location.origin).href.replace(/\/+$/, '')
}

export function sshAgentInstallCommand(url: string) {
  const base = url.replace(/\/+$/, '')

  // A fresh mktemp file rather than a fixed /tmp name, which another local user
  // could create first and swap before root runs it.
  return [
    'f=$(mktemp)',
    `curl -fsSL --proto '=https' ${shellArg(`${base}/api/ssh/agent/install.sh`)} -o "$f"`,
    `sh "$f" ${shellArg(base)}`,
  ].join(' && ')
}

/** One paste on the host: the token rides in the environment rather than argv.
 *  Debian's /root/.bashrc sets no HISTCONTROL, so a leading space alone is not
 *  enough; unsetting HISTFILE stops this shell writing any history on exit.
 *  The space still helps where ignorespace is set. */
export function sshAgentOneLiner(url: string, token: string) {
  const base = url.replace(/\/+$/, '')

  return ` unset HISTFILE; f=$(mktemp) && curl -fsSL --proto '=https' ${shellArg(`${base}/api/ssh/agent/install.sh`)} -o "$f" && HEADBOARD_AGENT_TOKEN=${shellArg(token)} sh "$f" ${shellArg(base)}`
}

/** Pre-auth keys are disabled, so `tailscale up` waits for an admin to approve
 *  the registration before the installer runs. */
export function sshNewMachineCommand(headscaleUrl: string, url: string) {
  return `${serverRegistrationCommand(headscaleUrl, 'standard', '')} && ${sshAgentInstallCommand(url)}`
}

type Row = SshServer['rows'][number] & { pending?: boolean }

/** A row exists on the server only while someone is assigned to it, so a unix
 *  user just added in the UI is held locally until its first person is picked. */
export function visibleRows(saved: Row[], pending: string[]): Row[] {
  const extra = pending
    .filter((u) => !saved.some((r) => r.unixUser === u))
    .map((unixUser) => ({ unixUser, users: [], warnings: [], pending: true }))

  return [...saved, ...extra]
}

/** The installer and agent refuse anything but https, so a command built from
 *  an http address would fail on the server; the page says so up front. */
export function insecureInstallUrl(url: string) {
  return !url.startsWith('https://')
}

/** The server refuses a plain revoke while an agent still has bindings or
 *  pending clears; that refusal is what earns the Force button. */
export function revokeRefusal(err: unknown): string | null {
  return err instanceof ApiError && err.status === 409 ? err.message : null
}

const cleanupKey = 'headboard.ssh.cleanup'
let cleanupFallback: string[] = []

/** Cleanup commands from a forced revoke are the only record of which keys
 *  were left behind, so they outlive navigation until the admin dismisses them. */
export function saveCleanup(cmds: string[]) {
  cleanupFallback = cmds
  try {
    if (cmds.length === 0) sessionStorage.removeItem(cleanupKey)
    else sessionStorage.setItem(cleanupKey, JSON.stringify(cmds))
  } catch {
    // Storage can be unavailable; the in-memory copy still covers this page.
  }
}

export function loadCleanup(): string[] {
  try {
    const raw = sessionStorage.getItem(cleanupKey)
    if (raw) {
      const v: unknown = JSON.parse(raw)
      return Array.isArray(v) && v.every((c) => typeof c === 'string') ? v : []
    }
  } catch {
    // Fall through to the in-memory copy.
  }

  return cleanupFallback
}

/** The access data as it will be once a row save lands. Applied to the query
 *  cache before the request, so a second pick in the same row builds on the
 *  first instead of on the copy the server last sent. */
export function withRow(access: SshAccess, host: string, unixUser: string, ids: number[], people: { id: number; name: string }[]): SshAccess {
  return {
    ...access,
    servers: access.servers.map((s) => {
      if (s.name !== host) return s

      const old = s.rows.find((r) => r.unixUser === unixUser)
      const users = ids.map(
        (id) => old?.users.find((u) => u.id === id) ?? { id, name: people.find((p) => p.id === id)?.name ?? `#${id}`, keys: 0, missing: false },
      )
      const rows = old
        ? s.rows.map((r) => (r.unixUser === unixUser ? { ...r, users } : r))
        : [...s.rows, { unixUser, users, warnings: [] }]

      return { ...s, rows: rows.filter((r) => r.users.length > 0) }
    }),
  }
}
