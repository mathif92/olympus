// Thin fetch wrapper over the console gateway. The browser always talks to the
// same origin (/api/<service>/...), which the gateway reverse-proxies to each
// backend. All six control-plane services read the tenant from X-Account-Id.

export type Service = 'amphora' | 'paramdora' | 'hephaestus' | 'orpheus' | 'clio' | 'mneme' | 'iris'

export const SERVICES: Service[] = [
  'amphora',
  'paramdora',
  'hephaestus',
  'orpheus',
  'clio',
  'mneme',
  'iris',
]

const TENANT_KEY = 'olympus.tenant'

export function getTenant(): string {
  return localStorage.getItem(TENANT_KEY) || 'default'
}

export function setTenant(tenant: string) {
  localStorage.setItem(TENANT_KEY, tenant)
}

export class ApiError extends Error {
  status: number
  constructor(status: number, message: string) {
    super(message)
    this.status = status
  }
}

async function readError(res: Response): Promise<string> {
  const text = await res.text()
  if (text) return text.slice(0, 300)
  return `${res.status} ${res.statusText}`
}

interface RequestOptions {
  method?: string
  query?: Record<string, string | number | undefined>
  headers?: Record<string, string>
  body?: string
}

export async function api<T = unknown>(
  service: Service,
  path: string,
  opts: RequestOptions = {},
): Promise<T> {
  const { method = 'GET', query, headers = {}, body } = opts
  const url = new URL(`/api/${service}${path}`, window.location.origin)
  if (query) {
    for (const [k, v] of Object.entries(query)) {
      if (v !== undefined && v !== '') url.searchParams.set(k, String(v))
    }
  }
  const res = await fetch(url.toString(), {
    method,
    headers: {
      'X-Account-Id': getTenant(),
      'Content-Type': 'application/json',
      ...headers,
    },
    body,
    credentials: 'same-origin',
  })
  if (!res.ok) {
    throw new ApiError(res.status, await readError(res))
  }
  if (res.status === 204) return undefined as T
  const text = await res.text()
  if (!text) return undefined as T
  try {
    return JSON.parse(text) as T
  } catch {
    return text as unknown as T
  }
}

export function apiJSON<T = unknown>(service: Service, path: string, opts: RequestOptions = {}): Promise<T> {
  return api(service, path, { ...opts, headers: { 'Content-Type': 'application/json' } })
}

// Health aggregation from the gateway itself.
export interface GatewayHealth {
  status: string
  services: Record<string, string>
}

export async function fetchGatewayHealth(): Promise<GatewayHealth> {
  const res = await fetch('/api/health', { credentials: 'same-origin' })
  if (!res.ok) throw new ApiError(res.status, await readError(res))
  return res.json()
}