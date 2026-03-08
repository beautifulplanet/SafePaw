import { useState, useEffect, useCallback, useRef } from 'react'
import { api, type ServiceInfo, type StatusResponse, type GatewayMetrics, type UsageResponse } from '../api'

const POLL_INTERVAL = 5000 // ms

interface DashboardProps {
  onOpenConfig?: () => void
  onOpenActivity?: () => void
  onOpenSettings?: () => void
}

export function Dashboard({ onOpenActivity, onOpenSettings }: DashboardProps) {
  const [data, setData] = useState<StatusResponse | null>(null)
  const [metrics, setMetrics] = useState<GatewayMetrics | null>(null)
  const [usage, setUsage] = useState<UsageResponse | null>(null)
  const [error, setError] = useState('')
  const [restarting, setRestarting] = useState<string | null>(null)
  const [openingAssistant, setOpeningAssistant] = useState(false)
  const intervalRef = useRef<ReturnType<typeof setInterval>>(undefined)
  const usageIntervalRef = useRef<ReturnType<typeof setInterval>>(undefined)

  const fetchStatus = useCallback(async () => {
    try {
      const res = await api.status()
      setData(res)
      setError('')
    } catch {
      setError('Failed to fetch service status')
    }
  }, [])

  const fetchMetrics = useCallback(async () => {
    try {
      const res = await api.gatewayMetrics()
      setMetrics(res)
    } catch {
      // Metrics are non-critical, don't show error
    }
  }, [])

  const fetchUsage = useCallback(async () => {
    try {
      const res = await api.gatewayUsage()
      setUsage(res)
    } catch {
      // Usage is non-critical, don't show error
    }
  }, [])

  const handleOpenAssistant = useCallback(async () => {
    setOpeningAssistant(true)
    try {
      const { token } = await api.gatewayToken('wizard-user', 'proxy', 1)
      const url = `${window.location.protocol}//${window.location.hostname}:8080/?token=${encodeURIComponent(token)}`
      window.open(url, '_blank', 'noopener,noreferrer')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to open AI assistant')
    } finally {
      setOpeningAssistant(false)
    }
  }, [])

  const handleRestart = useCallback(async (name: string) => {
    if (restarting) return
    setRestarting(name)
    try {
      await api.restartService(name)
      await fetchStatus()
    } catch {
      setError(`Failed to restart ${name}`)
    } finally {
      setRestarting(null)
    }
  }, [restarting, fetchStatus])

  useEffect(() => {
    void fetchStatus()
    void fetchMetrics()
    void fetchUsage()
    intervalRef.current = setInterval(() => {
      void fetchStatus()
      void fetchMetrics()
    }, POLL_INTERVAL)
    usageIntervalRef.current = setInterval(() => {
      void fetchUsage()
    }, 60000)
    return () => {
      clearInterval(intervalRef.current)
      clearInterval(usageIntervalRef.current)
    }
  }, [fetchStatus, fetchMetrics, fetchUsage])

  return (
    <div>
      <div className="flex items-center justify-between mb-8">
        <div>
          <h2 className="text-2xl font-bold tracking-tight">Home</h2>
          <p className="text-gray-400 mt-1">
            Everything running your private AI — at a glance.
          </p>
        </div>
        <div className="flex items-center gap-3">
          {data && <OverallBadge overall={data.overall} />}
          <button
            onClick={handleOpenAssistant}
            disabled={openingAssistant}
            className="btn-primary text-sm py-1.5 px-4 flex items-center gap-2"
          >
            <span>💬</span>
            {openingAssistant ? 'Opening…' : 'Chat with AI'}
          </button>
          <button onClick={fetchStatus} className="btn-secondary text-sm py-1.5 px-3">
            Refresh
          </button>
        </div>
      </div>

      {error && (
        <div className="rounded-lg bg-red-500/10 border border-red-500/20 px-4 py-3 text-sm text-red-400 mb-6">
          {error}
        </div>
      )}

      {/* Quick Stats Row */}
      {metrics && metrics.gateway_reachable && (
        <div className="grid grid-cols-2 sm:grid-cols-4 gap-4 mb-8">
          <StatCard label="Conversations" value={metrics.total_requests} hint="Total messages sent to the AI" />
          <StatCard label="People Online" value={metrics.active_connections} hint="Users chatting right now" />
          <StatCard label="Blocked Logins" value={metrics.auth_failures} warn={metrics.auth_failures > 0} hint="Someone tried to access without permission" />
          <StatCard label="Threats Stopped" value={metrics.injections_found + metrics.rate_limited} warn={metrics.injections_found > 0} hint="Malicious messages caught and blocked" />
        </div>
      )}

      {/* LLM Usage & Cost */}
      {usage && usage.status === 'ok' && <CostPanel usage={usage} />}

      {!data ? (
        // Loading skeleton
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {Array.from({ length: 5 }).map((_, i) => (
            <div key={i} className="card animate-pulse">
              <div className="h-4 bg-gray-800 rounded w-24 mb-3" />
              <div className="h-3 bg-gray-800/50 rounded w-36 mb-2" />
              <div className="h-3 bg-gray-800/50 rounded w-20" />
            </div>
          ))}
        </div>
      ) : data.services.length === 0 ? (
        <EmptyState />
      ) : (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {data.services.map((svc, i) => (
            <ServiceCard
              key={svc.name || svc.id}
              service={svc}
              onRestart={handleRestart}
              restarting={restarting}
              index={i}
            />
          ))}
        </div>
      )}

      {/* How it works */}
      <div className="mt-10 card">
        <h3 className="font-semibold mb-1">How it works</h3>
        <p className="text-xs text-gray-500 mb-4">Your AI runs on your own servers. No data leaves your network.</p>
        <div className="grid sm:grid-cols-3 gap-4 text-sm">
          <div className="space-y-1">
            <p className="text-gray-400 font-medium">🛡️ Security Layer</p>
            <p className="text-gray-500">Every message is scanned for attacks before it reaches the AI. Spam and abuse are automatically blocked.</p>
          </div>
          <div className="space-y-1">
            <p className="text-gray-400 font-medium">🤖 Private AI</p>
            <p className="text-gray-500">Your AI assistant lives behind the security layer. It's never directly exposed to the internet.</p>
          </div>
          <div className="space-y-1">
            <p className="text-gray-400 font-medium">🔒 Your Data</p>
            <p className="text-gray-500">Conversations stay on your servers. Only the AI provider sees your prompts — no middlemen.</p>
          </div>
        </div>
      </div>

      {/* Quick nav */}
      <div className="mt-6 flex gap-3">
        {onOpenActivity && (
          <button onClick={onOpenActivity} className="btn-secondary text-sm py-2 px-4 flex-1">
            📊 View Activity
          </button>
        )}
        {onOpenSettings && (
          <button onClick={onOpenSettings} className="btn-secondary text-sm py-2 px-4 flex-1">
            ⚙️ Settings
          </button>
        )}
      </div>
    </div>
  )
}

function StatCard({ label, value, warn, hint }: { label: string; value: number; warn?: boolean; hint?: string }) {
  return (
    <div className="card py-3 px-4 card-enter" title={hint}>
      <p className="text-xs text-gray-500 mb-1">{label}</p>
      <p className={`text-2xl font-bold tabular-nums ${warn ? 'text-yellow-400' : 'text-gray-100'}`}>
        {value.toLocaleString()}
      </p>
      {hint && <p className="text-[10px] text-gray-600 mt-1 leading-tight">{hint}</p>}
    </div>
  )
}

const SERVICE_INFO: Record<string, { label: string; emoji: string; desc: string }> = {
  wizard:   { label: 'Control Panel',    emoji: '🐾', desc: 'This admin interface you\'re looking at right now.' },
  gateway:  { label: 'Security Shield',  emoji: '🛡️', desc: 'Scans every message for threats and blocks unauthorized access.' },
  openclaw: { label: 'AI Assistant',     emoji: '🤖', desc: 'The private AI your team chats with.' },
  redis:    { label: 'Fast Memory',      emoji: '⚡', desc: 'Temporary storage that makes everything respond quickly.' },
  postgres: { label: 'Database',         emoji: '💾', desc: 'Stores conversation history and settings permanently.' },
}

const RESTARTABLE_SERVICES = ['wizard', 'gateway', 'openclaw', 'redis', 'postgres']

function ServiceCard({ service, onRestart, restarting, index }: { service: ServiceInfo; onRestart: (name: string) => void; restarting: string | null; index: number }) {
  const stateColor = getStateColor(service.state)
  const healthColor = getHealthColor(service.health)
  const name = service.name || 'unknown'
  const friendly = SERVICE_INFO[name] || { label: name, emoji: '📦', desc: '' }
  const canRestart = RESTARTABLE_SERVICES.includes(name)
  const isRestarting = restarting === name

  return (
    <div className="card group hover:border-gray-700 transition-colors card-enter" style={{ animationDelay: `${index * 60}ms` }}>
      {/* Header */}
      <div className="flex items-center justify-between mb-3">
        <div className="flex items-center gap-2">
          <span className="text-lg">{friendly.emoji}</span>
          <h3 className="font-semibold text-lg">{friendly.label}</h3>
        </div>
        <div className="flex items-center gap-2">
          {canRestart && (
            <button
              type="button"
              onClick={() => onRestart(name)}
              disabled={isRestarting}
              className="text-xs px-2 py-1 rounded border border-gray-600 text-gray-400 hover:border-gray-500 hover:text-gray-300 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {isRestarting ? 'Restarting…' : 'Restart'}
            </button>
          )}
          <div className={`w-2.5 h-2.5 rounded-full ${stateColor} ${service.state === 'running' ? 'status-pulse' : ''}`} />
        </div>
      </div>

      {/* What this does */}
      {friendly.desc && <p className="text-xs text-gray-500 mb-3">{friendly.desc}</p>}

      {/* Details */}
      <div className="space-y-2 text-sm">
        <div className="flex justify-between">
          <span className="text-gray-500">State</span>
          <span className={`font-medium ${stateColor.replace('bg-', 'text-').replace('/50', '')}`}>
            {service.state}
          </span>
        </div>
        <div className="flex justify-between">
          <span className="text-gray-500">Health</span>
          <span className={`font-medium ${healthColor}`}>
            {service.health}
          </span>
        </div>
        {service.uptime && (
          <div className="flex justify-between">
            <span className="text-gray-500">Uptime</span>
            <span className="text-gray-300 font-mono text-xs">{service.uptime}</span>
          </div>
        )}
        {service.id && (
          <div className="flex justify-between">
            <span className="text-gray-500">ID</span>
            <span className="text-gray-400 font-mono text-xs">{service.id}</span>
          </div>
        )}
      </div>
    </div>
  )
}

function OverallBadge({ overall }: { overall: string }) {
  const config: Record<string, { bg: string; text: string; label: string }> = {
    healthy: { bg: 'bg-green-500/10 border-green-500/20', text: 'text-green-400', label: 'All Healthy' },
    degraded: { bg: 'bg-yellow-500/10 border-yellow-500/20', text: 'text-yellow-400', label: 'Degraded' },
    down: { bg: 'bg-red-500/10 border-red-500/20', text: 'text-red-400', label: 'Down' },
    unknown: { bg: 'bg-gray-500/10 border-gray-500/20', text: 'text-gray-400', label: 'Unknown' },
  }
  const c = config[overall] ?? config['unknown']!

  return (
    <span className={`inline-flex items-center gap-1.5 px-3 py-1 rounded-full border text-xs font-medium ${c.bg} ${c.text}`}>
      <span className={`w-1.5 h-1.5 rounded-full ${c.text.replace('text-', 'bg-')} ${overall === 'healthy' ? 'status-pulse' : ''}`} />
      {c.label}
    </span>
  )
}

function EmptyState() {
  return (
    <div className="card text-center py-12">
      <div className="text-4xl mb-4">📦</div>
      <h3 className="text-lg font-semibold mb-2">No Services Found</h3>
      <p className="text-gray-400 text-sm max-w-md mx-auto">
        No services are running yet. Contact your administrator to start the system.
      </p>
    </div>
  )
}

function getStateColor(state: string): string {
  switch (state) {
    case 'running': return 'bg-green-500/50'
    case 'exited':
    case 'dead': return 'bg-red-500/50'
    case 'created':
    case 'restarting': return 'bg-yellow-500/50'
    default: return 'bg-gray-500/50'
  }
}

function getHealthColor(health: string): string {
  switch (health) {
    case 'healthy': return 'text-green-400'
    case 'unhealthy': return 'text-red-400'
    case 'starting': return 'text-yellow-400'
    default: return 'text-gray-400'
  }
}

// ─── Cost Panel ──────────────────────────────────────────────

function costColor(usd: number): string {
  if (usd >= 10) return 'text-red-400'
  if (usd >= 1) return 'text-yellow-400'
  return 'text-green-400'
}

function costBarColor(usd: number): string {
  if (usd >= 10) return 'bg-red-500'
  if (usd >= 1) return 'bg-yellow-500'
  return 'bg-green-500'
}

function formatTokens(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`
  return String(n)
}

function CostPanel({ usage }: { usage: UsageResponse }) {
  const todayCost = usage.todayCostUsd ?? 0
  const periodCost = usage.periodCostUsd ?? 0
  const totals = usage.totals
  const daily = usage.daily ?? []

  // Find max daily cost for bar scaling
  const maxDailyCost = daily.reduce((max, d) => Math.max(max, d.totalCost), 0.01)

  return (
    <div className="card mb-8">
      <div className="flex items-center justify-between mb-4">
        <h3 className="font-semibold text-lg">💰 LLM Usage & Cost</h3>
        {usage.alert && usage.alert !== 'ok' && (
          <span className={`text-xs px-2 py-0.5 rounded-full border ${
            usage.alert === 'critical'
              ? 'border-red-500/30 bg-red-500/10 text-red-400'
              : 'border-yellow-500/30 bg-yellow-500/10 text-yellow-400'
          }`}>
            {usage.alert === 'critical' ? '🔴 High Spend' : '🟡 Elevated'}
          </span>
        )}
      </div>

      {/* Cost summary */}
      <div className="grid grid-cols-2 sm:grid-cols-4 gap-4 mb-6">
        <div>
          <p className="text-xs text-gray-500 mb-1">Today</p>
          <p className={`text-xl font-bold tabular-nums ${costColor(todayCost)}`}>
            ${todayCost.toFixed(2)}
          </p>
        </div>
        <div>
          <p className="text-xs text-gray-500 mb-1">30-Day Total</p>
          <p className="text-xl font-bold tabular-nums text-gray-100">
            ${periodCost.toFixed(2)}
          </p>
        </div>
        {totals && (
          <>
            <div>
              <p className="text-xs text-gray-500 mb-1">Tokens In</p>
              <p className="text-xl font-bold tabular-nums text-gray-100">
                {formatTokens(totals.input)}
              </p>
            </div>
            <div>
              <p className="text-xs text-gray-500 mb-1">Tokens Out</p>
              <p className="text-xl font-bold tabular-nums text-gray-100">
                {formatTokens(totals.output)}
              </p>
            </div>
          </>
        )}
      </div>

      {/* Token breakdown */}
      {totals && (totals.cacheRead > 0 || totals.cacheWrite > 0) && (
        <div className="flex gap-4 mb-6 text-xs text-gray-500">
          <span>Cache Read: {formatTokens(totals.cacheRead)}</span>
          <span>Cache Write: {formatTokens(totals.cacheWrite)}</span>
        </div>
      )}

      {/* Daily cost bars */}
      {daily.length > 0 && (
        <div>
          <p className="text-xs text-gray-500 mb-2">Daily Cost (last {daily.length} days)</p>
          <div className="flex items-end gap-px h-16">
            {daily.slice(-30).map((d) => {
              const pct = Math.max((d.totalCost / maxDailyCost) * 100, 2)
              return (
                <div
                  key={d.date}
                  className={`flex-1 rounded-t-sm ${costBarColor(d.totalCost)} opacity-80 hover:opacity-100 transition-opacity`}
                  style={{ height: `${pct}%` }}
                  title={`${d.date}: $${d.totalCost.toFixed(2)}`}
                />
              )
            })}
          </div>
        </div>
      )}
    </div>
  )
}
