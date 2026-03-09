import { useState, useCallback, useEffect } from 'react'
import { api } from '../api'

interface SetupProps {
  onComplete: () => void
}

type Step = 'welcome' | 'apikey' | 'security' | 'done'

const STEPS: { id: Step; label: string }[] = [
  { id: 'welcome', label: 'Welcome' },
  { id: 'apikey', label: 'AI Model' },
  { id: 'security', label: 'Security' },
  { id: 'done', label: 'Ready' },
]

export function Setup({ onComplete }: SetupProps) {
  const [step, setStep] = useState<Step>('welcome')
  const [apiKey, setApiKey] = useState('')
  const [apiProvider, setApiProvider] = useState<'anthropic' | 'openai'>('anthropic')
  const [authEnabled, setAuthEnabled] = useState(true)
  const [authSecret, setAuthSecret] = useState('')
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [hasExistingKey, setHasExistingKey] = useState(false)
  const [hasExistingAuth, setHasExistingAuth] = useState(false)

  // Check existing config on mount — auto-fill / auto-skip steps
  useEffect(() => {
    void (async () => {
      try {
        const { config } = await api.getConfig()
        // API keys are masked but present if value is non-empty
        if (config['ANTHROPIC_API_KEY'] || config['OPENAI_API_KEY']) {
          setHasExistingKey(true)
        }
        if (config['AUTH_SECRET'] && config['AUTH_SECRET'] !== 'CHANGE_ME_run_openssl_rand_base64_48') {
          setHasExistingAuth(true)
        }
      } catch {
        // Config not available — proceed normally
      }
    })()
  }, [])

  const currentIndex = STEPS.findIndex(s => s.id === step)

  // Navigate from welcome — skip API key step if already configured
  const handleWelcomeContinue = useCallback(() => {
    if (hasExistingKey) {
      setStep('security')
    } else {
      setStep('apikey')
    }
  }, [hasExistingKey])

  const handleSaveApiKey = useCallback(async () => {
    if (!apiKey.trim()) {
      setError('Please enter an API key')
      return
    }
    setSaving(true)
    setError('')
    try {
      const key = apiProvider === 'anthropic' ? 'ANTHROPIC_API_KEY' : 'OPENAI_API_KEY'
      await api.putConfig({ [key]: apiKey.trim() })
      setStep('security')
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to save API key')
    } finally {
      setSaving(false)
    }
  }, [apiKey, apiProvider])

  const handleSaveSecurity = useCallback(async () => {
    setSaving(true)
    setError('')
    try {
      const updates: Record<string, string> = {
        AUTH_ENABLED: authEnabled ? 'true' : 'false',
      }
      if (authEnabled && authSecret.trim()) {
        updates.AUTH_SECRET = authSecret.trim()
      }
      await api.putConfig(updates)
      setStep('done')
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to save security settings')
    } finally {
      setSaving(false)
    }
  }, [authEnabled, authSecret])

  return (
    <div className="flex items-center justify-center min-h-[calc(100vh-12rem)]">
      <div className="w-full max-w-lg">
        {/* Progress bar */}
        <div className="flex items-center gap-2 mb-8">
          {STEPS.map((s, i) => (
            <div key={s.id} className="flex items-center gap-2 flex-1">
              <div className={`flex items-center justify-center w-8 h-8 rounded-full text-sm font-medium transition-colors ${
                i < currentIndex ? 'bg-paw-600 text-white' :
                i === currentIndex ? 'bg-paw-600/20 border-2 border-paw-500 text-paw-400' :
                'bg-gray-800 text-gray-500'
              }`}>
                {i < currentIndex ? (
                  <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={3}>
                    <path strokeLinecap="round" strokeLinejoin="round" d="M5 13l4 4L19 7" />
                  </svg>
                ) : (
                  i + 1
                )}
              </div>
              {i < STEPS.length - 1 && (
                <div className={`flex-1 h-0.5 rounded ${i < currentIndex ? 'bg-paw-600' : 'bg-gray-800'}`} />
              )}
            </div>
          ))}
        </div>

        {/* Step content */}
        <div className="page-enter">
          {step === 'welcome' && (
            <div className="card text-center">
              <div className="inline-flex items-center justify-center w-20 h-20 rounded-2xl bg-paw-600/20 border border-paw-600/30 mb-6">
                <span className="text-4xl">🐾</span>
              </div>
              <h2 className="text-2xl font-bold tracking-tight mb-3">Welcome to SafePaw</h2>
              <p className="text-gray-400 mb-2">
                Let's get your private AI assistant up and running. This takes about 2 minutes.
              </p>
              <p className="text-gray-500 text-sm mb-8">
                We'll connect your AI provider, turn on security, and you're done.
              </p>
              <div className="space-y-3">
                <button onClick={handleWelcomeContinue} className="btn-primary w-full text-lg py-3">
                  Get Started
                </button>
              </div>
            </div>
          )}

          {step === 'apikey' && (
            <div className="card">
              <h2 className="text-xl font-bold tracking-tight mb-2">Connect your AI</h2>
              <p className="text-gray-400 text-sm mb-6">
                SafePaw needs an API key to talk to the AI. It's like a password that connects your system to the AI provider. Pick one:
              </p>

              {/* Provider selector */}
              <div className="flex gap-2 mb-6">
                <button
                  onClick={() => setApiProvider('anthropic')}
                  className={`flex-1 py-3 px-4 rounded-lg border text-sm font-medium transition-colors ${
                    apiProvider === 'anthropic'
                      ? 'border-paw-500 bg-paw-600/10 text-paw-400'
                      : 'border-gray-700 bg-gray-900 text-gray-400 hover:border-gray-600'
                  }`}
                >
                  Anthropic (Claude)
                </button>
                <button
                  onClick={() => setApiProvider('openai')}
                  className={`flex-1 py-3 px-4 rounded-lg border text-sm font-medium transition-colors ${
                    apiProvider === 'openai'
                      ? 'border-paw-500 bg-paw-600/10 text-paw-400'
                      : 'border-gray-700 bg-gray-900 text-gray-400 hover:border-gray-600'
                  }`}
                >
                  OpenAI (GPT)
                </button>
              </div>

              <div className="mb-6">
                <label htmlFor="apikey" className="block text-sm font-medium text-gray-300 mb-1.5">
                  {apiProvider === 'anthropic' ? 'Anthropic API Key' : 'OpenAI API Key'}
                </label>
                <input
                  id="apikey"
                  type="password"
                  className="input"
                  placeholder={apiProvider === 'anthropic' ? 'sk-ant-...' : 'sk-...'}
                  value={apiKey}
                  onChange={e => setApiKey(e.target.value)}
                  autoFocus
                />
                <p className="text-xs text-gray-500 mt-1.5">
                  {apiProvider === 'anthropic'
                    ? 'Sign up at console.anthropic.com, go to API Keys, and create one'
                    : 'Sign up at platform.openai.com, go to API Keys, and create one'}
                </p>
              </div>

              {error && (
                <div className="rounded-lg bg-red-500/10 border border-red-500/20 px-4 py-3 text-sm text-red-400 mb-4">
                  {error}
                </div>
              )}

              <div className="flex gap-3">
                <button onClick={() => setStep('welcome')} className="btn-secondary flex-1">
                  Back
                </button>
                <button onClick={handleSaveApiKey} disabled={saving || !apiKey.trim()} className="btn-primary flex-1">
                  {saving ? 'Saving…' : 'Continue'}
                </button>
              </div>
              <button
                onClick={() => { setError(''); setStep('security') }}
                className="w-full text-center text-sm text-gray-500 hover:text-gray-400 mt-3 transition-colors"
              >
                Skip for now — I'll add a key later
              </button>
            </div>
          )}

          {step === 'security' && (
            <div className="card">
              <h2 className="text-xl font-bold tracking-tight mb-2">Protect your AI</h2>
              <p className="text-gray-400 text-sm mb-6">
                Choose who can access your AI assistant.
              </p>

              {hasExistingAuth && (
                <div className="rounded-lg bg-green-500/10 border border-green-500/20 px-4 py-3 text-sm text-green-400 mb-4">
                  ✓ Security is already configured with a strong secret.
                </div>
              )}

              {/* Auth toggle */}
              <div className="flex items-center justify-between p-4 rounded-lg bg-gray-800/50 border border-gray-700 mb-4">
                <div>
                  <p className="text-sm font-medium text-gray-200">Require Login</p>
                  <p className="text-xs text-gray-500 mt-0.5">Users will need a token to access the AI</p>
                </div>
                <button
                  onClick={() => setAuthEnabled(!authEnabled)}
                  className={`relative w-11 h-6 rounded-full transition-colors ${
                    authEnabled ? 'bg-paw-600' : 'bg-gray-600'
                  }`}
                >
                  <span className={`absolute top-0.5 left-0.5 w-5 h-5 rounded-full bg-white transition-transform ${
                    authEnabled ? 'translate-x-5' : ''
                  }`} />
                </button>
              </div>

              {authEnabled && (
                <div className="mb-6">
                  <label htmlFor="authsecret" className="block text-sm font-medium text-gray-300 mb-1.5">
                    Secret Key (optional)
                  </label>
                  <input
                    id="authsecret"
                    type="password"
                    className="input"
                    placeholder="Leave blank to auto-generate"
                    value={authSecret}
                    onChange={e => setAuthSecret(e.target.value)}
                  />
                  <p className="text-xs text-gray-500 mt-1.5">
                    A long random string used as a master key. Leave blank and we'll use the existing one, or set it later in Settings.
                  </p>
                </div>
              )}

              {!authEnabled && (
                <div className="rounded-lg bg-yellow-500/10 border border-yellow-500/20 px-4 py-3 text-sm text-yellow-400 mb-6">
                  ⚠️ Without login, anyone who finds your AI's address can use it freely. Only turn this off if the AI is on a private network.
                </div>
              )}

              {error && (
                <div className="rounded-lg bg-red-500/10 border border-red-500/20 px-4 py-3 text-sm text-red-400 mb-4">
                  {error}
                </div>
              )}

              <div className="flex gap-3">
                <button onClick={() => { setStep('apikey'); setError('') }} className="btn-secondary flex-1">
                  Back
                </button>
                <button onClick={handleSaveSecurity} disabled={saving} className="btn-primary flex-1">
                  {saving ? 'Saving…' : 'Continue'}
                </button>
              </div>
            </div>
          )}

          {step === 'done' && (
            <div className="card text-center">
              <div className="inline-flex items-center justify-center w-20 h-20 rounded-2xl bg-green-600/20 border border-green-600/30 mb-6">
                <svg className="w-10 h-10 text-green-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                  <path strokeLinecap="round" strokeLinejoin="round" d="M5 13l4 4L19 7" />
                </svg>
              </div>
              <h2 className="text-2xl font-bold tracking-tight mb-3">You're all set!</h2>
              <p className="text-gray-400 mb-2">
                Your private AI assistant is configured and protected. You're ready to go.
              </p>
              <p className="text-gray-500 text-sm mb-8">
                You can change any of these settings later. Click the button below to see your dashboard.
              </p>
              <button onClick={onComplete} className="btn-primary w-full text-lg py-3">
                Go to Dashboard
              </button>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
