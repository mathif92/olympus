import { useMemo, useState, type FormEvent } from 'react'
import { Input, TextArea } from '@heroui/react'
import { PageHeader, ProjectPicker } from '../components/PageHeader'
import { Card, Field, Button, Modal, useToast, StateBadge, useAsync, EmptyState, Badge, SelectField, Spinner } from '../components/ui'
import { formatTime } from '../components/format'
import type { PromFunction, PromRuntime, PromFunctionVersion, PromInvocation } from '../api/types'
import { api } from '../api/client'

const SERVICE = 'prometheus'

function CreateFunction({ project, runtimes, onDone }: { project: string; runtimes: PromRuntime[]; onDone: () => void }) {
  const { show } = useToast()
  const [name, setName] = useState('')
  const [runtime, setRuntime] = useState('')
  const [timeoutMs, setTimeoutMs] = useState('30000')
  const [memoryMb, setMemoryMb] = useState('128')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const rt = runtimes.find((r) => r.id === runtime)

  async function submit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError('')
    try {
      await api<PromFunction>(SERVICE, '/functions', {
        method: 'POST',
        body: JSON.stringify({ project, name, runtime, timeout_ms: Number(timeoutMs), memory_mb: Number(memoryMb) }),
      })
      show('success', `Created function ${name}`)
      onDone()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <form onSubmit={submit}>
      <div className="form-grid">
        <Field label="Function name"><Input value={name} onChange={(e) => setName(e.target.value)} required autoFocus /></Field>
        <Field label="Runtime">
          <SelectField
            label=""
            value={runtime}
            onChange={setRuntime}
            options={runtimes.map((r) => ({ value: r.id, label: `${r.name} (${r.id})` }))}
            placeholder="— select runtime —"
          />
        </Field>
      </div>
      <div className="form-grid">
        <Field label="Timeout (ms)"><Input value={timeoutMs} onChange={(e) => setTimeoutMs(e.target.value)} type="number" min={100} /></Field>
        <Field label="Memory (MB)"><Input value={memoryMb} onChange={(e) => setMemoryMb(e.target.value)} type="number" min={16} /></Field>
      </div>
      {rt && (
        <p className="text-xs text-muted">
          {rt.handler_file}: <span className="mono">{rt.handler_func}</span>
        </p>
      )}
      {error && <div className="form-errors">{error}</div>}
      <div className="row-end">
        <Button variant="primary" type="submit" disabled={busy || !runtime}>{busy ? 'Creating…' : 'Create'}</Button>
      </div>
    </form>
  )
}

function DeployCode({ project, fn, onDone }: { project: string; fn: PromFunction; onDone: () => void }) {
  const { show } = useToast()
  const [file, setFile] = useState<File | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const rt = fn.runtime

  async function submit(e: FormEvent) {
    e.preventDefault()
    if (!file) return
    setBusy(true)
    setError('')
    try {
      const fd = new FormData()
      fd.append('code', file)
      const ver = await api<PromFunctionVersion>(SERVICE, `/function/${encodeURIComponent(project)}/${encodeURIComponent(fn.name)}/versions`, {
        method: 'POST',
        body: fd,
      })
      show('success', `Deployed ${fn.name} v${ver.version}`)
      onDone()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <form onSubmit={submit}>
      <p className="text-xs text-muted">
        Upload a <span className="mono">.zip</span> containing the entrypoint for runtime <Badge tone="info">{rt}</Badge>.
      </p>
      <Field label="Code archive (.zip)">
        <Input type="file" accept=".zip,application/zip" onChange={(e) => setFile(e.target.files?.[0] ?? null)} required />
      </Field>
      {error && <div className="form-errors">{error}</div>}
      <div className="row-end">
        <Button variant="primary" type="submit" disabled={busy || !file}>{busy ? 'Deploying…' : 'Deploy version'}</Button>
      </div>
    </form>
  )
}

function InvokePanel({ project, fn }: { project: string; fn: PromFunction }) {
  const { show } = useToast()
  const [event, setEvent] = useState('{\n  "message": "hello"\n}')
  const [busy, setBusy] = useState(false)
  const [result, setResult] = useState<PromInvocation | null>(null)
  const [invs, setInvs] = useState<PromInvocation[] | null>(null)

  async function run(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setResult(null)
    let payload: string
    try {
      payload = JSON.stringify(JSON.parse(event))
    } catch {
      show('error', 'event must be valid JSON')
      setBusy(false)
      return
    }
    try {
      const inv = await api<PromInvocation>(SERVICE, `/function/${encodeURIComponent(project)}/${encodeURIComponent(fn.name)}/invoke`, {
        method: 'POST',
        body: payload,
      })
      setResult(inv)
      show('success', `Invocation ${inv.status} in ${inv.duration_ms}ms`)
      loadInvs()
    } catch (err) {
      show('error', err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }

  async function loadInvs() {
    try {
      const r = await api<{ invocations: PromInvocation[] }>(SERVICE, `/function/${encodeURIComponent(project)}/${encodeURIComponent(fn.name)}/invocations`)
      setInvs(r.invocations ?? [])
    } catch {
      /* ignore */
    }
  }

  return (
    <div>
      <Card title={`Invoke ${fn.name} (v${fn.current_version || '—'})`}>
        <form onSubmit={run}>
          <Field label="Event (JSON)">
            <TextArea value={event} onChange={(e) => setEvent(e.target.value)} className="font-mono text-xs" />
          </Field>
          <div className="row-end">
            <Button variant="primary" type="submit" disabled={busy}>{busy ? 'Running…' : 'Invoke'}</Button>
          </div>
        </form>
      </Card>
      <div className="section-gap" />
      {result && (
        <Card title="Last result">
          <div className="row gap-2">
            <StateBadge state={result.status} />
            <Badge tone="info">{result.duration_ms} ms</Badge>
            {result.exit_code ? <Badge tone="warn">exit {result.exit_code}</Badge> : null}
          </div>
          <div style={{ height: 8 }} />
          {result.response !== undefined && (
            <pre className="overflow-auto rounded-md bg-surface-secondary p-3 text-xs">{result.response}</pre>
          )}
          {result.error && <pre className="overflow-auto rounded-md bg-surface-secondary p-3 text-xs text-red-500">{result.error}</pre>}
        </Card>
      )}
      <div className="section-gap" />
      <Card title="Recent invocations" actions={<Button variant="ghost" size="sm" onPress={loadInvs}>{invs ? 'Refresh' : 'Load'}</Button>}>
        {invs === null ? (
          <EmptyState icon="⚡" title="Load invocation history" hint="Show the most recent runs of this function." />
        ) : invs.length === 0 ? (
          <EmptyState icon="🌙" title="No invocations yet" hint="Invoke the function to see results here." />
        ) : (
          <table className="data-table">
            <thead><tr><th>Version</th><th>Status</th><th>Duration</th><th>Response</th><th>Invoked</th></tr></thead>
            <tbody>
              {invs.map((i) => (
                <tr key={i.id}>
                  <td className="num">{i.version}</td>
                  <td><StateBadge state={i.status} /></td>
                  <td className="num">{i.duration_ms}ms</td>
                  <td><span className="mono" style={{ fontSize: 11 }}>{(i.response || i.error || '').slice(0, 48)}</span></td>
                  <td className="muted">{formatTime(i.invoked_at)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Card>
    </div>
  )
}

export default function PrometheusPage() {
  const [project, setProject] = useState(() => sessionStorage.getItem(`olympus.project.${SERVICE}`) || '')
  const [creating, setCreating] = useState(false)
  const [openFn, setOpenFn] = useState<PromFunction | null>(null)
  const [deployFn, setDeployFn] = useState<PromFunction | null>(null)
  const { show } = useToast()

  const runtimes = useAsync<PromRuntime[]>(() =>
    api<{ runtimes: PromRuntime[] }>(SERVICE, '/runtimes').then((r) => r.runtimes ?? []),
  )
  const functions = useAsync<PromFunction[]>(() => {
    if (!project) return Promise.resolve([])
    return api<{ functions: PromFunction[] }>(SERVICE, '/functions', { query: { project } }).then((r) => r.functions ?? [])
  })
  const runtimeById = useMemo(() => {
    const m = new Map<string, PromRuntime>()
    for (const r of runtimes.data ?? []) m.set(r.id, r)
    return m
  }, [runtimes.data])

  async function del(name: string) {
    try {
      await api(SERVICE, `/function/${encodeURIComponent(project)}/${encodeURIComponent(name)}`, { method: 'DELETE' })
      show('success', `Deleted function ${name}`)
      functions.refetch()
    } catch (err) {
      show('error', err instanceof Error ? err.message : String(err))
    }
  }

  return (
    <div>
      <PageHeader icon="🔥" title="Prometheus" tagline="Serverless functions (λ) — deploy a code zip per runtime; invoke it with a JSON event.">
        <Button variant="ghost" onPress={() => functions.refetch()}>Refresh</Button>
        <Button variant="primary" disabled={!project} onPress={() => setCreating(true)}>+ Function</Button>
      </PageHeader>
      <ProjectPicker service={SERVICE} onSelect={setProject} />

      {!project && <EmptyState icon="🔥" title="Select a project" hint="Choose a project to work with its functions." />}

      {project && (
        <Card title={`Functions · ${project}`}>
          {functions.loading ? <Spinner /> : functions.data?.length === 0 ? (
            <EmptyState icon="🔥" title="No functions" hint="Create a function, then deploy a code zip and invoke it." />
          ) : (
            <table className="data-table">
              <thead><tr><th>Name</th><th>Runtime</th><th>Version</th><th>Timeout</th><th>Memory</th><th className="right">Actions</th></tr></thead>
              <tbody>
                {functions.data?.map((f) => (
                  <tr key={f.id}>
                    <td><span className="mono">{f.name}</span></td>
                    <td><Badge tone="info">{runtimeById.get(f.runtime)?.name ?? f.runtime}</Badge></td>
                    <td className="num">{f.current_version || '—'}</td>
                    <td className="num">{f.timeout_ms}ms</td>
                    <td className="num">{f.memory_mb}MB</td>
                    <td className="right">
                      <Button variant="ghost" size="sm" onPress={() => setDeployFn(f)}>Deploy</Button>
                      <Button variant="ghost" size="sm" onPress={() => setOpenFn(f)}>Invoke</Button>
                      <Button variant="danger" size="sm" onPress={() => del(f.name)}>Delete</Button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </Card>
      )}

      <Modal open={creating} onClose={() => setCreating(false)} title="Create function" footer={<Button variant="ghost" onPress={() => setCreating(false)}>Cancel</Button>}>
        <CreateFunction project={project} runtimes={runtimes.data ?? []} onDone={() => { setCreating(false); functions.refetch() }} />
      </Modal>

      {deployFn && (
        <Modal open wide onClose={() => setDeployFn(null)} title={`Deploy code — ${deployFn.name}`} footer={<Button variant="ghost" onPress={() => setDeployFn(null)}>Close</Button>}>
          <DeployCode project={project} fn={deployFn} onDone={() => { setDeployFn(null); functions.refetch() }} />
        </Modal>
      )}

      {openFn && (
        <Modal open wide onClose={() => setOpenFn(null)} title={`Function ${openFn.name}`} footer={<Button variant="ghost" onPress={() => setOpenFn(null)}>Close</Button>}>
          <InvokePanel project={project} fn={openFn} />
        </Modal>
      )}
    </div>
  )
}
