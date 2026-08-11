import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { fetchGatewayHealth, SERVICES, type Service } from '../api/client'
import { SERVICE_META } from '../components/AppShell'
import { Badge, Card, Spinner } from '../components/ui'

interface HealthMap {
  [k: string]: string
}

function healthTone(state: string): 'ok' | 'warn' | 'danger' | 'neutral' | 'info' {
  if (state === 'ok') return 'ok'
  if (state === 'down') return 'danger'
  return 'warn'
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
      <div className="mb-5">
        <h1 className="m-0 text-[22px] font-semibold text-foreground">🏛️ Overview</h1>
        <p className="mt-1 mb-0 text-muted">
          An on-premise cloud built from a family of Go services: object
          storage, parameters, compute, Kubernetes, databases, caches and
          messaging — all behind one console.
        </p>
      </div>

      <Card
        className="mb-4"
        title="Platform health"
        actions={<Badge tone={status === 'healthy' ? 'ok' : status === 'down' ? 'danger' : 'warn'}>{status}</Badge>}
      >
        {!health ? (
          <div className="flex items-center gap-2 text-muted">
            <Spinner /> Querying the console gateway…
          </div>
        ) : (
          <div className="grid grid-cols-[repeat(auto-fill,minmax(220px,1fr))] gap-3">
            {SERVICES.map((s: Service) => {
              const meta = SERVICE_META[s]
              const state = health[s] ?? 'unknown'
              return (
                <Link
                  key={s}
                  to={`/${s}`}
                  className="flex flex-col gap-2 rounded-xl border border-border bg-surface-secondary p-4 transition-colors hover:border-accent"
                >
                  <div className="flex items-center gap-2 text-[15px] font-semibold text-foreground">
                    <span>{meta.icon}</span> {meta.name}
                    <span className="ml-auto">
                      <Badge tone={healthTone(state)}>{state}</Badge>
                    </span>
                  </div>
                  <div className="text-xs text-muted">{meta.tagline}</div>
                </Link>
              )
            })}
          </div>
        )}
      </Card>

      <Card title="Getting started">
        <p className="m-0 text-muted">
          Set the tenant identity in the <em>Tenant</em> field (top right) —
          every control-plane service is multi-tenant via the X-Account-Id
          header. Each service page lets you create projects and operate on
          its resources. Amphora is the exception: there the bucket lives in
          the URL, so upload to a bucket of your choosing directly.
        </p>
        <div className="row mt-4">
          {SERVICES.map((s: Service) => (
            <Link key={s} to={`/${s}`} className="btn-link">
              {SERVICE_META[s].icon} {SERVICE_META[s].name}
            </Link>
          ))}
        </div>
      </Card>
    </div>
  )
}
