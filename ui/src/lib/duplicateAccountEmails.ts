/**
 * Accounts are keyed on issuer + subject, so signing in through a different
 * issuer address (localhost vs a LAN IP, or a changed OIDC URL) creates a
 * second account for the same person. Flag it so an admin can remove the stray.
 */
export function duplicateAccountEmails(accounts: { email: string }[]): Set<string> {
  const counts = new Map<string, number>()

  for (const a of accounts) {
    // A blank email says nothing about who the account belongs to.
    if (a.email.trim() === '') continue
    const key = a.email.toLowerCase()
    counts.set(key, (counts.get(key) ?? 0) + 1)
  }

  return new Set([...counts].filter(([, count]) => count > 1).map(([email]) => email))
}
