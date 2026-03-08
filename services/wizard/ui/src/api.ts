// =============================================================
// SafePaw Wizard — API Client
// =============================================================
// Typed fetch wrapper for the wizard REST API. All endpoints
// return JSON and use the admin token for auth.
// =============================================================

const BASE = '/api/v1'

let token: string | null = null

/** Set the auth token for subsequent requests. */
export function setToken(t: string) {
  token = t
}

/** Clear the auth token (logout). */
export function clearToken() {
  token = null
}

/** Check if we have a stored token. */
export function hasToken(): boolean {
  return token !== null
}

async function request<T>(path: string, opts: RequestInit = {}): Promise<T> {
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...(opts.headers as Record<string, string> ?? {}),
  }
  if (token) {
    headers['Authorization'] = `Bearer ${token}`
  }

  const res = await fetch(`${BASE}${path}`, { ...opts, headers })

  if (res.status === 401) {
    clearToken()
    throw new ApiError('Unauthorized', 401)
  }

  if (!res.ok) {
    const body = await res.json().catch(() => ({ error: res.statusText }))
    throw new ApiError(body.error ?? 'Unknown error', res.status)
  }

  return res.json() as Promise<T>
}

export class ApiError extends Error {
  constructor(message: string, public status: number) {
    super(message)
    this.name = 'ApiError'
  }
}

// ─── Types ───────────────────────────────────────────────────

export interface HealthResponse {
  status: string
  service: string
  version: string
  uptime: string
  needs_setup: boolean
}

export interface LoginResponse {
  token: string
  expires_in: number
}

export interface PrerequisiteCheck {
  name: string
  status: 'pass' | 'fail' | 'warn'
  message: string
  help_url?: string
  required: boolean
}

export interface PrerequisitesResponse {
  checks: PrerequisiteCheck[]
  all_pass: boolean
}

export interface ServiceInfo {
  name: string
  id: string
  state: string
  health: string
  image: string
  uptime?: string
}

export interface StatusResponse {
  services: ServiceInfo[]
  overall: 'healthy' | 'degraded' | 'down' | 'unknown'
}

export interface ConfigResponse {
  config: Record<string, string>
}

export interface GatewayTokenResponse {
  token: string
  expires_at: string
}

export interface GatewayMetrics {
  total_requests: number
  auth_failures: number
  injections_found: number
  rate_limited: number
  active_connections: number
  avg_response_ms: number
  tokens_revoked: number
  gateway_reachable: boolean
}

export interface PathCount {
  path: string
  count: number
}

export interface GatewayActivity {
  metrics: GatewayMetrics
  top_paths: PathCount[]
  recent_ips: string[]
}

export interface UsageDailyEntry {
  date: string
  totalCost: number
  totalTokens: number
}

export interface UsageTotals {
  input: number
  output: number
  cacheRead: number
  cacheWrite: number
  totalTokens: number
  totalCost: number
}

export interface UsageResponse {
  status: string
  collector?: string
  updatedAt?: string
  alert?: string
  warnThresholdUsd?: number
  critThresholdUsd?: number
  todayCostUsd?: number
  periodCostUsd?: number
  days?: number
  daily?: UsageDailyEntry[]
  totals?: UsageTotals
}

// ─── Endpoints ───────────────────────────────────────────────

export const api = {
  health: () =>
    request<HealthResponse>('/health'),

  login: (password: string, totp?: string) =>
    request<LoginResponse>('/auth/login', {
      method: 'POST',
      body: JSON.stringify({ password, ...(totp && totp.trim() && { totp: totp.trim() }) }),
    }),

  prerequisites: () =>
    request<PrerequisitesResponse>('/prerequisites'),

  status: () =>
    request<StatusResponse>('/status'),

  /** Restart a SafePaw service (wizard, gateway, openclaw, redis, postgres). */
  restartService: (name: string) =>
    request<{ status: string; service: string }>(`/services/${encodeURIComponent(name)}/restart`, {
      method: 'POST',
    }),

  /** Get current .env config (secrets masked). */
  getConfig: () =>
    request<ConfigResponse>('/config'),

  /** Update allowed config keys. Keys not in allowlist are ignored. */
  putConfig: (updates: Record<string, string>) =>
    request<{ status: string }>('/config', {
      method: 'PUT',
      body: JSON.stringify(updates),
    }),

  /** Generate a gateway auth token (for "Open AI Assistant" button). */
  gatewayToken: (subject = 'wizard-proxy', scope = 'proxy', ttlHours = 24) =>
    request<GatewayTokenResponse>('/gateway/token', {
      method: 'POST',
      body: JSON.stringify({ subject, scope, ttl_hours: ttlHours }),
    }),

  /** Get parsed gateway metrics summary. */
  gatewayMetrics: () =>
    request<GatewayMetrics>('/gateway/metrics'),

  /** Get gateway activity (metrics + top paths). */
  gatewayActivity: () =>
    request<GatewayActivity>('/gateway/activity'),

  /** Get LLM cost/usage data proxied from gateway. */
  gatewayUsage: () =>
    request<UsageResponse>('/gateway/usage'),
}
