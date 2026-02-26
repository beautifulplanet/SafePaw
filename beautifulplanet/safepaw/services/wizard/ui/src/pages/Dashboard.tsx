import { useState, useEffect, useCallback, useRef } from 'react'
import { api, type ServiceInfo, type StatusResponse } from '../api'

const POLL_INTERVAL = 5000 // ms

export function Dashboard() {
  const [data, setData] = useState<StatusResponse | null>(null)
  const [error, setError] = useState('')
  const intervalRef = useRef<ReturnType<typeof setInterval>>(undefined)

  const fetchStatus = useCallback(async () => {
    try {
      const res = await api.status()
      setData(res)
      setError('')
    } catch {
      setError('Failed to fetch service status')
    }
  }, [])

  useEffect(() => {
    void fetchStatus()
    intervalRef.current = setInterval(() => void fetchStatus(), POLL_INTERVAL)
    return () => clearInterval(intervalRef.current)
  }, [fetchStatus])

  return (
    <div>
      <div className="flex items-center justify-between mb-8">
        <div>
          <h2 className="text-2xl font-bold tracking-tight">Service Dashboard</h2>
          <p className="text-gray-400 mt-1">
            Live status of your SafePaw deployment.
          </p>
        </div>
        <div className="flex items-center gap-3">
          {data && <OverallBadge overall={data.overall} />}
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
          {data.services.map((svc) => (
            <ServiceCard key={svc.name || svc.id} service={svc} />
          ))}
        </div>
      )}

      {/* Architecture info */}
      <div className="mt-10 card">
        <h3 className="font-semibold mb-3">Architecture</h3>
        <div className="grid sm:grid-cols-3 gap-4 text-sm">
          <div className="space-y-1">
            <p className="text-gray-500">Gateway</p>
            <p className="text-gray-300">Go reverse proxy with rate limiting, auth, and AI input sanitization</p>
          </div>
          <div className="space-y-1">
            <p className="text-gray-500">OpenClaw</p>
            <p className="text-gray-300">Node.js AI assistant (internal, not exposed to host)</p>
          </div>
          <div className="space-y-1">
            <p className="text-gray-500">Data Layer</p>
            <p className="text-gray-300">Redis (state cache) + Postgres (auth, sessions)</p>
          </div>
        </div>
      </div>
    </div>
  )
}

function ServiceCard({ service }: { service: ServiceInfo }) {
  const stateColor = getStateColor(service.state)
  const healthColor = getHealthColor(service.health)

  return (
    <div className="card group hover:border-gray-700 transition-colors">
      {/* Header */}
      <div className="flex items-center justify-between mb-3">
        <h3 className="font-semibold text-lg">{service.name || 'unknown'}</h3>
        <div className={`w-2.5 h-2.5 rounded-full ${stateColor} ${service.state === 'running' ? 'status-pulse' : ''}`} />
      </div>

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
        No SafePaw containers are running yet. Run{' '}
        <code className="px-1.5 py-0.5 rounded bg-gray-800 text-paw-400 font-mono text-xs">
          docker compose up -d
        </code>{' '}
        to start the deployment.
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
