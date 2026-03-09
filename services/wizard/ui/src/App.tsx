import { useState, useCallback } from 'react'
import { Login } from './pages/Login'
import { Prerequisites } from './pages/Prerequisites'
import { Dashboard } from './pages/Dashboard'
import { Config } from './pages/Config'
import { Activity } from './pages/Activity'
import { Settings } from './pages/Settings'
import { Setup } from './pages/Setup'
import { Layout } from './components/Layout'
import { hasToken, clearToken, api } from './api'

type Page = 'login' | 'prerequisites' | 'dashboard' | 'config' | 'activity' | 'settings' | 'setup'

export function App() {
  const [page, setPage] = useState<Page>(hasToken() ? 'prerequisites' : 'login')

  // On login, skip prerequisites if this is a first-run (needs_setup).
  // start.sh already validated Docker/ports, so go straight to Setup wizard.
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
