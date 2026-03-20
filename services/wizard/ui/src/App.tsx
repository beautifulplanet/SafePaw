import { useState, useCallback, useEffect } from 'react'
import { Login } from './pages/Login'
import { Prerequisites } from './pages/Prerequisites'
import { Dashboard } from './pages/Dashboard'
import { Config } from './pages/Config'
import { Activity } from './pages/Activity'
import { Settings } from './pages/Settings'
import { Setup } from './pages/Setup'
import { Layout } from './components/Layout'
import { clearToken, setUserRole, api } from './api'

type Page = 'login' | 'prerequisites' | 'dashboard' | 'config' | 'activity' | 'settings' | 'setup'

export function App() {
  const [page, setPage] = useState<Page>('login')
  const [sessionChecked, setSessionChecked] = useState(false)

  // Restore session on load: if we have a valid cookie, GET /auth/me gives us role and we go to dashboard (S1 RBAC).
  useEffect(() => {
    if (sessionChecked) return
    setSessionChecked(true)
    api.authMe()
      .then((me) => {
        setUserRole(me.role)
        return api.health()
      })
      .then((health) => {
        if (health.needs_setup) setPage('setup')
        else setPage('dashboard')
      })
      .catch(() => {
        // No session or expired — stay on login
      })
  }, [sessionChecked])

  // On login, skip prerequisites if this is a first-run (needs_setup).
  const handleLogin = useCallback(async () => {
    try {
      const health = await api.health()
      if (health.needs_setup) {
        setPage('setup')
        return
      }
    } catch {
      // Fall through to prerequisites if health check fails
    }
    setPage('prerequisites')
  }, [])

  const handlePrerequisitesDone = useCallback(async () => {
    // Check if initial setup is needed before going to dashboard
    try {
      const health = await api.health()
      if (health.needs_setup) {
        setPage('setup')
        return
      }
    } catch {
      // If health check fails, proceed to dashboard anyway
    }
    setPage('dashboard')
  }, [])

  const handleSetupComplete = useCallback(() => {
    setPage('dashboard')
  }, [])

  const handleLogout = useCallback(() => {
    clearToken()
    setPage('login')
    setSessionChecked(false)
  }, [])

  const handleBackToPrereqs = useCallback(() => {
    setPage('prerequisites')
  }, [])

  const handleOpenConfig = useCallback(() => {
    setPage('config')
  }, [])

  const handleBackToDashboard = useCallback(() => {
    setPage('dashboard')
  }, [])

  const handleOpenActivity = useCallback(() => {
    setPage('activity')
  }, [])

  const handleOpenSettings = useCallback(() => {
    setPage('settings')
  }, [])

  const navigateTo = useCallback((p: Page) => setPage(p), [])

  return (
    <Layout
      page={page}
      onLogout={page !== 'login' ? handleLogout : undefined}
      onNavigate={page === 'dashboard' ? handleBackToPrereqs : page === 'config' ? handleBackToDashboard : undefined}
      onNavigateTo={navigateTo}
    >
      {page === 'login' && <Login onSuccess={handleLogin} />}
      {page === 'prerequisites' && <Prerequisites onContinue={handlePrerequisitesDone} />}
      {page === 'setup' && <Setup onComplete={handleSetupComplete} />}
      {page === 'dashboard' && <Dashboard onOpenConfig={handleOpenConfig} onOpenActivity={handleOpenActivity} onOpenSettings={handleOpenSettings} />}
      {page === 'config' && <Config onBack={handleBackToDashboard} />}
      {page === 'activity' && <Activity onBack={handleBackToDashboard} />}
      {page === 'settings' && <Settings onBack={handleBackToDashboard} />}
    </Layout>
  )
}
