/** The pages the palette can jump to, gated by the same capabilities as the
 *  sidebar in App.tsx so the two never disagree about what a role can open. */
export function paletteDestinations(capabilities: string[]) {
  const can = (c: string) => capabilities.includes(c)

  return [
    { id: 'devices', label: can('view:all') ? 'Devices' : 'My devices', to: '/devices', show: true },
    { id: 'acl', label: 'Access control', to: '/acl', show: can('manage:policy') },
    { id: 'people', label: 'People', to: '/people', show: can('manage:users') },
    { id: 'ssh', label: 'SSH access', to: '/ssh', show: can('manage:ssh') },
    { id: 'keys', label: 'Keys', to: '/keys', show: true },
    { id: 'help', label: 'Help', to: '/help', show: can('manage:devices') },
  ]
    .filter((d) => d.show)
    .map(({ id, label, to }) => ({ id, label, to }))
}
