import { useState, type FormEvent } from 'react'
import { api, setToken, ApiError } from '../api'

interface LoginProps {
  onSuccess: () => void
}

export function Login({ onSuccess }: LoginProps) {
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError('')
    setLoading(true)

    try {
      const res = await api.login(password)
      setToken(res.token)
      onSuccess()
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        setError('Invalid password')
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
            Enter your admin password to begin setup.
          </p>
        </div>

        {/* Login form */}
        <form onSubmit={handleSubmit} className="card space-y-5">
          <div>
            <label htmlFor="password" className="block text-sm font-medium text-gray-300 mb-1.5">
              Admin Password
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
              The password was shown once when the wizard started.
            </p>
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

        {/* Info box */}
        <div className="mt-6 rounded-lg border border-gray-800 bg-gray-900/30 p-4 text-sm text-gray-500">
          <p className="font-medium text-gray-400 mb-1">🔒 Security Note</p>
          <p>
            The wizard only accepts connections from localhost.
            Your session is protected by CSP, CORS, and rate limiting.
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
