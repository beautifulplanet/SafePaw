import type { ReactNode } from 'react'

type Page = 'login' | 'prerequisites' | 'dashboard' | 'config' | 'activity' | 'settings' | 'setup'

interface LayoutProps {
  children: ReactNode
  page: string
  onLogout?: () => void
  onNavigate?: () => void
  onNavigateTo?: (page: Page) => void
}

const mainTabs: { id: Page; label: string }[] = [
  { id: 'dashboard', label: 'Dashboard' },
  { id: 'activity', label: 'Activity' },
  { id: 'settings', label: 'Settings' },
]

export function Layout({ children, page, onLogout, onNavigate, onNavigateTo }: LayoutProps) {
  const showTabs = ['dashboard', 'activity', 'settings', 'config'].includes(page)

  return (
    <div className="min-h-screen flex flex-col">
      {/* Header */}
      <header className="border-b border-gray-800 bg-gray-900/80 backdrop-blur-sm sticky top-0 z-50">
        <div className="max-w-5xl mx-auto px-6 h-16 flex items-center justify-between">
          <div className="flex items-center gap-3">
            <div className="w-8 h-8 rounded-lg bg-paw-600 flex items-center justify-center text-lg">
              🐾
            </div>
            <div>
              <h1 className="text-lg font-semibold tracking-tight">SafePaw</h1>
              <p className="text-xs text-gray-500 -mt-0.5">Setup Wizard</p>
            </div>
          </div>

          <div className="flex items-center gap-3">
            {/* Breadcrumb nav for early flow */}
            {!showTabs && (
              <nav className="hidden sm:flex items-center gap-1 text-sm text-gray-500">
                <span className={page === 'login' ? 'text-paw-400 font-medium' : 'text-gray-400'}>
                  Login
                </span>
                <ChevronRight />
                <span className={page === 'prerequisites' ? 'text-paw-400 font-medium' : 'text-gray-400'}>
                  Prerequisites
                </span>
                {page === 'setup' && (
                  <>
                    <ChevronRight />
                    <span className="text-paw-400 font-medium">Setup</span>
                  </>
                )}
              </nav>
            )}

            {/* Tab pills for main pages */}
            {showTabs && onNavigateTo && (
              <nav className="hidden sm:flex items-center gap-1 bg-gray-800/50 rounded-lg p-1">
                {mainTabs.map(tab => {
                  const active = page === tab.id || (tab.id === 'settings' && page === 'config')
                  return (
                    <button
                      key={tab.id}
                      onClick={() => onNavigateTo(tab.id)}
                      className={`text-sm px-3 py-1.5 rounded-md transition-colors ${
                        active
                          ? 'bg-paw-600 text-white font-medium'
                          : 'text-gray-400 hover:text-gray-200 hover:bg-gray-700/50'
                      }`}
                    >
                      {tab.label}
                    </button>
                  )
                })}
              </nav>
            )}

            {onNavigate && (
              <button onClick={onNavigate} className="btn-secondary text-sm py-1.5 px-3">
                {page === 'config' ? 'Dashboard' : 'Prerequisites'}
              </button>
            )}
            {onLogout && (
              <button onClick={onLogout} className="btn-secondary text-sm py-1.5 px-3">
                Logout
              </button>
            )}
          </div>
        </div>
      </header>

      {/* Main content */}
      <main key={page} className="flex-1 max-w-5xl mx-auto w-full px-6 py-10 page-enter">
        {children}
      </main>

      {/* Footer */}
      <footer className="border-t border-gray-800 py-4">
        <div className="max-w-5xl mx-auto px-6 flex items-center justify-between text-xs text-gray-600">
          <span>SafePaw v0.1.0</span>
          <span>Secure OpenClaw Deployer</span>
        </div>
      </footer>
    </div>
  )
}

function ChevronRight() {
  return (
    <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
      <path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" />
    </svg>
  )
}
