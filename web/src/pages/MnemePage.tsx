import { useState, type FormEvent } from 'react'
import { PageHeader, ProjectPicker } from '../components/PageHeader'
import { Card, Field, Button, Modal, useToast, StateBadge, useAsync, EmptyState, Badge } from '../components/ui'
import { formatTime } from '../components/format'
import type { CacheCluster, CacheEngine, NodeType, CacheSnapshot } from '../api/types'
import { api } from '../api/client'

const SERVICE = 'mneme'

function CreateCluster({ project, onDone }: { project: string; onDone: () => void }) {
  const { show } = useToast()
  const [name, setName] = useState('')
  const [engine, setEngine] = useState('')
  const [engineVersion, setEngineVersion] = useState('')
  const [nodeType, setNodeType] = useState('')
  const [numNodes, setNumNodes] = useState('1')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  const engines = useAsync<CacheEngine[]>(() => api<{ cache_engines: CacheEngine[] }>(SERVICE, '/engines').then((r) => r.cache_engines ?? []))
  const types = useAsync<NodeType[]>(() => api<{ node_types: NodeType[] }>(SERVICE, '/node-types').then((r) => r.node_types ?? []))

  async function submit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError('')
    try {
      await api<CacheCluster>(SERVICE, '/clusters', {
        method: 'POST',
        body: JSON.stringify({ project, name, engine, engine_version: engineVersion, node_type: nodeType, num_nodes: Number(numNodes) }),
      })
      show('success', `Created cache cluster ${name}`)
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
        <Field label="Name"><input value={name} onChange={(e) => setName(e.target.value)} required autoFocus /></Field>
        <Field label="Engine">
          <select value={engine} onChange={(e) => { setEngine(e.target.value); setEngineVersion('') }} required>
            <option value="">—</option>
            {Array.from(new Set((engines.data ?? []).map((x) => x.engine))).map((e) => (
              <option key={e} value={e}>{e}</option>
            ))}
          </select>
        </Field>
      </div>
      <Field label="Engine version">
        <select value={engineVersion} onChange={(e) => setEngineVersion(e.target.value)} required>
          <option value="">—</option>
          {(engines.data ?? []).filter((x) => x.engine === engine).map((x) => (
            <option key={x.version} value={x.version}>{x.version} ({x.status})</option>
          ))}
        </select>
      </Field>
      <div className="form-grid">
        <Field label="Node type">
          <select value={nodeType} onChange={(e) => setNodeType(e.target.value)} required>
            <option value="">—</option>
            {types.data?.map((t) => <option key={t.name} value={t.name}>{t.name} ({t.vcpus} vCPU · {t.memory_gb} GB)</option>)}
          </select>
        </Field>
        <Field label="Node count"><input value={numNodes} onChange={(e) => setNumNodes(e.target.value)} type="number" min={1} /></Field>
      </div>
      {error && <div className="form-errors">{error}</div>}
      <div className="row-end">
        <Button variant="primary" type="submit" disabled={busy}>{busy ? 'Creating…' : 'Create'}</Button>
      </div>
    </form>
  )
}

function CreateSnapshot({ project, clusters, onDone }: { project: string; clusters: CacheCluster[]; onDone: () => void }) {
  const { show } = useToast()
  const [name, setName] = useState('')
  const [cluster, setCluster] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  async function submit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError('')
    try {
      await api<CacheSnapshot>(SERVICE, '/snapshots', {
        method: 'POST',
        body: JSON.stringify({ project, cluster, name }),
      })
      show('success', `Snapshotted ${cluster} -> ${name}`)
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
        <Field label="Snapshot name"><input value={name} onChange={(e) => setName(e.target.value)} required autoFocus /></Field>
        <Field label="Cache cluster">
          <select value={cluster} onChange={(e) => setCluster(e.target.value)} required>
            <option value="">—</option>
            {clusters.map((x) => <option key={x.id} value={x.name}>{x.name} ({x.engine} {x.engine_version})</option>)}
          </select>
        </Field>
      </div>
      {error && <div className="form-errors">{error}</div>}
      <div className="row-end">
        <Button variant="primary" type="submit" disabled={busy}>{busy ? 'Creating…' : 'Snapshot'}</Button>
      </div>
    </form>
  )
}

export default function MnemePage() {
  const [project, setProject] = useState(() => sessionStorage.getItem(`olympus.project.${SERVICE}`) || '')
  const [creating, setCreating] = useState(false)
  const [snapshotting, setSnapshotting] = useState(false)
  const [snapCluster, setSnapCluster] = useState('')
  const [viewing, setViewing] = useState<CacheCluster | null>(null)
  const { show } = useToast()

  const clusters = useAsync<CacheCluster[]>(() => {
    if (!project) return Promise.resolve([])
    return api<{ clusters: CacheCluster[] }>(SERVICE, '/clusters', { query: { project } }).then((r) => r.clusters ?? [])
  })

  const snapshots = useAsync<CacheSnapshot[]>(() => {
    if (!project || !snapCluster) return Promise.resolve([])
    return api<{ snapshots: CacheSnapshot[] }>(SERVICE, '/snapshots', { query: { project, cluster: snapCluster } }).then((r) => r.snapshots ?? [])
  })

  async function delCluster(name: string) {
    try {
      await api(SERVICE, `/cluster/${encodeURIComponent(project)}/${encodeURIComponent(name)}`, { method: 'DELETE' })
      show('success', `Deleted cluster ${name}`)
      clusters.refetch()
    } catch (err) {
      show('error', err instanceof Error ? err.message : String(err))
    }
  }

  async function delSnapshot(cluster: string, name: string) {
    try {
      await api(SERVICE, `/snapshot/${encodeURIComponent(project)}/${encodeURIComponent(cluster)}/${encodeURIComponent(name)}`, { method: 'DELETE' })
      show('success', `Deleted snapshot ${name}`)
      snapshots.refetch()
    } catch (err) {
      show('error', err instanceof Error ? err.message : String(err))
    }
  }

  return (
    <div>
      <PageHeader icon="💾" title="Mneme" tagline="Managed in-memory caches — provision Redis clusters, take snapshots.">
        <Button variant="ghost" onClick={() => { clusters.refetch(); snapshots.refetch() }}>Refresh</Button>
        <Button variant="primary" disabled={!project} onClick={() => setCreating(true)}>+ Cluster</Button>
      </PageHeader>
      <ProjectPicker service={SERVICE} onSelect={setProject} />

      {!project ? <EmptyState icon="💾" title="Select a project" hint="Choose a project to work with its caches." /> : (
        <>
          <Card title={`Clusters · ${project}`}>
            {clusters.loading ? <p>Loading…</p> : clusters.data?.length === 0 ? <EmptyState icon="💾" title="No cache clusters" hint="Provision your first cache." /> : (
              <table className="data">
                <thead><tr><th>Name</th><th>State</th><th>Engine</th><th>Node type</th><th>Nodes</th><th>Endpoint</th><th className="right">Actions</th></tr></thead>
                <tbody>
                  {clusters.data?.map((c) => (
                    <tr key={c.id}>
                      <td><span className="mono">{c.name}</span></td>
                      <td><StateBadge state={c.state} /></td>
                      <td><Badge tone="info">{c.engine} {c.engine_version}</Badge></td>
                      <td>{c.node_type}</td>
                      <td className="num">{c.num_nodes}</td>
                      <td className="mono">{c.endpoint || '—'}</td>
                      <td className="right">
                        <Button variant="ghost" onClick={() => { setSnapCluster(c.name); snapshots.refetch() }}>Snapshots</Button>
                        {c.state !== 'deleted' && <Button variant="ghost" onClick={() => setViewing(c)}>Details</Button>}
                        {c.state !== 'deleted' && <Button variant="danger" onClick={() => delCluster(c.name)}>Delete</Button>}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </Card>

          <div className="section-gap" />
          <Card
            title={`Snapshots${snapCluster ? ` · ${snapCluster}` : ''}`}
            actions={<Button variant="ghost" onClick={() => setSnapshotting(true)}>+ Snapshot</Button>}
          >
            <div className="row" style={{ marginBottom: 12 }}>
              <label className="field" style={{ margin: 0, minWidth: 240 }}>
                <span className="field-label">Cluster</span>
                <select value={snapCluster} onChange={(e) => { setSnapCluster(e.target.value); snapshots.refetch() }}>
                  <option value="">— select cluster —</option>
                  {clusters.data?.map((x) => <option key={x.id} value={x.name}>{x.name}</option>)}
                </select>
              </label>
            </div>
            {!snapCluster ? (
              <EmptyState icon="📸" title="Select a cluster" hint="Snapshots are listed per cache cluster." />
            ) : (
              <div>
                {snapshots.loading ? <p>Loading…</p> : snapshots.error ? <div className="form-errors">{snapshots.error}</div> : (snapshots.data ?? []).length === 0 ? <EmptyState icon="📸" title="No snapshots" hint="Take a snapshot to capture this cache." /> : (
                  <table className="data">
                    <thead><tr><th>Name</th><th>State</th><th>Size</th><th>Created</th><th className="right">Actions</th></tr></thead>
                    <tbody>
                      {(snapshots.data ?? []).map((s) => (
                        <tr key={s.id}>
                          <td><span className="mono">{s.name}</span></td>
                          <td><StateBadge state={s.state} /></td>
                          <td className="num">{s.size_mb} MB</td>
                          <td className="muted">{formatTime(s.created_at)}</td>
                          <td className="right"><Button variant="danger" onClick={() => delSnapshot(snapCluster, s.name)}>Delete</Button></td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                )}
              </div>
            )}
          </Card>
        </>
      )}

      <Modal open={creating} onClose={() => setCreating(false)} title="Provision cache cluster" footer={<Button variant="ghost" onClick={() => setCreating(false)}>Cancel</Button>}>
        <CreateCluster project={project} onDone={() => { setCreating(false); clusters.refetch() }} />
      </Modal>

      <Modal open={snapshotting} onClose={() => setSnapshotting(false)} title="Snapshot a cache" footer={<Button variant="ghost" onClick={() => setSnapshotting(false)}>Cancel</Button>}>
        <CreateSnapshot project={project} clusters={clusters.data ?? []} onDone={() => { setSnapshotting(false); snapshots.refetch() }} />
      </Modal>

      {viewing && (
        <Modal open onClose={() => setViewing(null)} title={`Cluster ${viewing.name}`} footer={<Button variant="ghost" onClick={() => setViewing(null)}>Close</Button>}>
          <dl className="kv">
            <div className="kv-row"><dt>State</dt><dd><StateBadge state={viewing.state} /></dd></div>
            <div className="kv-row"><dt>Engine</dt><dd>{viewing.engine} {viewing.engine_version}</dd></div>
            <div className="kv-row"><dt>Node type</dt><dd>{viewing.node_type}</dd></div>
            <div className="kv-row"><dt>Node count</dt><dd>{viewing.num_nodes}</dd></div>
            <div className="kv-row"><dt>Endpoint</dt><dd className="mono">{viewing.endpoint || '—'}</dd></div>
            <div className="kv-row"><dt>Provider ref</dt><dd className="mono">{viewing.provider_ref || '—'}</dd></div>
            <div className="kv-row"><dt>Created</dt><dd>{formatTime(viewing.created_at)}</dd></div>
            <div className="kv-row"><dt>Updated</dt><dd>{formatTime(viewing.updated_at)}</dd></div>
          </dl>
        </Modal>
      )}
    </div>
  )
}