/**
 * Thin client for Headboard's own API.
 *
 * The browser never talks to Headscale directly — the admin API key lives only
 * in the Go process — so this is the single network surface the UI has.
 */

import { withBase } from './basePath'

export class ApiError extends Error {
  readonly status: number
  /** The server's machine-readable reason, e.g. "sudo_required". */
  readonly code?: string

  constructor(status: number, message: string, code?: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
  }
}

let sudoPrompt: (() => Promise<boolean>) | null = null

/**
 * A sudo_required that reaches a caller means the person saw the PIN prompt
 * and cancelled it. That is their choice, not a failure to report.
 */
export function isCancelledSudo(err: unknown) {
  return err instanceof ApiError && err.code === 'sudo_required'
}

/**
 * Registers the PIN prompt. A request the server refuses with
 * "sudo_required" opens it, and is sent once more if the PIN was accepted, so
 * no caller has to know which actions are dangerous.
 */
export function setSudoPrompt(prompt: (() => Promise<boolean>) | null) {
  sudoPrompt = prompt
}

/** The request never reached the server: offline, DNS, a dropped connection. */
export class NetworkError extends Error {
  constructor() {
    super('Could not reach Headboard')
    this.name = 'NetworkError'
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  try {
    return await requestOnce<T>(path, init)
  } catch (err) {
    if (!(err instanceof ApiError) || err.code !== 'sudo_required' || !sudoPrompt) throw err
    if (!(await sudoPrompt())) throw err

    return requestOnce<T>(path, init)
  }
}

async function requestOnce<T>(path: string, init?: RequestInit): Promise<T> {
  // Every call goes through here, so prefixing once is what lets the rest of
  // the client keep writing "/api/devices" while the app is served at /manage.
  // fetch rejects only when the request never got an answer; naming that apart
  // from a TypeError in our own code is what the offline screen keys on.
  const res = await fetch(withBase(path), {
    credentials: 'same-origin',
    headers: { Accept: 'application/json', ...init?.headers },
    ...init,
  }).catch(() => {
    throw new NetworkError()
  })

  if (!res.ok) {
    // Huma reports failures as RFC 9457 problem+json; fall back to the status
    // line when the body is not JSON (a proxy error page, say).
    const body = await res
      .json()
      .then((b: { detail?: string; title?: string; code?: string }) => b)
      .catch(() => undefined)

    throw new ApiError(res.status, body?.detail ?? body?.title ?? `${res.status} ${res.statusText}`, body?.code)
  }

  if (res.status === 204) return undefined as T

  return res.json() as Promise<T>
}

function send<T>(method: string, path: string, body?: unknown): Promise<T> {
  return request<T>(path, {
    method,
    headers: body === undefined ? {} : { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
}

// ----------------------------------------------------------------- types ---

/** One recorded change. before/after are whatever the action stored. */
export interface AuditEntry {
  id: number
  actorUserId?: number
  actorLabel: string
  action: string
  targetType: string
  targetId?: string
  targetLabel?: string
  before?: unknown
  after?: unknown
  ip?: string
  createdAt: string
}

export type AuditCategory = '' | 'devices' | 'policy' | 'people' | 'keys' | 'ssh' | 'signin'

export interface Health {
  status: string
  version: string
  headscaleVersion: string
  headscaleServerVersion: string
  headscaleVersionMatch: boolean
  headscaleState: 'connected' | 'stale' | 'unavailable'
  headscaleLastSynced?: string
}

export type Role = 'owner' | 'admin' | 'network-admin' | 'auditor' | 'member'
export type AdmissionState = 'active' | 'pending' | 'rejected'

export interface Account {
  id: number
  role: Role
  admission: AdmissionState
  email: string
  displayName: string
  avatarUrl?: string
  headscaleUserId?: number
  createdAt: string
  lastLoginAt?: string
}

export interface Me {
  local: boolean
  user: Account
  capabilities: string[]
  linked: boolean
  admission: AdmissionState
  /** The role needs a sudo PIN and this account has none: hold at setup. */
  needsPin: boolean
  pinSet: boolean
  /** When sudo ends; absent when it is off. */
  sudoUntil?: string
  /** Signed in again at the identity provider within the last five minutes. */
  reauthFresh: boolean
}

export interface AuthStatus {
  localEnabled: boolean
  authenticated: boolean
  oidcEnabled: boolean
  issuer?: string
  loginUrl?: string
}

export interface Device {
  id: number
  name: string
  hostname: string
  ips: string[]
  tags?: string[]
  owner?: string
  ownerId?: number
  online: boolean
  expired: boolean
  ephemeral: boolean
  lastSeen?: string
  expiry?: string
  advertisedRoutes?: string[]
  approvedRoutes?: string[]
  subnetRoutes?: string[]
  exitNode: boolean
  mine: boolean
}

export interface Endpoint {
  raw: string
  label: string
  kind: 'node' | 'nodes' | 'internet' | 'any' | 'prefix'
  nodeIds?: number[]
  owner?: string
}

export type Dest = Endpoint & { ports: string }

export interface EffectiveRule {
  protocols?: number[]
  sources: Endpoint[]
  dests: Dest[]
}

export interface Peer {
  id: number
  givenName: string
  owner?: string
  ips: string[]
  online: boolean
}

export interface DeviceRules {
  inbound: EffectiveRule[]
  outbound: EffectiveRule[]
  peers: Peer[]
}

export interface TailnetUser {
  id: number
  name: string
  displayName?: string
  email?: string
  providerId?: string
  devices: number
  linkedAccountId?: number
}

export interface AclRule {
  action: string
  src: string[]
  dst: string[]
  proto?: string
}

export interface Grant {
  src: string[]
  dst: string[]
  ip?: string[]
  app?: unknown
  via?: string[]
}

export interface SshRule {
  action: string
  src: string[]
  dst: string[]
  users: string[]
  checkPeriod?: string
  acceptEnv?: string[]
}

export interface AclSchema {
  groups?: Record<string, string[]>
  hosts?: Record<string, string>
  tagOwners?: Record<string, string[]>
  acls?: AclRule[]
  grants?: Grant[]
  ssh?: SshRule[]
  autoApprovers?: {
    routes?: Record<string, string[]>
    exitNode?: string[]
  }
  tests?: AclTest[]
  sshTests?: SshTest[]
}

export interface AclTest {
  src: string
  proto?: string
  accept?: string[]
  deny?: string[]
}

export interface SshTest {
  src: string
  dst: string[]
  accept?: string[]
  deny?: string[]
  check?: string[]
}

export interface Tokens {
  users: string[]
  groups: string[]
  tags: string[]
  hosts: string[]
  autogroups: string[]
}

export interface Policy {
  hujson: string
  sha256: string
  sections: string[]
  schema?: AclSchema
  tokens: Tokens
  editable: boolean
  parseError?: string
}

export interface PatchOp {
  op: 'add' | 'remove' | 'replace' | 'move' | 'copy' | 'test'
  path: string
  from?: string
  value?: unknown
}

export interface DiffHunk {
  oldStart: number
  oldLines: number
  newStart: number
  newLines: number
  lines: string[]
}

export interface PolicyPreview {
  hujson: string
  diff: { hunks: DiffHunk[]; added: number; removed: number; identical: boolean }
  valid: boolean
  error?: string
}

/** One claim a tests-block entry makes, evaluated on its own. */
export interface Assertion {
  section: 'tests' | 'sshTests'
  index: number
  pointer: string
  kind: 'accept' | 'deny' | 'check'
  src: string
  dst: string
  proto?: string
  user?: string
  passed: boolean
  error?: string
}

export interface TestRun {
  ran: boolean
  allPassed: boolean
  assertions: Assertion[]
}

export interface Attribution {
  section: string
  index: number
  pointer: string
}

export interface Simulation {
  source: Endpoint
  dest: Endpoint
  port: number
  allowed: boolean
  literalDestination?: boolean
  because?: Attribution
  rule?: EffectiveRule
}

/** A pending change, so tests and the simulator can answer for a draft. */
export type PolicyDraft = { sha256?: string; ops?: PatchOp[]; hujson?: string }

export type SimulationRequest =
  | ({ src: number; dst: number; destinationIP?: never; port: number } & PolicyDraft)
  | ({ src: number; destinationIP: string; dst?: never; port: number } & PolicyDraft)

export interface PolicyRevision {
  id: number
  sha256: string
  body?: string
  authorUserId?: number
  note?: string
  createdAt: string
}

export interface PreAuthKey {
  id: string
  key?: string
  user?: string
  userId?: number
  reusable: boolean
  ephemeral: boolean
  used: boolean
  tags?: string[]
  createdAt?: string
  expiry?: string
}

export interface ApiKey {
  id: string
  prefix: string
  protected?: boolean
  expiration?: string
  createdAt?: string
  lastSeen?: string
}

export interface SshKey {
  id: number
  headscaleUserId: number
  type: string
  fingerprint: string
  label: string
  createdAt: string
}

export type SshBindingState = 'active' | 'suspended' | 'orphan'

export interface SshServer {
  name: string
  ip: string
  binding?: {
    agentId: number
    agentName: string
    targetRef: string
    approvedIp: string
    approvedLabel: string
    state: SshBindingState
    reapprovable: boolean
  }
  suggestions: { agentId: number; agentName: string; targetRef: string; hostname: string; ambiguous: boolean }[]
  rows: { unixUser: string; users: { id: number; name: string; keys: number; missing: boolean }[]; warnings: string[] }[]
  pendingClears: string[]
}

export interface SshAccess {
  servers: SshServer[]
  orphans: { hostName: string; unixUsers: string[]; bound: boolean }[]
  /** Every Headscale user with their SSH key count. */
  people?: { id: number; name: string; keys: number }[]
}

export interface SshAgent {
  id: number
  name: string
  mode: string
  version: string
  versionMatch: boolean
  online: boolean
  lastSeenAt?: string
  discovered: { ref: string; hostname: string; ips?: string[] }[]
  warnings: { ref: string; unix_user?: string; message: string }[]
  bindings: number
  pendingClears: number
}

// ------------------------------------------------------------- endpoints ---

export const api = {
  health: () => request<Health>('/api/health'),
  authStatus: () => request<AuthStatus>('/api/auth/status'),
  me: () => request<Me>('/api/me'),
  login: (email: string, password: string) =>
    send<void>('POST', '/auth/login', { email, password }),
  setPin: (pin: string, password?: string) => send<{ sudoUntil: string }>('POST', '/api/me/pin', { pin, password }),
  startSudo: (pin: string) => send<{ sudoUntil: string }>('POST', '/api/me/sudo', { pin }),
  endSudo: () => send<void>('DELETE', '/api/me/sudo'),
  changePassword: (current: string, next: string) =>
    send<Me>('POST', '/api/me/password', { current, new: next }),

  devices: () => request<{ devices: Device[]; revision: number }>('/api/devices'),
  device: (id: number) => request<Device>(`/api/devices/${id}`),
  deviceRules: (id: number) => request<DeviceRules>(`/api/devices/${id}/rules`),
  renameDevice: (id: number, name: string) =>
    send<Device>('POST', `/api/devices/${id}/rename`, { name }),
  setDeviceTags: (id: number, tags: string[]) =>
    send<Device>('PUT', `/api/devices/${id}/tags`, { tags }),
  approveRoutes: (id: number, routes: string[]) =>
    send<Device>('PUT', `/api/devices/${id}/routes`, { routes }),
  expireDevice: (id: number) => send<Device>('POST', `/api/devices/${id}/expire`),
  deleteDevice: (id: number) => send<void>('DELETE', `/api/devices/${id}`),

  tailnetUsers: () => request<{ users: TailnetUser[] }>('/api/users'),
  createTailnetUser: (body: { name: string; displayName?: string; email?: string }) =>
    send<TailnetUser>('POST', '/api/users', body),
  renameTailnetUser: (id: number, name: string) =>
    send<TailnetUser>('POST', `/api/users/${id}/rename`, { name }),
  deleteTailnetUser: (id: number) => send<void>('DELETE', `/api/users/${id}`),

  accounts: () => request<{ accounts: Account[] }>('/api/accounts'),
  linkAccount: (id: number, headscaleUserId: number | null) =>
    send<Account>('PUT', `/api/accounts/${id}/headscale-user`, { headscaleUserId }),
  setAccountRole: (id: number, role: Role) =>
    send<Account>('PUT', `/api/accounts/${id}/role`, { role }),
  setAccountAdmission: (id: number, admission: 'active' | 'rejected') =>
    send<Account>('PUT', `/api/accounts/${id}/admission`, { admission }),

  policy: () => request<Policy>('/api/policy'),
  previewPolicy: (body: { sha256: string; ops?: PatchOp[]; hujson?: string }) =>
    send<PolicyPreview>('POST', '/api/policy/preview', body),
  savePolicy: (body: { sha256: string; ops?: PatchOp[]; hujson?: string; note?: string }) =>
    send<Policy>('PUT', '/api/policy', body),
  policyRevisions: () => request<{ revisions: PolicyRevision[] }>('/api/policy/revisions'),
  runPolicyTests: (draft?: PolicyDraft) =>
    send<TestRun>('POST', '/api/policy/tests', draft ?? {}),
  simulate: (body: SimulationRequest) =>
    send<Simulation>('POST', '/api/policy/simulate', body),

  preAuthKeys: () => request<{ keys: PreAuthKey[] }>('/api/preauth-keys'),
  revokeActivePreAuthKeys: () => send<{ expired: string[]; failed: string[] }>('POST', '/api/preauth-keys/revoke-active'),

  apiKeys: () => request<{ keys: ApiKey[] }>('/api/headscale-keys'),
  createApiKey: (expiresIn?: string) =>
    send<{ key: string; warning: string }>('POST', '/api/headscale-keys', { expiresIn }),
  expireApiKey: (prefix: string) =>
    send<void>('POST', `/api/headscale-keys/${encodeURIComponent(prefix)}/expire`),

  approveRegistration: (authId: string, user?: string) =>
    send<{ approved: boolean; device?: Device }>('POST', '/api/registrations/approve', {
      authId,
      user,
    }),
  rejectRegistration: (authId: string) =>
    send<{ approved: boolean }>('POST', '/api/registrations/reject', { authId }),
  registrationInfo: () => request<{ headscalePublicUrl: string }>('/api/registrations/info'),
  sshKeys: (userId: number) => request<{ keys: SshKey[] }>(`/api/users/${userId}/ssh-keys`),
  addSshKey: (userId: number, publicKey: string, label: string) =>
    send<SshKey>('POST', `/api/users/${userId}/ssh-keys`, { publicKey, label }),
  deleteSshKey: (id: number) => send<void>('DELETE', `/api/ssh-keys/${id}`),
  sshAccess: () => request<SshAccess>('/api/ssh/access'),
  setSshRow: (host: string, unixUser: string, userIds: number[]) =>
    send<void>('PUT', `/api/ssh/access/${encodeURIComponent(host)}/${encodeURIComponent(unixUser)}`, { userIds }),
  approveSshTarget: (host: string, agentId: number, targetRef: string) =>
    send<void>('POST', `/api/ssh/targets/${encodeURIComponent(host)}`, { agentId, targetRef }),
  unbindSshTarget: (host: string) => send<void>('DELETE', `/api/ssh/targets/${encodeURIComponent(host)}`),
  removeSshOrphan: (host: string) => send<void>('DELETE', `/api/ssh/access/${encodeURIComponent(host)}`),
  audit: (q: { category?: AuditCategory; actor?: string; before?: number }) => {
    const p = new URLSearchParams()
    if (q.category) p.set('category', q.category)
    if (q.actor) p.set('actor', q.actor)
    if (q.before) p.set('before', String(q.before))
    const qs = p.toString()

    return request<{ entries: AuditEntry[]; next: number }>(`/api/audit${qs ? `?${qs}` : ''}`)
  },
  sshAgents: () => request<{ agents: SshAgent[] }>('/api/ssh/agents'),
  createSshAgent: (name: string) => send<{ agent: SshAgent; token: string }>('POST', '/api/ssh/agents', { name }),
  rotateSshAgent: (id: number) => send<{ token: string }>('POST', `/api/ssh/agents/${id}/rotate`),
  revokeSshAgent: (id: number, force = false) =>
    send<{ cleanup: string[] }>('DELETE', `/api/ssh/agents/${id}${force ? '?force=true' : ''}`),
}
