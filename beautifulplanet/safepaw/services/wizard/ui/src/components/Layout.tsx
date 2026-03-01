import type { ReactNode } from 'react'

interface LayoutProps {
  children: ReactNode
  page: string
  onLogout?: () => void
  onNavigate?: () => void
}

export function Layout({ children, page, onLogout, onNavigate }: LayoutProps) {
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
            {/* Breadcrumb-style navigation */}
            <nav className="hidden sm:flex items-center gap-1 text-sm text-gray-500">
              <span className={page === 'login' ? 'text-paw-400 font-medium' : 'text-gray-400'}>
                Login
              </span>
              <ChevronRight />
              <span className={page === 'prerequisites' ? 'text-paw-400 font-medium' : 'text-gray-400'}>
                Prerequisites
              </span>
              <ChevronRight />
              <span className={page === 'dashboard' ? 'text-paw-400 font-medium' : page === 'config' ? 'text-gray-400' : ''}>
                Dashboard
              </span>
              {page === 'config' && (
                <>
                  <ChevronRight />
                  <span className="text-paw-400 font-medium">Configuration</span>
                </>
              )}
            </nav>

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
      <main className="flex-1 max-w-5xl mx-auto w-full px-6 py-10">
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
