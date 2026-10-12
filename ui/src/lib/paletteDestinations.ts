/** The pages the palette can jump to, gated by the same capabilities as the
 *  sidebar in App.tsx so the two never disagree about what a role can open. */
export function paletteDestinations(capabilities: string[]) {
  const can = (c: string) => capabilities.includes(c)

  return [
    { id: 'devices', label: can('view:all') ? 'Devices' : 'My devices', short: 'Devices', to: '/devices', show: true },
    { id: 'acl', label: 'Access control', short: 'Access', to: '/acl', show: can('manage:policy') },
    { id: 'people', label: 'People', short: 'People', to: '/people', show: can('manage:users') },
    { id: 'ssh', label: 'SSH access', short: 'SSH', to: '/ssh', show: can('manage:ssh') },
    { id: 'keys', label: 'Keys', short: 'Keys', to: '/keys', show: true },
    { id: 'help', label: 'Help', short: 'Help', to: '/help', show: can('manage:devices') },
  ]
    .filter((d) => d.show)
    .map(({ id, label, short, to }) => ({ id, label, short, to }))
}
