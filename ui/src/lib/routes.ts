import { paletteDestinations } from './paletteDestinations'

export interface AppLocation {
  path: string
  search: URLSearchParams
  hash: string
}

export type RouteId = 'devices' | 'acl' | 'people' | 'ssh' | 'keys' | 'help' | 'account'

export interface NavItem {
  id: RouteId
  label: string
  short: string
  to: string
}

/** Parses an app-absolute href (base path already stripped). */
export function parseLocation(appHref: string): AppLocation {
  const url = new URL(appHref || '/', 'http://app.invalid')

  return { path: url.pathname, search: url.searchParams, hash: url.hash.replace(/^#/, '') }
}

/**
 * The pages a role may open, in rail order. Built on paletteDestinations so
 * the rail, the bottom bar and the palette can never disagree.
 */
function allowed(capabilities: string[]): NavItem[] {
  const pages = paletteDestinations(capabilities).map((d) => ({ id: d.id as RouteId, label: d.label, short: d.short, to: d.to }))

  return [...pages, { id: 'account', label: 'Account', short: 'Account', to: '/account' }]
}

export function resolveRoute(path: string, capabilities: string[]): { route: RouteId } | { redirect: string } {
  if (path === '/' || path === '/devices') return { route: 'devices' }

  const hit = allowed(capabilities).find((i) => i.to === path)

  return hit ? { route: hit.id } : { redirect: '/devices' }
}

const PRIMARY: RouteId[] = ['devices', 'acl', 'ssh', 'people']

export function navItems(capabilities: string[]) {
  const all = allowed(capabilities)

  return {
    all,
    primary: PRIMARY.flatMap((id) => all.filter((i) => i.id === id)),
    more: all.filter((i) => !PRIMARY.includes(i.id)),
  }
}
