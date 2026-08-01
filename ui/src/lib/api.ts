/**
 * Thin client for Headboard's own API.
 *
 * The browser never talks to Headscale directly — the admin API key lives only
 * in the Go process — so this is the single network surface the UI has.
 */

export class ApiError extends Error {
  readonly status: number

  constructor(status: number, message: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    credentials: 'same-origin',
    headers: { Accept: 'application/json', ...init?.headers },
    ...init,
  })

  if (!res.ok) {
    // Huma reports failures as RFC 9457 problem+json; fall back to the status
    // line when the body is not JSON (a proxy error page, say).
    const detail = await res
      .json()
      .then((body: { detail?: string; title?: string }) => body.detail ?? body.title)
      .catch(() => undefined)

    throw new ApiError(res.status, detail ?? `${res.status} ${res.statusText}`)
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
  ssh?: SshRule[]
  autoApprovers?: {
    routes?: Record<string, string[]>
    exitNode?: string[]
    services?: Record<string, string[]>
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
  because?: Attribution
  rule?: EffectiveRule
}

/** A pending change, so tests and the simulator can answer for a draft. */
export type PolicyDraft = { sha256?: string; ops?: PatchOp[]; hujson?: string }

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

// ------------------------------------------------------------- endpoints ---

export const api = {
  health: () => request<Health>('/api/health'),
  authStatus: () => request<AuthStatus>('/api/auth/status'),
  me: () => request<Me>('/api/me'),
  login: (email: string, password: string) =>
    send<void>('POST', '/auth/login', { email, password }),
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
  simulate: (body: { src: number; dst: number; port: number } & PolicyDraft) =>
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
}
