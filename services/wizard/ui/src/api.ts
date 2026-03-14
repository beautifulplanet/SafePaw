// =============================================================
// SafePaw Wizard — API Client
// =============================================================
// Typed fetch wrapper for the wizard REST API. All endpoints
// return JSON and use the admin token for auth.
// =============================================================

const BASE = '/api/v1'

let token: string | null = null

/** Current user role from session (admin, operator, viewer). Used for RBAC UI. */
let userRole: string | null = null

/** Clear the auth token (logout). */
export function clearToken() {
  token = null
  userRole = null
}

/** Set current user role (call after login or from GET /auth/me). */
export function setUserRole(role: string) {
  userRole = role
}

/** Current user role, or null if not known. */
export function getUserRole(): string | null {
  return userRole
}

/** Check if we have a stored token. */
export function hasToken(): boolean {
  return token !== null
}

function getCsrfToken(): string {
  const match = document.cookie.match(/(?:^|;\s*)csrf=([^;]+)/)
  return match?.[1] ?? ''
}

async function request<T>(path: string, opts: RequestInit = {}): Promise<T> {
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...(opts.headers as Record<string, string> ?? {}),
  }
  if (token) {
    headers['Authorization'] = `Bearer ${token}`
  }

  // Add CSRF token for mutating requests using cookie-based auth
  const method = (opts.method ?? 'GET').toUpperCase()
  if (!token && (method === 'POST' || method === 'PUT' || method === 'DELETE')) {
    const csrf = getCsrfToken()
    if (csrf) {
      headers['X-CSRF-Token'] = csrf
    }
  }

  const res = await fetch(`${BASE}${path}`, { ...opts, headers, credentials: 'same-origin' })

  if (res.status === 401) {
    clearToken()
    userRole = null
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
  expires_in: number
  role: string
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

// ─── Setup Verification ──────────────────────────────────────

export interface VerifyCheck {
  name: string
  pass: boolean
  message: string
}

export interface VerifyResponse {
  checks: VerifyCheck[]
  overall: boolean
}

// ─── Cost Analytics (Postgres-backed) ────────────────────────

export interface CostDailyRow {
  date: string
  totalTokens: number
  totalCost: number
  promptTokens: number
  completionTokens: number
  messages: number
  toolCalls: number
}

export interface CostHistoryResponse {
  status: string
  days?: number
  daily?: CostDailyRow[]
  error?: string
}

export interface CostModelRow {
  date: string
  provider: string
  model: string
  requestCount: number
  totalTokens: number
  totalCost: number
  promptTokens: number
  completionTokens: number
}

export interface CostModelsResponse {
  status: string
  days?: number
  models?: CostModelRow[]
  error?: string
}

export interface CostTrend {
  recentDays: number
  recentCost: number
  recentTokens: number
  priorCost: number
  priorTokens: number
  costChangePct: number
  tokenChangePct: number
  dailyAvgRecent: number
  dailyAvgPrior: number
  anomalyScore: number
}

export interface CostTrendsResponse {
  status: string
  trend?: CostTrend
  error?: string
}

// ─── Endpoints ───────────────────────────────────────────────

export const api = {
  health: () =>
    request<HealthResponse>('/health'),

  login: async (password: string, totp?: string) => {
    const res = await request<LoginResponse>('/auth/login', {
      method: 'POST',
      body: JSON.stringify({ password, ...(totp && totp.trim() && { totp: totp.trim() }) }),
    })
    setUserRole(res.role)
    return res
  },

  /** Current session role (for RBAC UI). Call on app load to restore session + role. */
  authMe: () =>
    request<{ role: string }>('/auth/me'),

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

  /** Get historical daily cost snapshots from Postgres. */
  costHistory: (days = 30) =>
    request<CostHistoryResponse>(`/cost/history?days=${days}`),

  /** Get per-model cost breakdown from Postgres. */
  costModels: (days = 30) =>
    request<CostModelsResponse>(`/cost/models?days=${days}`),

  /** Get trend analysis (recent vs prior period) from Postgres. */
  costTrends: (days = 7) =>
    request<CostTrendsResponse>(`/cost/trends?days=${days}`),

  /** Run setup verification (API key + gateway + backend round-trip). */
  setupVerify: () =>
    request<VerifyResponse>('/setup/verify', { method: 'POST' }),
}
