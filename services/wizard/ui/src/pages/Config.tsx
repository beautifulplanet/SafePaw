import { useState, useEffect, useCallback } from 'react'
import { api, type ConfigResponse } from '../api'

/** Keys we show in the form (editable). Order and labels. */
const EDITABLE_KEYS: { key: string; label: string; placeholder: string }[] = [
  { key: 'ANTHROPIC_API_KEY', label: 'Anthropic API key', placeholder: 'sk-ant-...' },
  { key: 'OPENAI_API_KEY', label: 'OpenAI API key (optional)', placeholder: 'sk-...' },
  { key: 'DISCORD_BOT_TOKEN', label: 'Discord bot token', placeholder: '' },
  { key: 'TELEGRAM_BOT_TOKEN', label: 'Telegram bot token', placeholder: '' },
  { key: 'SLACK_BOT_TOKEN', label: 'Slack bot token', placeholder: '' },
  { key: 'SLACK_APP_TOKEN', label: 'Slack app token', placeholder: '' },
  { key: 'WIZARD_ADMIN_PASSWORD', label: 'Wizard admin password', placeholder: 'Set a fixed password' },
  { key: 'AUTH_ENABLED', label: 'Gateway auth enabled', placeholder: 'true or false' },
  { key: 'RATE_LIMIT', label: 'Rate limit (req/min per IP)', placeholder: '60' },
]

function isMasked(value: string): boolean {
  return value === '' || (value.startsWith('***') && value.length <= 8)
}

interface ConfigProps {
  onBack: () => void
}

export function Config({ onBack }: ConfigProps) {
  const [data, setData] = useState<ConfigResponse | null>(null)
  const [edits, setEdits] = useState<Record<string, string>>({})
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [saved, setSaved] = useState(false)

  const fetchConfig = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      const res = await api.getConfig()
      setData(res)
    } catch {
      setError('Failed to load config')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { void fetchConfig() }, [fetchConfig])

  const handleChange = (key: string, value: string) => {
    setEdits((prev) => ({ ...prev, [key]: value }))
    setSaved(false)
  }

  const handleSave = async () => {
    const updates: Record<string, string> = {}
    for (const { key } of EDITABLE_KEYS) {
      const v = edits[key]
      if (v !== undefined && v.trim() !== '') {
        updates[key] = v.trim()
      }
    }
    if (Object.keys(updates).length === 0) {
      setError('No changes to save')
      return
    }
    setSaving(true)
    setError('')
    try {
      await api.putConfig(updates)
      setSaved(true)
      setEdits({})
      await fetchConfig()
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to save config')
    } finally {
      setSaving(false)
    }
  }

  const getDisplayValue = (key: string): string => {
    if (edits[key] !== undefined) return edits[key]
    const v = data?.config?.[key]
    if (v === undefined) return ''
    if (isMasked(v)) return ''
    return v
  }

  const getPlaceholder = (key: string): string => {
    const row = EDITABLE_KEYS.find((r) => r.key === key)
    if (data?.config?.[key] && isMasked(data.config[key])) return '•••••• (set to change)'
    return row?.placeholder ?? ''
  }

  return (
    <div className="max-w-2xl mx-auto">
      <div className="flex items-center justify-between mb-8">
        <div>
          <h2 className="text-2xl font-bold tracking-tight">Configuration</h2>
          <p className="text-gray-400 mt-1">
            Edit .env values. Secrets are masked; enter a new value to change.
          </p>
        </div>
        <button onClick={onBack} className="btn-secondary text-sm py-1.5 px-3">
          Back to Dashboard
        </button>
      </div>

      {error && (
        <div className="rounded-lg bg-red-500/10 border border-red-500/20 px-4 py-3 text-sm text-red-400 mb-6">
          {error}
        </div>
      )}
      {saved && (
        <div className="rounded-lg bg-green-500/10 border border-green-500/20 px-4 py-3 text-sm text-green-400 mb-6">
          Config saved. Restart services if needed for changes to take effect.
        </div>
      )}

      {loading ? (
        <div className="card animate-pulse space-y-4">
          <div className="h-4 bg-gray-800 rounded w-3/4" />
          <div className="h-10 bg-gray-800/50 rounded" />
          <div className="h-10 bg-gray-800/50 rounded" />
        </div>
      ) : (
        <div className="space-y-4">
          {EDITABLE_KEYS.map(({ key, label }) => (
            <div key={key} className="card">
              <label htmlFor={key} className="block text-sm font-medium text-gray-300 mb-1.5">
                {label}
              </label>
              <input
                id={key}
                type={key.includes('PASSWORD') || key.includes('TOKEN') || key.includes('KEY') ? 'password' : 'text'}
                className="input w-full"
                placeholder={getPlaceholder(key)}
                value={getDisplayValue(key)}
                onChange={(e) => handleChange(key, e.target.value)}
                autoComplete="off"
              />
            </div>
          ))}
          <div className="flex justify-end gap-3 pt-4">
            <button onClick={onBack} className="btn-secondary">
              Cancel
            </button>
            <button onClick={handleSave} disabled={saving} className="btn-primary">
              {saving ? 'Saving…' : 'Save'}
            </button>
          </div>
        </div>
      )}

      <p className="mt-6 text-xs text-gray-500">
        Only allowed keys are updated. Infrastructure (e.g. POSTGRES_PASSWORD) cannot be changed here.
      </p>
    </div>
  )
}
