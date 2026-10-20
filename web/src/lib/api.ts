import { keepPreviousData, useQuery } from '@tanstack/react-query'

export type Meta = {
  version: string
  commit: string
  date: string
  auth_configured: boolean
  provider?: string
  issuer?: string
  tls_enabled: boolean
  provider_presets?: ProviderPreset[]
  demo_mode?: boolean
  demo_reset_at?: string
  demo_reset_interval_seconds?: number
}

export type ProviderPreset = {
  id: string
  label: string
  issuer?: string
  scopes: string[]
  groups_claim: string
  email_claim: string
  name_claim: string
  notes?: string
}

export type Role = 'viewer' | 'member' | 'admin'

export type User = {
  id: string
  email: string
  display_name: string
  role: Role
  last_login_at?: string
  created_at: string
}

export type AuthState = {
  user: User | null
  csrf_token: string | null
}

export type Stats = {
  total: number
  active: number
  expired: number
  expiring_7: number
  expiring_30: number
  expiring_90: number
  unknown: number
  paused: number
  archived: number
  open_incidents: number
}

export type Domain = {
  id: string
  name: string
  name_ace: string
  registrar?: string
  expires_at?: string
  days_left?: number
  state: 'active' | 'paused' | 'archived'
  inherit_global_alerts: boolean
  muted_until?: string
  mute_reason?: string
  check_interval_seconds: number
  next_check_at?: string
  last_checked_at?: string
  last_check_status?: string
  last_check_error?: string
  expiry_confidence: string
  consecutive_failures: number
  version: number
  notes?: string
  created_at: string
  updated_at: string
  archived_at?: string
}

export type Check = {
  id: string
  domain_id: string
  source: 'rdap' | 'whois'
  status: string
  http_status?: number
  expires_at?: string
  registrar?: string
  confidence: string
  error_code?: string
  error_message?: string
  duration_ms: number
  started_at: string
  finished_at?: string
}

export type Rule = {
  id: string
  name: string
  scope: 'global' | 'domain'
  domain_id?: string
  offset_value: number
  offset_unit: 'days' | 'months'
  direction: 'before' | 'after'
  send_time_local: string
  enabled: boolean
  channel_ids: string[]
  created_at: string
  updated_at: string
}

export type ChannelType = 'email' | 'webhook' | 'discord' | 'telegram'

export type Channel = {
  id: string
  name: string
  type: ChannelType
  enabled: boolean
  receive_system_alerts: boolean
  config: Record<string, unknown>
  muted_until?: string
  mute_reason?: string
  archived_at?: string
  created_at: string
  updated_at: string
}

export type Retention = {
  raw_days: number
  checks_days: number
  notifications_days: number
  managed_by_env: boolean
}

export type CheckSettings = {
  default_interval: number
  manual_rate_limit: number
  managed_by_env: boolean
}

export type Settings = {
  smtp: {
    host: string
    port: number
    tls_mode: string
    username: string
    from: string
    password_set: boolean
    managed_by_env: boolean
  }
  alerts: {
    timezone: string
    default_time: string
    paused_until?: string
    managed_by_env: boolean
  }
  retention: Retention
  check: CheckSettings
}

export type Incident = {
  id: string
  domain_id: string
  kind: string
  state: 'open' | 'resolved'
  opened_at: string
  resolved_at?: string
  last_seen_at: string
  details: string
  updated_at: string
}

export type Notification = {
  id: string
  kind: string
  domain_id?: string
  domain_name?: string
  incident_id?: string
  rule_id?: string
  channel_id?: string
  channel_name?: string
  expiry_date?: string
  state: string
  scheduled_at: string
  next_attempt_at?: string
  attempts: number
  last_error?: string
  created_at: string
  sent_at?: string
}

export type AuditEntry = {
  id: string
  actor_user_id?: string
  actor_email?: string
  actor_display_name?: string
  action: string
  entity_type: string
  entity_id?: string
  before?: string
  after?: string
  ip?: string
  user_agent?: string
  created_at: string
}

export type Job = {
  id: string
  kind: string
  state: string
  run_at: string
  attempts: number
  max_attempts: number
  last_error?: string
  unique_key?: string
  created_at: string
  finished_at?: string
}

type Problem = {
  title?: string
  detail?: string
  status?: number
  errors?: Record<string, string[]>
}

export class ApiError extends Error {
  status: number
  detail: string
  fields?: Record<string, string[]>

  constructor(status: number, title: string, detail: string, fields?: Record<string, string[]>) {
    super(title)
    this.name = 'ApiError'
    this.status = status
    this.detail = detail
    this.fields = fields
  }
}

let csrfToken: string | null = null

export function setCSRFToken(token: string | null) {
  csrfToken = token
}

async function readProblem(response: Response): Promise<Problem> {
  try {
    return (await response.json()) as Problem
  } catch {
    return {}
  }
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers)
  headers.set('Accept', 'application/json')
  const method = init.method ?? 'GET'
  if (init.body && !(init.body instanceof FormData) && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json')
  }
  if (method !== 'GET' && method !== 'HEAD' && csrfToken) {
    headers.set('X-CSRF-Token', csrfToken)
  }
  const response = await fetch(path, {
    credentials: 'same-origin',
    ...init,
    headers,
  })
  if (response.status === 401 && path !== '/api/v1/me') {
    const next = `${window.location.pathname}${window.location.search}`
    window.location.assign(`/login?next=${encodeURIComponent(next)}`)
    throw new ApiError(401, 'Authentication required', 'Redirecting to sign in.')
  }
  if (!response.ok) {
    const problem = await readProblem(response)
    throw new ApiError(
      response.status,
      problem.title ?? `Request failed (${response.status})`,
      problem.detail ?? response.statusText,
      problem.errors,
    )
  }
  if (response.status === 204) {
    return undefined as T
  }
  return (await response.json()) as T
}

function queryString(params: Record<string, string | number | boolean | undefined>) {
  const search = new URLSearchParams()
  for (const [key, value] of Object.entries(params)) {
    if (value !== undefined && value !== '') {
      search.set(key, String(value))
    }
  }
  const value = search.toString()
  return value ? `?${value}` : ''
}

export async function fetchMeta(signal?: AbortSignal): Promise<Meta> {
  return request<Meta>('/api/v1/meta', { signal })
}

export async function fetchMe(signal?: AbortSignal): Promise<AuthState> {
  const response = await fetch('/api/v1/me', {
    signal,
    credentials: 'same-origin',
    headers: { Accept: 'application/json' },
  })
  if (response.status === 401 || response.status === 503) {
    setCSRFToken(null)
    return { user: null, csrf_token: null }
  }
  if (!response.ok) {
    throw new ApiError(response.status, 'Session check failed', response.statusText)
  }
  const state = (await response.json()) as AuthState
  setCSRFToken(state.csrf_token)
  return state
}

export const api = {
  stats: () => request<Stats>('/api/v1/stats'),

  domains: (params: {
    search?: string
    state?: string
    expiry?: string
    limit?: number
    offset?: number
  }) => request<{ items: Domain[]; total: number; limit: number; offset: number }>(`/api/v1/domains${queryString(params)}`),

  domain: (id: string) => request<Domain>(`/api/v1/domains/${id}`),

  createDomain: (input: { name: string; notes?: string }) =>
    request<Domain>('/api/v1/domains', { method: 'POST', body: JSON.stringify(input) }),

  updateDomain: (
    id: string,
    input: {
      version: number
      registrar?: string
      notes?: string
      inherit_global_alerts?: boolean
      check_interval_seconds?: number
      state?: string
    },
  ) => request<Domain>(`/api/v1/domains/${id}`, { method: 'PATCH', body: JSON.stringify(input) }),

  archiveDomain: (id: string) => request<void>(`/api/v1/domains/${id}`, { method: 'DELETE' }),

  restoreDomain: (id: string) => request<Domain>(`/api/v1/domains/${id}/restore`, { method: 'POST' }),

  deleteDomainPermanent: (id: string) =>
    request<void>(`/api/v1/domains/${id}/permanent`, { method: 'DELETE' }),

  checkDomain: (id: string) => request<{ queued: boolean }>(`/api/v1/domains/${id}/check`, { method: 'POST' }),

  muteDomain: (id: string, input: { until: string; reason?: string }) =>
    request<Domain>(`/api/v1/domains/${id}/mute`, { method: 'POST', body: JSON.stringify(input) }),

  unmuteDomain: (id: string) => request<Domain>(`/api/v1/domains/${id}/unmute`, { method: 'POST' }),

  importDomains: (file: File) => {
    const body = new FormData()
    body.set('file', file)
    return request<{ imported: number; failed: Array<{ domain: string; error: string }> }>('/api/v1/domains/import', {
      method: 'POST',
      body,
    })
  },

  checks: (id: string, limit = 50) => request<{ items: Check[] }>(`/api/v1/domains/${id}/checks?limit=${limit}`),

  rawCheckURL: (domainID: string, checkID: string) =>
    `/api/v1/domains/${domainID}/checks/${checkID}/raw`,

  rules: (params: { scope?: string; domain_id?: string } = {}) =>
    request<{ items: Rule[] }>(`/api/v1/alert-rules${queryString(params)}`),

  createRule: (input: {
    name: string
    scope: 'global' | 'domain'
    domain_id?: string
    offset_value: number
    offset_unit: 'days' | 'months'
    direction: 'before' | 'after'
    send_time_local: string
    enabled: boolean
    channel_ids: string[]
  }) => request<Rule>('/api/v1/alert-rules', { method: 'POST', body: JSON.stringify(input) }),

  updateRule: (
    id: string,
    input: {
      name: string
      offset_value: number
      offset_unit: 'days' | 'months'
      direction: 'before' | 'after'
      send_time_local: string
      enabled: boolean
      channel_ids: string[]
    },
  ) => request<Rule>(`/api/v1/alert-rules/${id}`, { method: 'PATCH', body: JSON.stringify(input) }),

  archiveRule: (id: string) => request<void>(`/api/v1/alert-rules/${id}`, { method: 'DELETE' }),

  channels: () => request<{ items: Channel[] }>('/api/v1/channels'),

  createChannel: (input: {
    name: string
    type: ChannelType
    config: Record<string, unknown>
    enabled: boolean
    receive_system_alerts: boolean
  }) => request<Channel>('/api/v1/channels', { method: 'POST', body: JSON.stringify(input) }),

  updateChannel: (
    id: string,
    input: {
      name: string
      config?: Record<string, unknown>
      enabled: boolean
      receive_system_alerts: boolean
    },
  ) => request<Channel>(`/api/v1/channels/${id}`, { method: 'PATCH', body: JSON.stringify(input) }),

  archiveChannel: (id: string) => request<void>(`/api/v1/channels/${id}`, { method: 'DELETE' }),

  testChannel: (id: string) => request<{ status: string }>(`/api/v1/channels/${id}/test`, { method: 'POST' }),

  muteChannel: (id: string, input: { until: string; reason?: string }) =>
    request<Channel>(`/api/v1/channels/${id}/mute`, { method: 'POST', body: JSON.stringify(input) }),

  unmuteChannel: (id: string) => request<Channel>(`/api/v1/channels/${id}/unmute`, { method: 'POST' }),

  settings: () => request<Settings>('/api/v1/settings'),

  saveSMTP: (input: {
    host: string
    port: number
    tls_mode: string
    username: string
    password?: string
    from: string
  }) => request<{ status: string }>('/api/v1/settings/smtp', { method: 'PUT', body: JSON.stringify(input) }),

  testSMTP: (to: string[]) =>
    request<{ status: string }>('/api/v1/settings/smtp/test', { method: 'POST', body: JSON.stringify({ to }) }),

  saveAlerts: (input: { timezone: string; default_time: string; paused_until?: string | null }) =>
    request<{ status: string }>('/api/v1/settings/alerts', { method: 'PUT', body: JSON.stringify(input) }),

  saveRetention: (input: { raw_days: number; checks_days: number; notifications_days: number }) =>
    request<{ status: string }>('/api/v1/settings/retention', { method: 'PUT', body: JSON.stringify(input) }),

  saveCheck: (input: { default_interval: number; manual_rate_limit: number }) =>
    request<{ status: string }>('/api/v1/settings/check', { method: 'PUT', body: JSON.stringify(input) }),

  incidents: (params: { domain_id?: string; state?: string; limit?: number } = {}) =>
    request<{ items: Incident[] }>(`/api/v1/incidents${queryString(params)}`),

  notifications: (params: {
    domain_id?: string
    channel_id?: string
    state?: string
    limit?: number
    exclude_archived?: boolean
    include_future?: boolean
  } = {}) => request<{ items: Notification[] }>(`/api/v1/notifications${queryString(params)}`),

  domainNotifications: (id: string) =>
    request<{ items: Notification[] }>(`/api/v1/domains/${id}/notifications`),

  domainIncidents: (id: string) => request<{ items: Incident[] }>(`/api/v1/domains/${id}/incidents`),

  retryNotification: (id: string) =>
    request<{ queued: boolean }>(`/api/v1/notifications/${id}/retry`, { method: 'POST' }),

  users: () => request<{ items: User[] }>('/api/v1/users'),

  setUserRole: (id: string, role: Role) =>
    request<{ status: string }>(`/api/v1/users/${id}/role`, {
      method: 'PATCH',
      body: JSON.stringify({ role }),
    }),

  revokeUserSessions: (id: string) =>
    request<void>(`/api/v1/users/${id}/sessions`, { method: 'DELETE' }),

  audit: (limit = 100) => request<{ items: AuditEntry[] }>(`/api/v1/audit?limit=${limit}`),

  jobs: (limit = 50) => request<{ items: Job[] }>(`/api/v1/jobs?limit=${limit}`),

  logout: () => request<void>('/api/v1/auth/logout', { method: 'POST' }),
}

export function useMeta() {
  return useQuery({
    queryKey: ['meta'],
    queryFn: ({ signal }) => fetchMeta(signal),
    staleTime: 30_000,
    retry: 1,
  })
}

export function useMe() {
  return useQuery({
    queryKey: ['me'],
    queryFn: ({ signal }) => fetchMe(signal),
    staleTime: 30_000,
    retry: 0,
  })
}

export function useStats() {
  return useQuery({
    queryKey: ['stats'],
    queryFn: api.stats,
    staleTime: 15_000,
    refetchInterval: 30_000,
  })
}

export function useDomains(params: {
  search?: string
  state?: string
  expiry?: string
  limit?: number
  offset?: number
} = {}) {
  return useQuery({
    queryKey: ['domains', params],
    queryFn: () => api.domains(params),
    placeholderData: keepPreviousData,
    staleTime: 5_000,
    refetchInterval: 15_000,
  })
}

export function useDomain(id: string | null) {
  return useQuery({
    queryKey: ['domain', id],
    queryFn: () => api.domain(id as string),
    enabled: Boolean(id),
    staleTime: 5_000,
    refetchInterval: 10_000,
  })
}

export function useChecks(id: string | null) {
  return useQuery({
    queryKey: ['checks', id],
    queryFn: () => api.checks(id as string),
    enabled: Boolean(id),
    staleTime: 5_000,
    refetchInterval: 15_000,
  })
}

export function useRules(params: { scope?: string; domain_id?: string } = {}) {
  return useQuery({ queryKey: ['rules', params], queryFn: () => api.rules(params) })
}

export function useChannels() {
  return useQuery({ queryKey: ['channels'], queryFn: api.channels })
}

export function useSettings() {
  return useQuery({ queryKey: ['settings'], queryFn: api.settings })
}

export function useIncidents(params: { domain_id?: string; state?: string; limit?: number } = {}) {
  return useQuery({ queryKey: ['incidents', params], queryFn: () => api.incidents(params) })
}

export function useNotifications(params: {
  domain_id?: string
  channel_id?: string
  state?: string
  limit?: number
  exclude_archived?: boolean
  include_future?: boolean
} = {}) {
  return useQuery({
    queryKey: ['notifications', params],
    queryFn: () => api.notifications(params),
    placeholderData: keepPreviousData,
  })
}

export function useUsers() {
  return useQuery({ queryKey: ['users'], queryFn: api.users })
}

export function useAudit(limit = 100) {
  return useQuery({ queryKey: ['audit', limit], queryFn: () => api.audit(limit) })
}

export function useJobs(limit = 50) {
  return useQuery({ queryKey: ['jobs', limit], queryFn: () => api.jobs(limit) })
}
