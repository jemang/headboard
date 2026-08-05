import type { TailnetUser } from './api'

/**
 * Headscale allows two users to share a name once one has an OIDC provider
 * identifier — most often a CLI-seeded user and a self-registered device
 * owner picking the same username. Flag it: policy tokens like "name@" become
 * ambiguous and Headscale refuses to compile the policy while both exist.
 */
export function duplicateUserNames(users: TailnetUser[]): Set<string> {
  const counts = new Map<string, number>()

  for (const u of users) {
    const key = u.name.toLowerCase()
    counts.set(key, (counts.get(key) ?? 0) + 1)
  }

  return new Set([...counts].filter(([, count]) => count > 1).map(([name]) => name))
}
