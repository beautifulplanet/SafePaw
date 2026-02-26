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

// ─── Endpoints ───────────────────────────────────────────────

export const api = {
  health: () =>
    request<HealthResponse>('/health'),

  login: (password: string) =>
    request<LoginResponse>('/auth/login', {
      method: 'POST',
      body: JSON.stringify({ password }),
    }),

  prerequisites: () =>
    request<PrerequisitesResponse>('/prerequisites'),

  status: () =>
    request<StatusResponse>('/status'),
}
