import { useState } from 'react'
import { NavLink, Outlet } from 'react-router-dom'
import { Input, Toast } from '@heroui/react'
import { getTenant, setTenant, SERVICES, type Service } from '../api/client'

export const SERVICE_META: Record<Service, { name: string; icon: string; tagline: string }> = {
  amphora: { name: 'Amphora', icon: '🗄️', tagline: 'Object storage (S3)' },
  paramdora: { name: 'Paramdora', icon: '🔐', tagline: 'Parameter store (SSM)' },
  hephaestus: { name: 'Hephaestus', icon: '⚙️', tagline: 'Compute (EC2)' },
  orpheus: { name: 'Orpheus', icon: '☸️', tagline: 'Kubernetes (EKS)' },
  clio: { name: 'Clio', icon: '🗃️', tagline: 'Databases (RDS)' },
  mneme: { name: 'Mneme', icon: '💾', tagline: 'Caches (ElastiCache)' },
  iris: { name: 'Iris', icon: '📨', tagline: 'Messaging (SQS+SNS)' },
  themis: { name: 'Themis', icon: '⚖️', tagline: 'Identity & access (IAM)' },
}

export default function AppShell() {
  const [tenant, setTenantState] = useState(getTenant())

  return (
    <>
      <Toast.Provider placement="bottom end" />
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
            <div className="text-xs text-muted">Olympus platform · operating console</div>
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
