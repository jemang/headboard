const exitRoutes = ['0.0.0.0/0', '::/0'] as const

type ExitState = 'none' | 'incomplete' | 'pending' | 'approved'

export type RouteSummary = {
  exit: { state: ExitState; routes: string[] }
  subnets: Array<{ route: string; approved: boolean }>
}

// routeSummary mirrors Headscale's exit-node rule: both default routes must be
// advertised and approved. All other advertised prefixes are subnet routes.
export function routeSummary(advertised: string[], approved: string[]): RouteSummary {
  const advertisedExit = exitRoutes.filter((route) => advertised.includes(route))
  const exit = {
    state: exitState(advertisedExit, approved),
    routes: advertisedExit,
  }

  return {
    exit,
    subnets: advertised
      .filter((route) => !exitRoutes.includes(route as (typeof exitRoutes)[number]))
      .map((route) => ({ route, approved: approved.includes(route) })),
  }
}

// approveRoutes and revokeRoutes produce the complete replacement set required
// by Headscale's route endpoint, preserving unrelated approved routes.
export function approveRoutes(current: string[], routes: string[]): string[] {
  return [...new Set([...current, ...routes])]
}

export function revokeRoutes(current: string[], routes: string[]): string[] {
  const revoked = new Set(routes)
  return current.filter((route) => !revoked.has(route))
}

function exitState(advertised: string[], approved: string[]): ExitState {
  if (advertised.length === 0) return 'none'
  if (advertised.length !== exitRoutes.length) return 'incomplete'
  if (advertised.every((route) => approved.includes(route))) return 'approved'
  return 'pending'
}
