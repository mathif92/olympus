import { useState } from 'react'
import { NavLink, Outlet } from 'react-router-dom'
import { getTenant, setTenant, SERVICES, type Service } from '../api/client'
import { ToastProvider } from './ui'

export const SERVICE_META: Record<Service, { name: string; icon: string; tagline: string }> = {
  amphora: { name: 'Amphora', icon: '🗄️', tagline: 'Object storage (S3)' },
  paramdora: { name: 'Paramdora', icon: '🔐', tagline: 'Parameter store (SSM)' },
  hephaestus: { name: 'Hephaestus', icon: '⚙️', tagline: 'Compute (EC2)' },
  orpheus: { name: 'Orpheus', icon: '☸️', tagline: 'Kubernetes (EKS)' },
  clio: { name: 'Clio', icon: '🗃️', tagline: 'Databases (RDS)' },
  mneme: { name: 'Mneme', icon: '💾', tagline: 'Caches (ElastiCache)' },
  iris: { name: 'Iris', icon: '📨', tagline: 'Messaging (SQS+SNS)' },
}

export default function AppShell() {
  const [tenant, setTenantState] = useState(getTenant())

  return (
    <ToastProvider>
      <div className="shell">
        <aside className="sidebar">
          <NavLink to="/" className="brand">
            <span className="brand-mark">⚡</span>
            <span className="brand-text">
              <strong>Olympus</strong>
              <small>console</small>
            </span>
          </NavLink>
          <nav className="nav">
            <NavLink to="/" end className="nav-link">
              <span className="nav-icon">🏠</span> Overview
            </NavLink>
            <div className="nav-section">Services</div>
            {SERVICES.map((s) => (
              <NavLink key={s} to={`/${s}`} className="nav-link">
                <span className="nav-icon">{SERVICE_META[s].icon}</span>
                {SERVICE_META[s].name}
              </NavLink>
            ))}
          </nav>
        </aside>
        <div className="main">
          <header className="topbar">
            <div className="topbar-note">Olympus platform · operating console</div>
            <label className="tenant-switch">
              <span>Tenant</span>
              <input
                value={tenant}
                onChange={(e) => {
                  setTenant(e.target.value.trim() || 'default')
                  setTenantState(e.target.value.trim() || 'default')
                }}
                placeholder="default"
                aria-label="Account id"
              />
            </label>
          </header>
          <div className="content">
            <Outlet />
          </div>
        </div>
      </div>
    </ToastProvider>
  )
}