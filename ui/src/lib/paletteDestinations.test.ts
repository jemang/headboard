import { describe, expect, it } from 'vitest'
import { paletteDestinations } from './paletteDestinations'

describe('paletteDestinations', () => {
  it('offers every page the sidebar offers an owner', () => {
    const labels = paletteDestinations(['view:all', 'manage:policy', 'manage:users', 'manage:ssh', 'manage:devices']).map((d) => d.label)
    expect(labels).toEqual(['Devices', 'Access control', 'People', 'SSH access', 'Keys', 'Help'])
  })

  it('hides pages a member cannot use', () => {
    expect(paletteDestinations(['view:self']).map((d) => d.label)).toEqual(['My devices', 'Keys'])
  })
})
