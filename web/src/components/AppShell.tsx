import { useEffect, useState } from 'react'
import { NavLink, Outlet } from 'react-router-dom'
import { Input, Toast } from '@heroui/react'
import { getTenant, setTenant, getAuth, setAuth, onAuthRequired, SERVICES, type Service, type StoredAuth } from '../api/client'
import { Badge, Button } from './ui'
import SignInModal from './SignInModal'

export const SERVICE_META: Record<Service, { name: string; icon: string; tagline: string }> = {
  amphora: { name: 'Amphora', icon: '🗄️', tagline: 'Object storage (S3)' },
  paramdora: { name: 'Paramdora', icon: '🔐', tagline: 'Parameter store (SSM)' },
  hephaestus: { name: 'Hephaestus', icon: '⚙️', tagline: 'Compute (EC2)' },
  orpheus: { name: 'Orpheus', icon: '☸️', tagline: 'Kubernetes (EKS)' },
  clio: { name: 'Clio', icon: '🗃️', tagline: 'Databases (RDS)' },
  mneme: { name: 'Mneme', icon: '💾', tagline: 'Caches (ElastiCache)' },
  iris: { name: 'Iris', icon: '📨', tagline: 'Messaging (SQS+SNS)' },
  themis: { name: 'Themis', icon: '⚖️', tagline: 'Identity & access (IAM)' },
  prometheus: { name: 'Prometheus', icon: '🔥', tagline: 'Serverless functions (λ)' },
}

export default function AppShell() {
  const [tenant, setTenantState] = useState(getTenant())
  const [auth, setAuthState] = useState<StoredAuth | null>(() => getAuth())
  const [signInOpen, setSignInOpen] = useState(false)

  useEffect(() => {
    const unsub = onAuthRequired(() => {
      // A stored session stopped being accepted (expired/revoked): drop it and
      // prompt for fresh credentials.
      setAuth(null)
      setAuthState(null)
      setSignInOpen(true)
    })
    return unsub
  }, [])

  return (
    <>
      <Toast.Provider placement="bottom end" />
      <SignInModal open={signInOpen} onClose={() => setSignInOpen(false)} />
      <div className="grid min-h-screen grid-cols-[232px_1fr] bg-background text-foreground">
        <aside className="sticky top-0 flex h-screen flex-col gap-3.5 self-start border-r border-border bg-surface px-2.5 py-4">
          <NavLink to="/" className="flex items-center gap-2.5 px-2 py-1 text-foreground">
            <span className="text-[22px]">⚡</span>
            <span className="flex flex-col leading-[1.15]">
              <strong className="text-base tracking-[0.3px]">Olympus</strong>
              <small className="text-[10px] tracking-[2px] text-muted uppercase">console</small>
            </span>
          </NavLink>
          <nav className="flex flex-col gap-0.5">
            <NavLink
              to="/"
              end
              className={({ isActive }) =>
                `flex items-center gap-2.5 rounded-md px-2.5 py-2 font-medium transition-colors ${
                  isActive ? 'bg-accent text-accent-foreground' : 'text-foreground hover:bg-surface-secondary'
                }`
              }
            >
              <span className="w-[18px] text-center">🏠</span> Overview
            </NavLink>
            <div className="px-2 pt-2.5 pb-1 text-[11px] tracking-[1.2px] text-muted uppercase">Services</div>
            {SERVICES.map((s) => (
              <NavLink
                key={s}
                to={`/${s}`}
                className={({ isActive }) =>
                  `flex items-center gap-2.5 rounded-md px-2.5 py-2 font-medium transition-colors ${
                    isActive ? 'bg-accent text-accent-foreground' : 'text-foreground hover:bg-surface-secondary'
                  }`
                }
              >
                <span className="w-[18px] text-center">{SERVICE_META[s].icon}</span>
                {SERVICE_META[s].name}
              </NavLink>
            ))}
          </nav>
        </aside>
        <div className="flex min-w-0 flex-col">
          <header className="sticky top-0 z-20 flex items-center justify-between gap-4 border-b border-border bg-surface px-6 py-3">
            <div className="flex items-center gap-3 text-xs text-muted">
              <span>Olympus platform · operating console</span>
              {!auth && (
                <Badge tone="warn">not signed in — service calls require a Themis token</Badge>
              )}
            </div>
            <div className="flex items-center gap-2 text-xs text-muted">
              <span>Tenant</span>
              <Input
                className="w-[180px]"
                variant="secondary"
                value={tenant}
                onChange={(e) => {
                  const v = e.target.value.trim() || 'default'
                  setTenant(v)
                  setTenantState(v)
                }}
                placeholder="default"
                aria-label="Account id"
              />
              {auth ? (
                <>
                  <Badge tone="ok">{auth.subject}</Badge>
                  <Button variant="ghost" size="sm" onPress={() => { setAuth(null); setAuthState(null) }}>
                    Sign out
                  </Button>
                </>
              ) : (
                <Button variant="primary" size="sm" onPress={() => setSignInOpen(true)}>
                  Sign in
                </Button>
              )}
            </div>
          </header>
          <div className="w-full max-w-[1200px] p-6">
            <Outlet />
          </div>
        </div>
      </div>
    </>
  )
}
