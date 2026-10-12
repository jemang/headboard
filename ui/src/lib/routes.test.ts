import { describe, expect, it } from 'vitest'
import { navItems, parseLocation, resolveRoute } from './routes'

const owner = ['view:self', 'view:all', 'manage:devices', 'manage:policy', 'manage:users', 'manage:keys', 'view:audit', 'manage:owners', 'manage:ssh']
const netAdmin = ['view:self', 'view:all', 'manage:devices', 'manage:policy', 'view:audit']
const member = ['view:self']
const auditor = ['view:self', 'view:all', 'view:audit']

describe('parseLocation', () => {
  it('splits path, query and hash', () => {
    const l = parseLocation('/devices?device=3&section=routing#needs')
    expect(l.path).toBe('/devices')
    expect(l.search.get('device')).toBe('3')
    expect(l.search.get('section')).toBe('routing')
    expect(l.hash).toBe('needs')
  })

  it('defaults an empty path to /', () => {
    expect(parseLocation('').path).toBe('/')
  })
})

describe('resolveRoute', () => {
  it('sends a signed-in person on /login to devices', () => {
    expect(resolveRoute('/login', owner)).toEqual({ redirect: '/devices' })
  })

  it('sends an unknown path to devices', () => {
    expect(resolveRoute('/nope', owner)).toEqual({ redirect: '/devices' })
  })

  it('renders devices at / and /devices', () => {
    expect(resolveRoute('/', member)).toEqual({ route: 'devices' })
    expect(resolveRoute('/devices', member)).toEqual({ route: 'devices' })
  })

  it('redirects a page the role cannot open', () => {
    expect(resolveRoute('/people', netAdmin)).toEqual({ redirect: '/devices' })
    expect(resolveRoute('/acl', member)).toEqual({ redirect: '/devices' })
  })

  it('opens Activity for anyone who may read the audit log, and no one else', () => {
    expect(resolveRoute('/activity', auditor)).toEqual({ route: 'activity' })
    expect(resolveRoute('/activity', member)).toEqual({ redirect: '/devices' })
  })

  it('always allows keys and account', () => {
    expect(resolveRoute('/keys', member)).toEqual({ route: 'keys' })
    expect(resolveRoute('/account', member)).toEqual({ route: 'account' })
  })
})

describe('navItems', () => {
  it('gives an owner four primary tabs plus more', () => {
    const n = navItems(owner)
    expect(n.primary.map((i) => i.short)).toEqual(['Devices', 'Access', 'SSH', 'People'])
    expect(n.more.map((i) => i.id)).toEqual(['keys', 'activity', 'help', 'account'])
  })

  it('gives a network admin devices and access', () => {
    const n = navItems(netAdmin)
    expect(n.primary.map((i) => i.short)).toEqual(['Devices', 'Access'])
    expect(n.more.map((i) => i.id)).toEqual(['keys', 'activity', 'help', 'account'])
  })

  it('gives a member devices only', () => {
    expect(navItems(member).primary.map((i) => i.short)).toEqual(['Devices'])
    expect(navItems(member).more.map((i) => i.id)).toEqual(['keys', 'account'])
  })

  it('keeps the desktop rail order', () => {
    expect(navItems(owner).all.map((i) => i.id)).toEqual(['devices', 'acl', 'people', 'ssh', 'keys', 'activity', 'help', 'account'])
  })
})
