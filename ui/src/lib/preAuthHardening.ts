import type { PreAuthKey } from './api'

export function preAuthHardening(keys: PreAuthKey[], now = new Date()) {
  const active = keys.filter((key) => {
    if (!key.expiry) return true
    const expiry = Date.parse(key.expiry)
    return !Number.isFinite(expiry) || expiry > now.getTime()
  })
  return { active, compliant: active.length === 0 }
}
