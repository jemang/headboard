import { useQuery } from '@tanstack/react-query'
import { api, type Me } from './api'
import { deriveNeeds, type NeedItem } from './needs'

/**
 * Called at the top of Shell, before its early returns, so `me` may still be
 * loading; nothing is fetched until the account is active. Accounts and SSH
 * status are not pushed by the SSE stream, so they poll.
 */
export function useNeeds(me: Me | undefined): { items: NeedItem[]; health: ReturnType<typeof useHealth> } {
  const active = me?.user.admission === 'active'
  const can = (c: string) => active && Boolean(me?.capabilities.includes(c))
  const health = useHealth(active)
  const devices = useQuery({ queryKey: ['devices'], queryFn: api.devices, enabled: can('view:all') })
  const accounts = useQuery({ queryKey: ['accounts'], queryFn: api.accounts, enabled: can('manage:users'), refetchInterval: 15000 })
  const ssh = useQuery({ queryKey: ['ssh-access'], queryFn: api.sshAccess, enabled: can('manage:ssh'), refetchInterval: 15000 })

  const items = deriveNeeds({
    capabilities: active ? me?.capabilities ?? [] : [],
    health: health.data,
    healthUnavailable: health.isError,
    devices: devices.data?.devices,
    accounts: accounts.data?.accounts,
    ssh: ssh.data,
  })

  return { items, health }
}

function useHealth(enabled: boolean) {
  return useQuery({ queryKey: ['health'], queryFn: api.health, refetchInterval: 5000, enabled })
}
