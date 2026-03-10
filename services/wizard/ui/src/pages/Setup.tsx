import { useState, useCallback, useEffect } from 'react'
import { api, type VerifyCheck } from '../api'

interface SetupProps {
  onComplete: () => void
}

type Step = 'identity' | 'purpose' | 'provider' | 'security' | 'verify'

const STEPS: { id: Step; label: string }[] = [
  { id: 'identity', label: 'You' },
  { id: 'purpose', label: 'Purpose' },
  { id: 'provider', label: 'AI' },
  { id: 'security', label: 'Security' },
  { id: 'verify', label: 'Verify' },
]

type UseCase = 'personal' | 'team' | 'development'

export function Setup({ onComplete }: SetupProps) {
  const [step, setStep] = useState<Step>('identity')
  const [ownerName, setOwnerName] = useState('')
  const [useCase, setUseCase] = useState<UseCase>('personal')
  const [apiKey, setApiKey] = useState('')
  const [apiProvider, setApiProvider] = useState<'anthropic' | 'openai'>('anthropic')
  const [authEnabled, setAuthEnabled] = useState(true)
  const [authSecret, setAuthSecret] = useState('')
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [hasExistingKey, setHasExistingKey] = useState(false)
  const [hasExistingAuth, setHasExistingAuth] = useState(false)
  const [verifying, setVerifying] = useState(false)
  const [verifyChecks, setVerifyChecks] = useState<VerifyCheck[]>([])
  const [verifyDone, setVerifyDone] = useState(false)
  const [verifyOverall, setVerifyOverall] = useState(false)

  // Check existing config on mount — auto-fill / auto-skip steps
  useEffect(() => {
    void (async () => {
      try {
        const { config } = await api.getConfig()
        if (config['ANTHROPIC_API_KEY'] || config['OPENAI_API_KEY']) {
          setHasExistingKey(true)
        }
        if (config['AUTH_SECRET'] && config['AUTH_SECRET'] !== 'CHANGE_ME_run_openssl_rand_base64_48') {
          setHasExistingAuth(true)
        }
        if (config['OWNER_NAME']) {
          setOwnerName(config['OWNER_NAME'])
        }
        if (config['USE_CASE']) {
          setUseCase(config['USE_CASE'] as UseCase)
        }
      } catch {
        // Config not available — proceed normally
      }
    })()
  }, [])

  const currentIndex = STEPS.findIndex(s => s.id === step)

  const handleSaveIdentity = useCallback(async () => {
    setSaving(true)
    setError('')
    try {
      if (ownerName.trim()) {
        await api.putConfig({ OWNER_NAME: ownerName.trim() })
      }
      setStep('purpose')
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to save')
    } finally {
      setSaving(false)
    }
  }, [ownerName])

  const handleSavePurpose = useCallback(async () => {
    setSaving(true)
    setError('')
    try {
      await api.putConfig({ USE_CASE: useCase })
      if (hasExistingKey) {
        setStep('security')
      } else {
        setStep('provider')
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to save')
    } finally {
      setSaving(false)
    }
  }, [useCase, hasExistingKey])

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
      setStep('verify')
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to save security settings')
    } finally {
      setSaving(false)
    }
  }, [authEnabled, authSecret])

  const handleVerify = useCallback(async () => {
    setVerifying(true)
    setError('')
    setVerifyChecks([])
    setVerifyDone(false)
    try {
      const result = await api.setupVerify()
      setVerifyChecks(result.checks)
      setVerifyOverall(result.overall)
      setVerifyDone(true)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Verification failed')
      setVerifyDone(true)
    } finally {
      setVerifying(false)
    }
  }, [])

  // Auto-run verification when reaching the verify step
  useEffect(() => {
    if (step === 'verify' && !verifyDone && !verifying) {
      void handleVerify()
    }
  }, [step, verifyDone, verifying, handleVerify])

  const useCaseOptions: { id: UseCase; title: string; desc: string; icon: string }[] = [
    { id: 'personal', title: 'Personal AI', desc: 'Just for me — a private AI assistant on my own machine.', icon: '👤' },
    { id: 'team', title: 'Team AI', desc: 'A shared AI for a small group. Multiple people will use it.', icon: '👥' },
    { id: 'development', title: 'Development', desc: 'Testing, building, or integrating AI into an app.', icon: '🛠️' },
  ]

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
          {step === 'identity' && (
            <div className="card">
              <div className="inline-flex items-center justify-center w-16 h-16 rounded-2xl bg-paw-600/20 border border-paw-600/30 mb-5">
                <span className="text-3xl">🐾</span>
              </div>
              <h2 className="text-2xl font-bold tracking-tight mb-2">Who are you?</h2>
              <p className="text-gray-400 text-sm mb-6">
                SafePaw is your private AI gateway — it sits between you and the AI, keeping things secure.
                Your name helps us personalize the experience.
              </p>
              <div className="mb-6">
                <label htmlFor="ownerName" className="block text-sm font-medium text-gray-300 mb-1.5">
                  Your name or team name
                </label>
                <input
                  id="ownerName"
                  type="text"
                  className="input"
                  placeholder="e.g. Alex, Engineering Team"
                  value={ownerName}
                  onChange={e => setOwnerName(e.target.value)}
                  autoFocus
                  maxLength={100}
                />
                <p className="text-xs text-gray-500 mt-1.5">
                  This is just a display name — it's stored locally and never sent to any AI provider.
                </p>
              </div>
              {error && (
                <div className="rounded-lg bg-red-500/10 border border-red-500/20 px-4 py-3 text-sm text-red-400 mb-4">
                  {error}
                </div>
              )}
              <button onClick={handleSaveIdentity} disabled={saving} className="btn-primary w-full text-lg py-3">
                {saving ? 'Saving…' : 'Continue'}
              </button>
            </div>
          )}

          {step === 'purpose' && (
            <div className="card">
              <h2 className="text-xl font-bold tracking-tight mb-2">What will you use this for?</h2>
              <p className="text-gray-400 text-sm mb-6">
                This helps us pick the right defaults. You can change everything later.
              </p>
              <div className="space-y-3 mb-6">
                {useCaseOptions.map(opt => (
                  <button
                    key={opt.id}
                    onClick={() => setUseCase(opt.id)}
                    className={`w-full text-left p-4 rounded-lg border transition-colors ${
                      useCase === opt.id
                        ? 'border-paw-500 bg-paw-600/10'
                        : 'border-gray-700 bg-gray-900 hover:border-gray-600'
                    }`}
                  >
                    <div className="flex items-start gap-3">
                      <span className="text-xl mt-0.5">{opt.icon}</span>
                      <div>
                        <p className={`text-sm font-medium ${useCase === opt.id ? 'text-paw-400' : 'text-gray-200'}`}>
                          {opt.title}
                        </p>
                        <p className="text-xs text-gray-500 mt-0.5">{opt.desc}</p>
                      </div>
                    </div>
                  </button>
                ))}
              </div>
              {error && (
                <div className="rounded-lg bg-red-500/10 border border-red-500/20 px-4 py-3 text-sm text-red-400 mb-4">
                  {error}
                </div>
              )}
              <div className="flex gap-3">
                <button onClick={() => { setStep('identity'); setError('') }} className="btn-secondary flex-1">
                  Back
                </button>
                <button onClick={handleSavePurpose} disabled={saving} className="btn-primary flex-1">
                  {saving ? 'Saving…' : 'Continue'}
                </button>
              </div>
            </div>
          )}

          {step === 'provider' && (
            <div className="card">
              <h2 className="text-xl font-bold tracking-tight mb-2">Pick your AI</h2>
              <p className="text-gray-400 text-sm mb-2">
                SafePaw needs an API key to connect to your AI provider.
              </p>
              <p className="text-gray-500 text-xs mb-6">
                An API key is like a password between your SafePaw and the AI company's servers.
                It's stored locally in your .env file and never shared.
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
                    ? 'Get one at console.anthropic.com → API Keys → Create Key'
                    : 'Get one at platform.openai.com → API Keys → Create Key'}
                </p>
              </div>

              {error && (
                <div className="rounded-lg bg-red-500/10 border border-red-500/20 px-4 py-3 text-sm text-red-400 mb-4">
                  {error}
                </div>
              )}

              <div className="flex gap-3">
                <button onClick={() => { setStep('purpose'); setError('') }} className="btn-secondary flex-1">
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
                Skip — I'll add a key later
              </button>
            </div>
          )}

          {step === 'security' && (
            <div className="card">
              <h2 className="text-xl font-bold tracking-tight mb-2">Set your security level</h2>
              <p className="text-gray-400 text-sm mb-6">
                This controls who can talk to your AI. SafePaw sits in front of the AI and checks every request.
              </p>

              {hasExistingAuth && (
                <div className="rounded-lg bg-green-500/10 border border-green-500/20 px-4 py-3 text-sm text-green-400 mb-4">
                  ✓ A security secret is already configured.
                </div>
              )}

              {/* Auth toggle */}
              <div className="flex items-center justify-between p-4 rounded-lg bg-gray-800/50 border border-gray-700 mb-4">
                <div>
                  <p className="text-sm font-medium text-gray-200">Require Login</p>
                  <p className="text-xs text-gray-500 mt-0.5">
                    {useCase === 'personal'
                      ? 'Recommended — prevents anyone else from using your AI and running up costs.'
                      : useCase === 'team'
                      ? 'Recommended — each team member gets a token to prove who they are.'
                      : 'Optional for local dev — turn on before deploying anywhere reachable.'}
                  </p>
                </div>
                <button
                  onClick={() => setAuthEnabled(!authEnabled)}
                  className={`relative w-11 h-6 rounded-full transition-colors flex-shrink-0 ml-4 ${
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
                    Master Secret (optional)
                  </label>
                  <input
                    id="authsecret"
                    type="password"
                    className="input"
                    placeholder="Leave blank to keep the existing one"
                    value={authSecret}
                    onChange={e => setAuthSecret(e.target.value)}
                  />
                  <p className="text-xs text-gray-500 mt-1.5">
                    This is the key SafePaw uses to sign login tokens. A strong one was auto-generated at install — only change it if you know what you're doing.
                  </p>
                </div>
              )}

              {!authEnabled && (
                <div className="rounded-lg bg-yellow-500/10 border border-yellow-500/20 px-4 py-3 text-sm text-yellow-400 mb-6">
                  <p className="font-medium mb-1">No login means no protection</p>
                  <p className="text-xs text-yellow-500">
                    Anyone who can reach your server's address can use (and be billed for) your AI.
                    Only disable this on a private network with no outside access.
                  </p>
                </div>
              )}

              {error && (
                <div className="rounded-lg bg-red-500/10 border border-red-500/20 px-4 py-3 text-sm text-red-400 mb-4">
                  {error}
                </div>
              )}

              <div className="flex gap-3">
                <button onClick={() => { setStep(hasExistingKey ? 'purpose' : 'provider'); setError('') }} className="btn-secondary flex-1">
                  Back
                </button>
                <button onClick={handleSaveSecurity} disabled={saving} className="btn-primary flex-1">
                  {saving ? 'Saving…' : 'Verify Setup'}
                </button>
              </div>
            </div>
          )}

          {step === 'verify' && (
            <div className="card">
              <h2 className="text-xl font-bold tracking-tight mb-2">Verify it works</h2>
              <p className="text-gray-400 text-sm mb-6">
                SafePaw is testing the full chain: your config → the gateway → the AI backend.
              </p>

              <div className="space-y-3 mb-6">
                {verifying && verifyChecks.length === 0 && (
                  <div className="flex items-center gap-3 p-4 rounded-lg bg-gray-800/50 border border-gray-700">
                    <div className="w-5 h-5 border-2 border-paw-500 border-t-transparent rounded-full animate-spin" />
                    <p className="text-sm text-gray-300">Running checks…</p>
                  </div>
                )}

                {verifyChecks.map((check, i) => (
                  <div key={i} className={`flex items-start gap-3 p-4 rounded-lg border ${
                    check.pass
                      ? 'bg-green-500/5 border-green-500/20'
                      : 'bg-red-500/5 border-red-500/20'
                  }`}>
                    <div className="mt-0.5 flex-shrink-0">
                      {check.pass ? (
                        <svg className="w-5 h-5 text-green-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                          <path strokeLinecap="round" strokeLinejoin="round" d="M5 13l4 4L19 7" />
                        </svg>
                      ) : (
                        <svg className="w-5 h-5 text-red-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                          <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
                        </svg>
                      )}
                    </div>
                    <div>
                      <p className={`text-sm font-medium ${check.pass ? 'text-green-300' : 'text-red-300'}`}>
                        {check.name}
                      </p>
                      <p className="text-xs text-gray-400 mt-0.5">{check.message}</p>
                    </div>
                  </div>
                ))}
              </div>

              {error && (
                <div className="rounded-lg bg-red-500/10 border border-red-500/20 px-4 py-3 text-sm text-red-400 mb-4">
                  {error}
                </div>
              )}

              {verifyDone && (
                <div className="space-y-3">
                  {verifyOverall ? (
                    <>
                      <div className="rounded-lg bg-green-500/10 border border-green-500/20 px-4 py-3 text-center mb-2">
                        <p className="text-green-400 font-medium">Everything is working!</p>
                        <p className="text-xs text-green-500 mt-1">Your AI assistant is configured, protected, and ready to use.</p>
                      </div>
                      <button onClick={onComplete} className="btn-primary w-full text-lg py-3">
                        Go to Dashboard
                      </button>
                    </>
                  ) : (
                    <>
                      <p className="text-sm text-gray-400 text-center">
                        Some checks didn't pass. You can fix the issues and retry, or continue anyway.
                      </p>
                      <div className="flex gap-3">
                        <button onClick={() => { setStep('security'); setError(''); setVerifyDone(false); setVerifyChecks([]) }} className="btn-secondary flex-1">
                          Back
                        </button>
                        <button onClick={() => { setVerifyDone(false); setVerifyChecks([]) }} className="btn-primary flex-1">
                          Retry
                        </button>
                      </div>
                      <button onClick={onComplete} className="w-full text-center text-sm text-gray-500 hover:text-gray-400 mt-2 transition-colors">
                        Skip — go to Dashboard anyway
                      </button>
                    </>
                  )}
                </div>
              )}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
