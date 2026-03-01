import { useState, useCallback } from 'react'
import { Login } from './pages/Login'
import { Prerequisites } from './pages/Prerequisites'
import { Dashboard } from './pages/Dashboard'
import { Config } from './pages/Config'
import { Layout } from './components/Layout'
import { hasToken, clearToken } from './api'

type Page = 'login' | 'prerequisites' | 'dashboard' | 'config'

export function App() {
  const [page, setPage] = useState<Page>(hasToken() ? 'prerequisites' : 'login')

  const handleLogin = useCallback(() => {
    setPage('prerequisites')
  }, [])

  const handlePrerequisitesDone = useCallback(() => {
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

  return (
    <Layout
      page={page}
      onLogout={page !== 'login' ? handleLogout : undefined}
      onNavigate={page === 'dashboard' ? handleBackToPrereqs : page === 'config' ? handleBackToDashboard : undefined}
    >
      {page === 'login' && <Login onSuccess={handleLogin} />}
      {page === 'prerequisites' && <Prerequisites onContinue={handlePrerequisitesDone} />}
      {page === 'dashboard' && <Dashboard onOpenConfig={handleOpenConfig} />}
      {page === 'config' && <Config onBack={handleBackToDashboard} />}
    </Layout>
  )
}
