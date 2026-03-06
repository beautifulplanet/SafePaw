import { useState, useEffect, useCallback, useRef } from 'react'
import { api, type GatewayActivity } from '../api'

const POLL_INTERVAL = 5000

interface ActivityProps {
  onBack?: () => void
}

export function Activity(_props: ActivityProps) {
  const [data, setData] = useState<GatewayActivity | null>(null)
  const [error, setError] = useState('')
  const intervalRef = useRef<ReturnType<typeof setInterval>>(undefined)

  const fetchActivity = useCallback(async () => {
    try {
      const res = await api.gatewayActivity()
      setData(res)
      setError('')
    } catch {
      setError('Failed to fetch activity data')
    }
  }, [])

  useEffect(() => {
    void fetchActivity()
    intervalRef.current = setInterval(() => void fetchActivity(), POLL_INTERVAL)
    return () => clearInterval(intervalRef.current)
  }, [fetchActivity])

  return (
    <div>
      <div className="flex items-center justify-between mb-8">
        <div>
          <h2 className="text-2xl font-bold tracking-tight">Security Monitor</h2>
          <p className="text-gray-400 mt-1">
            Live view of who's using your AI and what's being blocked. Updates every 5 seconds.
          </p>
        </div>
        <button onClick={fetchActivity} className="btn-secondary text-sm py-1.5 px-3">
          Refresh
        </button>
      </div>

      {error && (
        <div className="rounded-lg bg-red-500/10 border border-red-500/20 px-4 py-3 text-sm text-red-400 mb-6">
          {error}
        </div>
      )}

      {!data ? (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
          {Array.from({ length: 4 }).map((_, i) => (
            <div key={i} className="card animate-pulse">
              <div className="h-3 bg-gray-800 rounded w-20 mb-3" />
              <div className="h-6 bg-gray-800/50 rounded w-16" />
            </div>
          ))}
        </div>
      ) : !data.metrics.gateway_reachable ? (
        <div className="card text-center py-12">
          <div className="text-4xl mb-4">🔌</div>
          <h3 className="text-lg font-semibold mb-2">Security Shield Offline</h3>
          <p className="text-gray-400 text-sm max-w-md mx-auto">
            Can't connect to the security layer. This usually means it's still starting up or was restarted. Try refreshing in a few seconds.
          </p>
        </div>
      ) : (
        <>
          {/* Metric cards */}
          <div className="grid grid-cols-2 sm:grid-cols-4 gap-4 mb-8">
            <MetricCard index={0} label="Total Conversations" value={data.metrics.total_requests} hint="Every message sent to the AI" />
            <MetricCard index={1} label="People Online" value={data.metrics.active_connections} hint="Users chatting right now" />
            <MetricCard index={2} label="Failed Logins" value={data.metrics.auth_failures} warn={data.metrics.auth_failures > 0} hint="Someone tried to access without permission" />
            <MetricCard index={3} label="Attacks Stopped" value={data.metrics.injections_found} warn={data.metrics.injections_found > 0} hint="Malicious messages caught before reaching the AI" />
          </div>

          <div className="grid grid-cols-2 sm:grid-cols-3 gap-4 mb-8">
            <MetricCard index={4} label="Spam Blocked" value={data.metrics.rate_limited} warn={data.metrics.rate_limited > 0} hint="Requests blocked because someone sent too many too fast" />
            <MetricCard index={5} label="Access Revoked" value={data.metrics.tokens_revoked} hint="Users whose access was manually cut off" />
            <MetricCard index={6} label="Speed" value={`${data.metrics.avg_response_ms.toFixed(1)}ms`} hint="How fast the AI responds on average" />
          </div>

          {/* Top paths */}
          {data.top_paths && data.top_paths.length > 0 && (
            <div className="card">
              <h3 className="font-semibold mb-1">Most Used Features</h3>
              <p className="text-xs text-gray-500 mb-4">Which parts of the AI are getting the most traffic.</p>
              <div className="space-y-2">
                {data.top_paths.map((p, i) => {
                  const maxCount = data.top_paths[0]?.count ?? 1
                  const pct = maxCount > 0 ? (p.count / maxCount) * 100 : 0
                  return (
                    <div key={i} className="flex items-center gap-3">
                      <code className="text-sm text-gray-300 font-mono w-48 truncate shrink-0">{p.path}</code>
                      <div className="flex-1 h-5 bg-gray-800 rounded overflow-hidden">
                        <div
                          className="h-full bg-paw-600/50 rounded transition-all duration-500"
                          style={{ width: `${pct}%` }}
                        />
                      </div>
                      <span className="text-sm text-gray-400 tabular-nums w-16 text-right">
                        {p.count.toLocaleString()}
                      </span>
                    </div>
                  )
                })}
              </div>
            </div>
          )}
        </>
      )}
    </div>
  )
}

function MetricCard({ index, label, value, warn, hint }: { index: number; label: string; value: number | string; warn?: boolean; hint?: string }) {
  return (
    <div className="card py-3 px-4 card-enter" style={{ animationDelay: `${index * 60}ms` }} title={hint}>
      <p className="text-xs text-gray-500 mb-1">{label}</p>
      <p className={`text-2xl font-bold tabular-nums ${warn ? 'text-yellow-400' : 'text-gray-100'}`}>
        {typeof value === 'number' ? value.toLocaleString() : value}
      </p>
      {hint && <p className="text-[10px] text-gray-600 mt-1 leading-tight">{hint}</p>}
    </div>
  )
}
