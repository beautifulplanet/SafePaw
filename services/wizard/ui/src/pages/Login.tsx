import { useState, type FormEvent } from 'react'
import { api, ApiError } from '../api'

interface LoginProps {
  onSuccess: () => void
}

export function Login({ onSuccess }: LoginProps) {
  const [password, setPassword] = useState('')
  const [totp, setTotp] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const [showRecovery, setShowRecovery] = useState(false)
  const [showTotp, setShowTotp] = useState(false)

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError('')
    setLoading(true)

    try {
      await api.login(password, totp || undefined)
      onSuccess()
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        const msg = err.message === 'totp_required' ? 'TOTP code required (MFA is enabled)' : err.message === 'invalid totp code' ? 'Invalid TOTP code' : 'Invalid password'
        setError(msg)
        if (err.message === 'totp_required') setShowTotp(true)
      } else if (err instanceof ApiError && err.status === 429) {
        setError('Too many failed attempts — your IP is temporarily locked out. Wait a few minutes and try again.')
      } else if (err instanceof ApiError) {
        setError(err.message || `Server error (${err.status})`)
      } else {
        setError('Connection failed — is the wizard server running?')
      }
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="flex items-center justify-center min-h-[calc(100vh-12rem)]">
      <div className="w-full max-w-md">
        {/* Branding */}
        <div className="text-center mb-8">
          <div className="inline-flex items-center justify-center w-16 h-16 rounded-2xl bg-paw-600/20 border border-paw-600/30 mb-4">
            <span className="text-3xl">🐾</span>
          </div>
          <h2 className="text-2xl font-bold tracking-tight">Welcome to SafePaw</h2>
          <p className="text-gray-400 mt-2">
            Your private AI assistant's control panel. Sign in with your password (admin, operator, or viewer).
          </p>
        </div>

        {/* Login form */}
        <form onSubmit={handleSubmit} className="card space-y-5">
          <div>
            <label htmlFor="password" className="block text-sm font-medium text-gray-300 mb-1.5">
              Password
            </label>
            <input
              id="password"
              type="password"
              className="input"
              placeholder="Paste from terminal output"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              autoFocus
              required
              disabled={loading}
            />
            <p className="text-xs text-gray-500 mt-1.5">
              Your password was shown in the terminal when the system first started. If you set one in your configuration, use that.
            </p>
          </div>
          {(showTotp || totp) && (
            <div>
              <label htmlFor="totp" className="block text-sm font-medium text-gray-300 mb-1.5">
                TOTP code (MFA)
              </label>
              <input
                id="totp"
                type="text"
                inputMode="numeric"
                autoComplete="one-time-code"
                maxLength={6}
                placeholder="000000"
                className="input font-mono text-lg tracking-widest"
                value={totp}
                onChange={(e) => setTotp(e.target.value.replace(/\D/g, '').slice(0, 6))}
                disabled={loading}
              />
              <p className="text-xs text-gray-500 mt-1.5">
                Enter the 6-digit code from your authenticator app.
              </p>
            </div>
          )}
          {!showTotp && !totp && (
            <button
              type="button"
              onClick={() => setShowTotp(true)}
              className="text-xs text-paw-400 hover:text-paw-300"
            >
              Using MFA? Enter TOTP code
            </button>
          )}
          <div>
            <button
              type="button"
              onClick={() => setShowRecovery(!showRecovery)}
              className="text-xs text-paw-400 hover:text-paw-300 mt-1"
            >
              {showRecovery ? 'Hide' : 'Lost your password?'}
            </button>
            {showRecovery && (
              <div className="mt-3 p-3 rounded-lg bg-gray-800/50 border border-gray-700 text-xs text-gray-400 space-y-2">
                <p className="font-medium text-gray-300">Recovery options:</p>
                <ol className="list-decimal list-inside space-y-1">
                  <li>Get the password from container logs: <code className="px-1 rounded bg-gray-900 font-mono">docker compose logs wizard</code> or <code className="px-1 rounded bg-gray-900 font-mono">docker logs safepaw-wizard</code> (check the first startup lines).</li>
                  <li>Set a new password: add <code className="px-1 rounded bg-gray-900">WIZARD_ADMIN_PASSWORD=yourpassword</code> to <code className="px-1 rounded bg-gray-900">.env</code>, then run <code className="px-1 rounded bg-gray-900 font-mono">docker compose restart wizard</code>. Use the new password to sign in.</li>
                </ol>
              </div>
            )}
          </div>

          {error && (
            <div className="rounded-lg bg-red-500/10 border border-red-500/20 px-4 py-3 text-sm text-red-400">
              {error}
            </div>
          )}

          <button
            type="submit"
            className="btn-primary w-full"
            disabled={loading || !password}
          >
            {loading ? (
              <span className="flex items-center gap-2">
                <Spinner />
                Authenticating…
              </span>
            ) : (
              'Sign In'
            )}
          </button>
        </form>

        {/* Security guidance */}
        <div className="mt-6 rounded-lg border border-gray-800 bg-gray-900/30 p-4 text-sm text-gray-500">
          <p className="font-medium text-gray-400 mb-1">🔒 Security</p>
          <p className="mb-2">
            The wizard only accepts connections from localhost. Your session is protected by CSP, CORS, and rate limiting.
          </p>
          <p className="text-gray-500 text-xs">
            For production: set a strong <code className="px-1 rounded bg-gray-800">WIZARD_ADMIN_PASSWORD</code> in <code className="px-1 rounded bg-gray-800">.env</code>, enable gateway <code className="px-1 rounded bg-gray-800">AUTH_ENABLED</code> and <code className="px-1 rounded bg-gray-800">TLS_ENABLED</code> with valid certs.
          </p>
        </div>
      </div>
    </div>
  )
}

function Spinner() {
  return (
    <svg className="animate-spin h-4 w-4" viewBox="0 0 24 24" fill="none">
      <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
      <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
    </svg>
  )
}
