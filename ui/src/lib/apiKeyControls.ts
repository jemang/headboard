export function expiryFromDays(days: string): string | undefined {
  const value = Number(days)

  if (!Number.isSafeInteger(value) || value <= 0) return undefined

  return `${value * 24}h`
}

export function isExpired(expiration?: string): boolean {
  if (!expiration) return false

  const time = Date.parse(expiration)

  return Number.isFinite(time) && time <= Date.now()
}
