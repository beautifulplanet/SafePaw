import { useState, useEffect, useCallback, useRef } from 'react'
import { api, type ServiceInfo, type StatusResponse, type GatewayMetrics, type UsageResponse, type CostHistoryResponse, type CostModelsResponse, type CostTrendsResponse } from '../api'

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
  const [costHistory, setCostHistory] = useState<CostHistoryResponse | null>(null)
  const [costModels, setCostModels] = useState<CostModelsResponse | null>(null)
  const [costTrends, setCostTrends] = useState<CostTrendsResponse | null>(null)
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

  const fetchCostAnalytics = useCallback(async () => {
    try {
      const [history, models, trends] = await Promise.all([
        api.costHistory(30),
        api.costModels(30),
        api.costTrends(7),
      ])
      setCostHistory(history)
      setCostModels(models)
      setCostTrends(trends)
    } catch {
      // Cost analytics non-critical
    }
  }, [])

  const handleOpenAssistant = useCallback(async () => {
    setOpeningAssistant(true)
    try {
      const { token } = await api.gatewayToken('wizard-user', 'proxy', 1)
      // Build gateway URL — handle Codespaces forwarded ports (port is in subdomain)
      const host = window.location.host
      let gatewayBase: string
      if (host.includes('.app.github.dev')) {
        // Codespaces: replace port in subdomain (e.g. -3000. → -8080.)
        gatewayBase = `${window.location.protocol}//${host.replace(/-\d+\.app\.github\.dev/, '-8080.app.github.dev')}`
      } else {
        gatewayBase = `${window.location.protocol}//${window.location.hostname}:8080`
      }
      const url = `${gatewayBase}/?token=${encodeURIComponent(token)}`
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
    void fetchCostAnalytics()
    intervalRef.current = setInterval(() => {
      void fetchStatus()
      void fetchMetrics()
    }, POLL_INTERVAL)
    usageIntervalRef.current = setInterval(() => {
      void fetchUsage()
      void fetchCostAnalytics()
    }, 60000)
    return () => {
      clearInterval(intervalRef.current)
      clearInterval(usageIntervalRef.current)
    }
  }, [fetchStatus, fetchMetrics, fetchUsage, fetchCostAnalytics])

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

      {/* Getting Started (show on fresh installs when no conversations yet) */}
      {metrics && metrics.total_requests === 0 && (
        <GettingStartedCard onChat={handleOpenAssistant} onOpenSettings={onOpenSettings} />
      )}

      {/* No-API-key banner (usage unavailable but system running) */}
      {usage && usage.status !== 'ok' && data && data.overall !== 'down' && (
        <div className="rounded-lg bg-yellow-500/10 border border-yellow-500/20 px-4 py-3 text-sm text-yellow-400 mb-6">
          💡 Connect an AI provider in Settings to start chatting and see usage data.
        </div>
      )}

      {/* Cost Analytics (Postgres-backed history) */}
      {costTrends?.status === 'ok' && (
        <CostAnalyticsPanel history={costHistory} models={costModels} trends={costTrends} />
      )}

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

function GettingStartedCard({ onChat, onOpenSettings }: { onChat: () => void; onOpenSettings?: () => void }) {
  return (
    <div className="card mb-8 card-enter" style={{ animationDelay: '60ms' }}>
      <div className="flex items-center gap-3 mb-4">
        <span className="text-2xl">🚀</span>
        <h3 className="font-semibold text-lg">Getting Started</h3>
      </div>
      <p className="text-gray-400 text-sm mb-5">
        Your AI is up and running! Here are 3 things to try:
      </p>
      <div className="grid sm:grid-cols-3 gap-4">
        <button onClick={onChat} className="text-left p-4 rounded-lg bg-gray-800/50 border border-gray-700 hover:border-paw-600/50 transition-colors group">
          <div className="text-lg mb-2">💬</div>
          <p className="text-sm font-medium text-gray-200 group-hover:text-paw-400 transition-colors">Send a message</p>
          <p className="text-xs text-gray-500 mt-1">Open the chat and talk to your AI</p>
        </button>
        {onOpenSettings && (
          <button onClick={onOpenSettings} className="text-left p-4 rounded-lg bg-gray-800/50 border border-gray-700 hover:border-paw-600/50 transition-colors group">
            <div className="text-lg mb-2">🔌</div>
            <p className="text-sm font-medium text-gray-200 group-hover:text-paw-400 transition-colors">Connect a channel</p>
            <p className="text-xs text-gray-500 mt-1">Add Discord, Telegram, or Slack</p>
          </button>
        )}
        <div className="p-4 rounded-lg bg-gray-800/50 border border-gray-700">
          <div className="text-lg mb-2">👥</div>
          <p className="text-sm font-medium text-gray-200">Invite a teammate</p>
          <p className="text-xs text-gray-500 mt-1">Share the gateway URL + a token</p>
        </div>
      </div>
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

// ─── Cost Analytics Panel (Postgres-backed) ──────────────────

function trendArrow(pct: number): string {
  if (pct > 10) return '↑'
  if (pct < -10) return '↓'
  return '→'
}

function trendColor(pct: number): string {
  if (pct > 50) return 'text-red-400'
  if (pct > 10) return 'text-yellow-400'
  if (pct < -10) return 'text-green-400'
  return 'text-gray-400'
}

function anomalyBadge(score: number): { label: string; cls: string } | null {
  if (score >= 0.7) return { label: '⚠ Anomaly', cls: 'border-red-500/30 bg-red-500/10 text-red-400' }
  if (score >= 0.4) return { label: 'Unusual', cls: 'border-yellow-500/30 bg-yellow-500/10 text-yellow-400' }
  return null
}

interface ModelAggregate {
  provider: string
  model: string
  totalCost: number
  totalTokens: number
  requestCount: number
}

function aggregateModels(models: CostModelsResponse | null): ModelAggregate[] {
  if (!models?.models?.length) return []
  const map = new Map<string, ModelAggregate>()
  for (const m of models.models) {
    const key = `${m.provider}/${m.model}`
    const agg = map.get(key) ?? { provider: m.provider, model: m.model, totalCost: 0, totalTokens: 0, requestCount: 0 }
    agg.totalCost += m.totalCost
    agg.totalTokens += m.totalTokens
    agg.requestCount += m.requestCount
    map.set(key, agg)
  }
  return [...map.values()].sort((a, b) => b.totalCost - a.totalCost)
}

function CostAnalyticsPanel({ history, models, trends }: {
  history: CostHistoryResponse | null
  models: CostModelsResponse | null
  trends: CostTrendsResponse | null
}) {
  const trend = trends?.trend
  const daily = history?.daily ?? []
  const modelAgg = aggregateModels(models)
  const maxBarCost = daily.reduce((m, d) => Math.max(m, d.totalCost), 0.01)
  const maxModelCost = modelAgg.length > 0 ? modelAgg[0]!.totalCost : 1

  return (
    <div className="card mb-8 card-enter" style={{ animationDelay: '120ms' }}>
      <div className="flex items-center justify-between mb-4">
        <h3 className="font-semibold text-lg">📊 Cost Analytics</h3>
        {trend && anomalyBadge(trend.anomalyScore) && (
          <span className={`text-xs px-2 py-0.5 rounded-full border ${anomalyBadge(trend.anomalyScore)!.cls}`}>
            {anomalyBadge(trend.anomalyScore)!.label}
          </span>
        )}
      </div>

      {/* Trend summary cards */}
      {trend && (
        <div className="grid grid-cols-2 sm:grid-cols-4 gap-4 mb-6">
          <div>
            <p className="text-xs text-gray-500 mb-1">Last {trend.recentDays}d</p>
            <p className="text-xl font-bold tabular-nums text-gray-100">
              ${trend.recentCost.toFixed(2)}
            </p>
          </div>
          <div>
            <p className="text-xs text-gray-500 mb-1">Prior {trend.recentDays}d</p>
            <p className="text-xl font-bold tabular-nums text-gray-100">
              ${trend.priorCost.toFixed(2)}
            </p>
          </div>
          <div>
            <p className="text-xs text-gray-500 mb-1">Cost Change</p>
            <p className={`text-xl font-bold tabular-nums ${trendColor(trend.costChangePct)}`}>
              {trendArrow(trend.costChangePct)} {trend.costChangePct >= 0 ? '+' : ''}{trend.costChangePct.toFixed(1)}%
            </p>
          </div>
          <div>
            <p className="text-xs text-gray-500 mb-1">Daily Avg</p>
            <p className="text-xl font-bold tabular-nums text-gray-100">
              ${trend.dailyAvgRecent.toFixed(2)}
            </p>
            {trend.dailyAvgPrior > 0 && (
              <p className="text-[10px] text-gray-600">was ${trend.dailyAvgPrior.toFixed(2)}</p>
            )}
          </div>
        </div>
      )}

      {/* Historical daily chart */}
      {daily.length > 0 && (
        <div className="mb-6">
          <p className="text-xs text-gray-500 mb-2">Daily Cost History (last {daily.length} days)</p>
          <div className="flex items-end gap-px h-20">
            {daily.slice().reverse().map((d) => {
              const pct = Math.max((d.totalCost / maxBarCost) * 100, 2)
              return (
                <div
                  key={d.date}
                  className={`flex-1 rounded-t-sm ${costBarColor(d.totalCost)} opacity-80 hover:opacity-100 transition-opacity`}
                  style={{ height: `${pct}%` }}
                  title={`${d.date}: $${d.totalCost.toFixed(2)} · ${formatTokens(d.totalTokens)} tokens · ${d.messages} msgs`}
                />
              )
            })}
          </div>
          <div className="flex justify-between mt-1">
            <span className="text-[10px] text-gray-600">{daily[daily.length - 1]?.date}</span>
            <span className="text-[10px] text-gray-600">{daily[0]?.date}</span>
          </div>
        </div>
      )}

      {/* Per-model breakdown */}
      {modelAgg.length > 0 && (
        <div>
          <p className="text-xs text-gray-500 mb-2">Cost by Model</p>
          <div className="space-y-2">
            {modelAgg.slice(0, 8).map((m) => {
              const pct = (m.totalCost / maxModelCost) * 100
              return (
                <div key={`${m.provider}/${m.model}`}>
                  <div className="flex items-center justify-between text-xs mb-0.5">
                    <span className="text-gray-300 truncate max-w-[60%]" title={`${m.provider}/${m.model}`}>
                      {m.model}
                    </span>
                    <span className="text-gray-400 tabular-nums">
                      ${m.totalCost.toFixed(2)} · {formatTokens(m.totalTokens)} · {m.requestCount} req
                    </span>
                  </div>
                  <div className="h-1.5 bg-gray-800 rounded-full overflow-hidden">
                    <div
                      className={`h-full rounded-full ${costBarColor(m.totalCost)} transition-all`}
                      style={{ width: `${Math.max(pct, 1)}%` }}
                    />
                  </div>
                </div>
              )
            })}
          </div>
        </div>
      )}
    </div>
  )
}
