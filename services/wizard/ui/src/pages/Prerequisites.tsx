import { useState, useEffect, useCallback } from 'react'
import { api, type PrerequisiteCheck } from '../api'

interface PrerequisitesProps {
  onContinue: () => void
}

export function Prerequisites({ onContinue }: PrerequisitesProps) {
  const [checks, setChecks] = useState<PrerequisiteCheck[]>([])
  const [allPass, setAllPass] = useState(false)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const runChecks = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      const res = await api.prerequisites()
      setChecks(res.checks)
      setAllPass(res.all_pass)
    } catch {
      setError('Failed to check prerequisites — is the wizard server running?')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { void runChecks() }, [runChecks])

  return (
    <div className="max-w-2xl mx-auto">
      <div className="mb-8">
        <h2 className="text-2xl font-bold tracking-tight">System Prerequisites</h2>
        <p className="text-gray-400 mt-2">
          SafePaw needs a few things before it can set up your private AI.
        </p>
      </div>

      {error && (
        <div className="rounded-lg bg-red-500/10 border border-red-500/20 px-4 py-3 text-sm text-red-400 mb-6">
          {error}
        </div>
      )}

      <div className="space-y-3">
        {loading && checks.length === 0 ? (
          // Skeleton loading
          Array.from({ length: 4 }).map((_, i) => (
            <div key={i} className="card animate-pulse">
              <div className="flex items-center gap-4">
                <div className="w-6 h-6 rounded-full bg-gray-800" />
                <div className="flex-1">
                  <div className="h-4 bg-gray-800 rounded w-32 mb-2" />
                  <div className="h-3 bg-gray-800/50 rounded w-48" />
                </div>
              </div>
            </div>
          ))
        ) : (
          checks.map((check) => (
            <CheckItem key={check.name} check={check} />
          ))
        )}
      </div>

      <div className="flex items-center justify-between mt-8">
        <button onClick={runChecks} className="btn-secondary" disabled={loading}>
          {loading ? 'Checking…' : 'Re-check'}
        </button>
        <button
          onClick={onContinue}
          className="btn-primary"
          disabled={!allPass}
        >
          {allPass ? 'Continue to Dashboard →' : 'Fix Issues to Continue'}
        </button>
      </div>
    </div>
  )
}

function CheckItem({ check }: { check: PrerequisiteCheck }) {
  const icon = statusIcon(check.status)
  const color = statusColor(check.status)

  return (
    <div className="card flex items-start gap-4">
      <div className={`mt-0.5 w-6 h-6 rounded-full flex items-center justify-center text-sm ${color}`}>
        {icon}
      </div>
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-2">
          <h3 className="font-medium">{check.name}</h3>
          {check.required && (
            <span className="text-[10px] uppercase tracking-wider font-medium text-gray-500 bg-gray-800 px-1.5 py-0.5 rounded">
              Required
            </span>
          )}
        </div>
        <p className="text-sm text-gray-400 mt-0.5">{check.message}</p>
        {check.status === 'fail' && check.help_url && (
          <a
            href={check.help_url}
            target="_blank"
            rel="noopener noreferrer"
            className="inline-flex items-center gap-1 text-xs text-paw-400 hover:text-paw-300 mt-2 transition-colors"
          >
            View installation guide
            <svg className="w-3 h-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14" />
            </svg>
          </a>
        )}
      </div>
    </div>
  )
}

function statusIcon(status: string): string {
  switch (status) {
    case 'pass': return '✓'
    case 'fail': return '✗'
    case 'warn': return '!'
    default: return '?'
  }
}

function statusColor(status: string): string {
  switch (status) {
    case 'pass': return 'bg-green-500/20 text-green-400'
    case 'fail': return 'bg-red-500/20 text-red-400'
    case 'warn': return 'bg-yellow-500/20 text-yellow-400'
    default: return 'bg-gray-500/20 text-gray-400'
  }
}
