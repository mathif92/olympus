import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { fetchGatewayHealth, SERVICES, type Service } from '../api/client'
import { SERVICE_META } from '../components/AppShell'
import { Badge } from '../components/ui'

interface HealthMap {
  [k: string]: string
}

export default function Overview() {
  const [health, setHealth] = useState<HealthMap | null>(null)
  const [status, setStatus] = useState<string>('checking')

  useEffect(() => {
    let alive = true
    const poll = () =>
      fetchGatewayHealth().then(
        (h) => {
          if (alive) {
            setHealth(h.services)
            setStatus(h.status)
          }
        },
        () => {
          if (alive) setStatus('down')
        },
      )
    poll()
    const t = setInterval(poll, 8000)
    return () => {
      alive = false
      clearInterval(t)
    }
  }, [])

  return (
    <div>
      <div className="page-head">
        <div className="page-head-text">
          <h1>🏛️ Overview</h1>
          <p>
            An on-premise cloud built from a family of Go services: object
            storage, parameters, compute, Kubernetes, databases, caches and
            messaging — all behind one console.
          </p>
        </div>
      </div>

      <div className="card" style={{ marginBottom: 16 }}>
        <div className="card-head">
          <h3>Platform health</h3>
          <Badge tone={status === 'healthy' ? 'ok' : status === 'down' ? 'danger' : 'warn'}>{status}</Badge>
        </div>
        <div className="card-body">
          {!health ? (
            <p className="muted">Querying the console gateway…</p>
          ) : (
            <div className="health-grid">
              {SERVICES.map((s: Service) => {
                const meta = SERVICE_META[s]
                const state = health[s] ?? 'unknown'
                return (
                  <Link key={s} to={`/${s}`} className="card health-card">
                    <div className="svc">
                      <span>{meta.icon}</span> {meta.name}
                      <span style={{ marginLeft: 'auto' }}>
                        <Badge tone={state === 'ok' ? 'ok' : state === 'down' ? 'danger' : 'warn'}>{state}</Badge>
                      </span>
                    </div>
                    <div className="svc-tagline">{meta.tagline}</div>
                  </Link>
                )
              })}
            </div>
          )}
        </div>
      </div>

      <div className="card">
        <div className="card-head">
          <h3>Getting started</h3>
        </div>
        <div className="card-body">
          <p className="muted" style={{ marginTop: 0 }}>
            Set the tenant identity in the <em>Tenant</em> field (top right) —
            every control-plane service is multi-tenant via the X-Account-Id
            header. Each service page lets you create projects and operate on
            its resources. Amphora is the exception: there the bucket lives in
            the URL, so upload to a bucket of your choosing directly.
          </p>
          <div className="row" style={{ flexWrap: 'wrap' }}>
            {SERVICES.map((s: Service) => (
              <Link key={s} to={`/${s}`} className="btn">
                {SERVICE_META[s].icon} {SERVICE_META[s].name}
              </Link>
            ))}
          </div>
        </div>
      </div>
    </div>
  )
}