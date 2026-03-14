import { useState, useEffect, useCallback } from 'react'
import { api, type ConfigResponse } from '../api'

// Settings are organized into sections for clarity
const SECTIONS: { title: string; description: string; keys: { key: string; label: string; placeholder: string; help?: string; type?: 'password' | 'text' | 'toggle' | 'profile' }[] }[] = [
  {
    title: '⚡ System Size',
    description: 'How much RAM does this server have? This controls memory allocation for all services.',
    keys: [
      { key: 'SYSTEM_PROFILE', label: 'Server Profile', placeholder: 'small', type: 'profile', help: 'Pick the closest match. You can always change this later — just restart services from the Home page.' },
    ],
  },
  {
    title: '🤖 AI Provider',
    description: 'Which AI service powers your assistant. You need at least one key.',
    keys: [
      { key: 'ANTHROPIC_API_KEY', label: 'Anthropic (Claude) Key', placeholder: 'sk-ant-...', type: 'password', help: 'Get this from console.anthropic.com — it\'s like a password that lets your system talk to Claude.' },
      { key: 'OPENAI_API_KEY', label: 'OpenAI (ChatGPT) Key', placeholder: 'sk-...', type: 'password', help: 'Get this from platform.openai.com — only needed if you prefer GPT over Claude.' },
    ],
  },
  {
    title: '🔒 Security',
    description: 'Who can access your AI and how they prove they\'re allowed.',
    keys: [
      { key: 'AUTH_ENABLED', label: 'Require Login', placeholder: 'true', type: 'toggle', help: 'When ON, users need a token (like a password) to use the AI. Strongly recommended.' },
      { key: 'AUTH_SECRET', label: 'Secret Key', placeholder: 'min 32 characters', type: 'password', help: 'A long random string used to create login tokens. Like a master key — keep it safe.' },
      { key: 'WIZARD_ADMIN_PASSWORD', label: 'Admin Panel Password', placeholder: 'Set a strong password', type: 'password', help: 'Full access: config, tokens, restart. One admin account.' },
      { key: 'WIZARD_OPERATOR_PASSWORD', label: 'Operator Password (Optional)', placeholder: 'Leave empty to disable', type: 'password', help: 'Operator can view and restart services; cannot change config or issue tokens.' },
      { key: 'WIZARD_VIEWER_PASSWORD', label: 'Viewer Password (Optional)', placeholder: 'Leave empty to disable', type: 'password', help: 'Viewer can only see dashboard and activity; no Settings or Restart.' },
      { key: 'WIZARD_TOTP_SECRET', label: 'Two-Factor Code (Optional)', placeholder: 'base32 secret', type: 'password', help: 'Adds a 6-digit code from an authenticator app on top of your password (admin and operator only).' },
    ],
  },
  {
    title: '🚦 Spam Protection',
    description: 'Prevent anyone from flooding the AI with too many messages.',
    keys: [
      { key: 'RATE_LIMIT', label: 'Max messages per window', placeholder: '60', help: 'How many messages one person can send before being temporarily blocked. Default: 60.' },
      { key: 'RATE_LIMIT_WINDOW_SEC', label: 'Window (seconds)', placeholder: '60', help: 'The time period for counting messages. Default: 60 seconds.' },
    ],
  },
  {
    title: '🔐 Encryption (HTTPS)',
    description: 'Encrypt traffic between users and the AI. Required if accessed over the internet.',
    keys: [
      { key: 'TLS_ENABLED', label: 'Enable HTTPS', placeholder: 'false', type: 'toggle' },
      { key: 'TLS_CERT_FILE', label: 'Certificate File Path', placeholder: '/certs/cert.pem' },
      { key: 'TLS_KEY_FILE', label: 'Private Key File Path', placeholder: '/certs/key.pem' },
    ],
  },
  {
    title: '💬 Messaging Channels',
    description: 'Connect the AI to chat platforms so your team can use it from Slack, Discord, or Telegram.',
    keys: [
      { key: 'DISCORD_BOT_TOKEN', label: 'Discord Bot Token', placeholder: '', type: 'password' },
      { key: 'TELEGRAM_BOT_TOKEN', label: 'Telegram Bot Token', placeholder: '', type: 'password' },
      { key: 'SLACK_BOT_TOKEN', label: 'Slack Bot Token', placeholder: '', type: 'password' },
      { key: 'SLACK_APP_TOKEN', label: 'Slack App Token', placeholder: '', type: 'password' },
    ],
  },
]

function isMasked(value: string): boolean {
  return value === '' || (value.startsWith('***') && value.length <= 8)
}

interface SettingsProps {
  onBack: () => void
}

export function Settings(_props: SettingsProps) {
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
      setError('Failed to load settings')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { void fetchConfig() }, [fetchConfig])

  const handleChange = (key: string, value: string) => {
    setEdits(prev => ({ ...prev, [key]: value }))
    setSaved(false)
  }

  const handleToggle = (key: string) => {
    const current = getDisplayValue(key)
    const newVal = current === 'true' ? 'false' : 'true'
    handleChange(key, newVal)
  }

  const handleSave = async () => {
    const updates: Record<string, string> = {}
    for (const section of SECTIONS) {
      for (const { key } of section.keys) {
        const v = edits[key]
        if (v !== undefined && v.trim() !== '') {
          updates[key] = v.trim()
        }
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
      setError(e instanceof Error ? e.message : 'Failed to save settings')
    } finally {
      setSaving(false)
    }
  }

  const handleCancel = () => {
    setEdits({})
    setSaved(false)
    setError('')
  }

  const getDisplayValue = (key: string): string => {
    if (edits[key] !== undefined) return edits[key]
    const v = data?.config?.[key]
    if (v === undefined) return ''
    if (isMasked(v)) return ''
    return v
  }

  const getPlaceholder = (key: string): string => {
    const row = SECTIONS.flatMap(s => s.keys).find(r => r.key === key)
    if (data?.config?.[key] && isMasked(data.config[key])) return '•••••• (set to change)'
    return row?.placeholder ?? ''
  }

  const hasChanges = Object.keys(edits).length > 0

  return (
    <div className="max-w-2xl mx-auto">
      <div className="flex items-center justify-between mb-8">
        <div>
          <h2 className="text-2xl font-bold tracking-tight">Settings</h2>
          <p className="text-gray-400 mt-1">
            Configure how your AI works, who can access it, and where it shows up.
          </p>
        </div>
      </div>

      {error && (
        <div className="rounded-lg bg-red-500/10 border border-red-500/20 px-4 py-3 text-sm text-red-400 mb-6">
          {error}
        </div>
      )}
      {saved && (
        <div className="rounded-lg bg-green-500/10 border border-green-500/20 px-4 py-3 text-sm text-green-400 mb-6">
          Settings saved. You may need to restart services from the Home page for changes to take effect.
        </div>
      )}

      {loading ? (
        <div className="space-y-6">
          {[1, 2, 3].map(i => (
            <div key={i} className="card animate-pulse space-y-4">
              <div className="h-5 bg-gray-800 rounded w-32" />
              <div className="h-10 bg-gray-800/50 rounded" />
              <div className="h-10 bg-gray-800/50 rounded" />
            </div>
          ))}
        </div>
      ) : (
        <div className="space-y-8">
          {SECTIONS.map(section => (
            <div key={section.title} className="card">
              <h3 className="font-semibold text-lg mb-1">{section.title}</h3>
              <p className="text-sm text-gray-500 mb-4">{section.description}</p>
              <div className="space-y-4">
                {section.keys.map(({ key, label, help, type }) => (
                  <div key={key}>
                    {type === 'profile' ? (
                      <div>
                        <label htmlFor={key} className="block text-sm font-medium text-gray-300 mb-1">{label}</label>
                        {help && <p className="text-xs text-gray-500 mb-2">{help}</p>}
                        <div className="grid grid-cols-2 gap-3">
                          {([
                            { value: 'small', label: 'Small', ram: '4–8 GB', desc: 'Lightweight — 7B models', emoji: '🟢' },
                            { value: 'medium', label: 'Medium', ram: '16–32 GB', desc: 'Balanced — 13-30B models', emoji: '🔵' },
                            { value: 'large', label: 'Large', ram: '64–128 GB', desc: 'Powerful — 70B models', emoji: '🟣' },
                            { value: 'very-large', label: 'Very Large', ram: '128+ GB', desc: 'Maximum — 100B+ models', emoji: '🔴' },
                          ] as const).map(opt => {
                            const selected = (getDisplayValue(key) || 'small') === opt.value
                            return (
                              <button
                                key={opt.value}
                                onClick={() => handleChange(key, opt.value)}
                                className={`text-left rounded-lg border-2 p-3 transition-all ${
                                  selected
                                    ? 'border-paw-500 bg-paw-500/10'
                                    : 'border-gray-700 bg-gray-800/50 hover:border-gray-600'
                                }`}
                              >
                                <div className="flex items-center gap-2 mb-1">
                                  <span>{opt.emoji}</span>
                                  <span className="font-medium text-sm">{opt.label}</span>
                                  {selected && <span className="ml-auto text-paw-400 text-xs">Active</span>}
                                </div>
                                <div className="text-xs text-gray-400">{opt.ram} RAM</div>
                                <div className="text-xs text-gray-500">{opt.desc}</div>
                              </button>
                            )
                          })}
                        </div>
                      </div>
                    ) : type === 'toggle' ? (
                      <div className="flex items-center justify-between">
                        <div>
                          <label className="text-sm font-medium text-gray-300">{label}</label>
                          {help && <p className="text-xs text-gray-500 mt-0.5">{help}</p>}
                        </div>
                        <button
                          onClick={() => handleToggle(key)}
                          className={`relative w-11 h-6 rounded-full transition-colors ${
                            getDisplayValue(key) === 'true' ? 'bg-paw-600' : 'bg-gray-600'
                          }`}
                        >
                          <span
                            className={`absolute top-0.5 left-0.5 w-5 h-5 rounded-full bg-white transition-transform ${
                              getDisplayValue(key) === 'true' ? 'translate-x-5' : ''
                            }`}
                          />
                        </button>
                      </div>
                    ) : (
                      <>
                        <label htmlFor={key} className="block text-sm font-medium text-gray-300 mb-1">
                          {label}
                        </label>
                        {help && <p className="text-xs text-gray-500 mb-1.5">{help}</p>}
                        <input
                          id={key}
                          type={type === 'password' ? 'password' : 'text'}
                          className="input w-full"
                          placeholder={getPlaceholder(key)}
                          value={getDisplayValue(key)}
                          onChange={e => handleChange(key, e.target.value)}
                          autoComplete="off"
                        />
                      </>
                    )}
                  </div>
                ))}
              </div>
            </div>
          ))}

          {/* Save bar */}
          <div className="flex justify-end gap-3 pt-2">
            {hasChanges && (
              <button onClick={handleCancel} className="btn-secondary">
                Discard Changes
              </button>
            )}
            <button onClick={handleSave} disabled={saving || !hasChanges} className="btn-primary">
              {saving ? 'Saving…' : 'Save Settings'}
            </button>
          </div>
        </div>
      )}

      <p className="mt-6 text-xs text-gray-500">
        Database and cache passwords can't be changed here to prevent accidentally breaking things.
      </p>
    </div>
  )
}
