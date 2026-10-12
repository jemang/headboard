import { CircleHelp, KeyRound, KeySquare, Laptop, ShieldCheck, UserRound, Users, type LucideIcon } from 'lucide-react'
import type { RouteId } from '../../lib/routes'

/** One icon per page, shared by the rail, the bottom bar and the More sheet. */
export const ICONS: Record<RouteId, LucideIcon> = {
  devices: Laptop,
  acl: ShieldCheck,
  people: Users,
  ssh: KeySquare,
  keys: KeyRound,
  help: CircleHelp,
  account: UserRound,
}
