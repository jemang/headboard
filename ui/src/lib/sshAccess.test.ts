import { describe, expect, it, vi } from 'vitest'
import { sshAgentInstallCommand, sshNewMachineCommand, tokenFor, userTokens, visibleRows, insecureInstallUrl, revokeRefusal, saveCleanup, loadCleanup, sshAgentOneLiner, withRow, idsFromTokens, pickableTokens, serverIssues, serverMatches, groupByAgent } from './sshAccess'
import type { SshAccess as SshAccessData, SshServer } from './api'
import { ApiError } from './api'

describe('userTokens', () => {
  it('uses plain names unless two users share one', () => {
    const t = userTokens([
      { id: 1, name: 'alice' },
      { id: 2, name: 'sam' },
      { id: 3, name: 'Sam' },
    ])
    expect([...t.entries()]).toEqual([
      ['alice', 1],
      ['sam#2', 2],
      ['Sam#3', 3],
    ])
    expect(tokenFor(t, 3)).toBe('Sam#3')
    expect(tokenFor(t, 99)).toBe('#99')
  })
})

describe('install commands', () => {
  it('downloads over https only, then runs with the URL as an argument', () => {
    expect(sshAgentInstallCommand('https://hb.example/manage/')).toBe(
      "f=$(mktemp) && curl -fsSL --proto '=https' 'https://hb.example/manage/api/ssh/agent/install.sh' -o \"$f\" && sh \"$f\" 'https://hb.example/manage'",
    )
  })

  it('joins the tailnet first for a new machine, without a pre-auth key', () => {
    const cmd = sshNewMachineCommand('https://hs.example', 'https://hb.example')
    expect(cmd.startsWith("tailscale up --login-server='https://hs.example' && f=$(mktemp) && curl")).toBe(true)
    expect(cmd).not.toContain('authkey')
    expect(cmd).not.toContain('sudo')
  })
})

describe('visibleRows', () => {
  it('shows a newly added unix user until the server has a row for it', () => {
    const saved = [{ unixUser: 'deploy', users: [{ id: 1, name: 'alice', keys: 1, missing: false }], warnings: [] }]

    expect(visibleRows(saved, ['staging', 'deploy']).map((r) => r.unixUser)).toEqual(['deploy', 'staging'])
    expect(visibleRows(saved, ['staging'])[1]).toEqual({ unixUser: 'staging', users: [], warnings: [], pending: true })
    expect(visibleRows(saved, [])[0].pending).toBeUndefined()
  })
})

describe('insecureInstallUrl', () => {
  it('flags a Headboard address the installer will refuse', () => {
    expect(insecureInstallUrl('http://192.0.2.1:3000')).toBe(true)
    expect(insecureInstallUrl('https://hb.example')).toBe(false)
  })
})

describe('revokeRefusal', () => {
  it('returns the server reason only for a 409', () => {
    expect(revokeRefusal(new ApiError(409, 'unbind first'))).toBe('unbind first')
    expect(revokeRefusal(new ApiError(500, 'boom'))).toBeNull()
    expect(revokeRefusal(new Error('x'))).toBeNull()
  })
})

describe('cleanup commands storage', () => {
  it('survives a remount and clears on dismiss', () => {
    saveCleanup(['sed a'])
    expect(loadCleanup()).toEqual(['sed a'])
    saveCleanup([])
    expect(loadCleanup()).toEqual([])
  })
})

describe('sshAgentOneLiner', () => {
  // Debian's /root/.bashrc does not set HISTCONTROL, so a leading space alone
  // leaves the token in /root/.bash_history on a stock Proxmox host.
  it('stops the shell saving its history before the token appears', () => {
    expect(sshAgentOneLiner('https://hb.example', 'hbk_abc')).toBe(
      " unset HISTFILE; f=$(mktemp) && curl -fsSL --proto '=https' 'https://hb.example/api/ssh/agent/install.sh' -o \"$f\" && HEADBOARD_AGENT_TOKEN='hbk_abc' sh \"$f\" 'https://hb.example'",
    )
  })
})

describe('withRow', () => {
  const access: SshAccessData = {
    servers: [
      { name: 'app-1', ip: '192.0.2.10', suggestions: [], pendingClears: [], rows: [
        { unixUser: 'deploy', users: [{ id: 1, name: 'alice', keys: 2, missing: false }], warnings: ['w'] },
      ] },
      { name: 'web', ip: '192.0.2.11', suggestions: [], pendingClears: [], rows: [] },
    ],
    orphans: [],
  }
  const people = [{ id: 1, name: 'alice' }, { id: 2, name: 'bob' }]

  // The selector builds each save from this cache; until the server answers,
  // a second pick must see the first one or it silently drops it.
  it('adds people to an existing row, keeping what is known about them', () => {
    const next = withRow(access, 'app-1', 'deploy', [1, 2], people)
    expect(next.servers[0].rows[0].users).toEqual([
      { id: 1, name: 'alice', keys: 2, missing: false },
      { id: 2, name: 'bob', keys: 0, missing: false },
    ])
    expect(next.servers[0].rows[0].warnings).toEqual(['w'])
    expect(access.servers[0].rows[0].users).toHaveLength(1)
  })

  it('creates a row for a new unix user and drops a row emptied of people', () => {
    expect(withRow(access, 'web', 'ops', [2], people).servers[1].rows).toEqual([
      { unixUser: 'ops', users: [{ id: 2, name: 'bob', keys: 0, missing: false }], warnings: [] },
    ])
    expect(withRow(access, 'app-1', 'deploy', [], people).servers[0].rows).toEqual([])
  })
})

describe('loadCleanup', () => {
  // sessionStorage outlives deploys; a value an older build or a hand edit
  // left there must not take the page down.
  it('ignores a stored value that is not a list of strings', () => {
    for (const raw of ['{"a":1}', '[1,2]', 'not json', 'null']) {
      vi.stubGlobal('sessionStorage', { getItem: () => raw, setItem: () => {}, removeItem: () => {} })
      expect(loadCleanup()).toEqual([])
    }
    vi.unstubAllGlobals()
  })
})

describe('idsFromTokens', () => {
  // A person deleted from Headscale shows as a "#9" chip; editing the row must
  // keep them until someone removes the chip, not drop them silently.
  it('keeps unresolved #id chips', () => {
    const tokens = new Map([['alice', 1]])
    expect(idsFromTokens(tokens, ['alice', '#9', 'nobody'])).toEqual([1, 9])
  })
})

describe('pickableTokens', () => {
  const users = [
    { id: 1, name: 'alice', keys: 2 },
    { id: 2, name: 'bob', keys: 0 },
    { id: 3, name: 'carol', keys: 1 },
  ]

  it('offers only people who hold an SSH key', () => {
    expect(pickableTokens(userTokens(users), users)).toEqual(['alice', 'carol'])
  })

  it('treats an unknown key count as able to log in', () => {
    expect(pickableTokens(userTokens([{ id: 4, name: 'dave' }]), [{ id: 4, name: 'dave' }])).toEqual(['dave'])
  })
})

describe('server finding', () => {
  const srv = (name: string, extra: Partial<SshServer> = {}): SshServer => ({ name, ip: '10.0.0.1', suggestions: [], rows: [], pendingClears: [], ...extra })
  const bound = (agentName: string, state: 'active' | 'suspended' = 'active') => ({ agentId: 1, agentName, targetRef: '150', approvedIp: '10.0.0.1', approvedLabel: '', state, reapprovable: false })

  it('counts a binding to approve, agent warnings and people who cannot log in', () => {
    expect(serverIssues(srv('a', { binding: bound('pve') }))).toBe(0)
    // Nothing reported this IP yet: nothing to do on this page.
    expect(serverIssues(srv('a'))).toBe(0)
    expect(serverIssues(srv('a', { suggestions: [{ agentId: 1, agentName: 'pve', targetRef: '150', hostname: 'a', ambiguous: false }] }))).toBe(1)
    expect(
      serverIssues(srv('a', {
        binding: bound('pve', 'suspended'),
        rows: [{ unixUser: 'deploy', warnings: ['unix user does not exist'], users: [{ id: 1, name: 'al', keys: 0, missing: false }] }],
      })),
    ).toBe(3)
  })

  it('finds a server by unix user, person, agent or CT and applies the filter', () => {
    const s = srv('etempahan-mdp', { binding: bound('sgp-ns563693'), rows: [{ unixUser: 'pentadbir', warnings: [], users: [{ id: 1, name: 'azman-mini', keys: 1, missing: false }] }] })
    expect(serverMatches(s, 'PENTAD', 'all')).toBe(true)
    expect(serverMatches(s, 'azman', 'all')).toBe(true)
    expect(serverMatches(s, 'ct 150', 'all')).toBe(true)
    expect(serverMatches(s, 'berapay', 'all')).toBe(false)
    expect(serverMatches(s, '', 'attention')).toBe(false)
    expect(serverMatches(s, '', 'unbound')).toBe(false)
    expect(serverMatches(srv('x'), '', 'unbound')).toBe(true)
    expect(serverMatches(srv('x'), '', 'attention')).toBe(false)
  })

  it('groups by agent, sorted, with unbound servers last', () => {
    const groups = groupByAgent([srv('z', { binding: bound('pve2') }), srv('loose'), srv('b', { binding: bound('pve1') }), srv('a', { binding: bound('pve2') })])
    expect(groups.map((g) => [g.agent, g.servers.map((s) => s.name)])).toEqual([
      ['pve1', ['b']],
      ['pve2', ['a', 'z']],
      [null, ['loose']],
    ])
  })
})
