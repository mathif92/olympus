import { useState, type FormEvent } from 'react'
import { PageHeader, ProjectPicker } from '../components/PageHeader'
import { Card, Field, Button, Modal, useToast, StateBadge, useAsync, EmptyState, Badge } from '../components/ui'
import { CopyButton, formatTime } from '../components/format'
import type { DBInstance, DatabaseEngine, InstanceSize, DBSnapshot } from '../api/types'
import { api } from '../api/client'

const SERVICE = 'clio'

function CreateInstance({ project, onDone }: { project: string; onDone: (pw: string, name: string) => void }) {
  const { show } = useToast()
  const [name, setName] = useState('')
  const [engine, setEngine] = useState('')
  const [engineVersion, setEngineVersion] = useState('')
  const [size, setSize] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  const engines = useAsync<DatabaseEngine[]>(() => api<{ database_engines: DatabaseEngine[] }>(SERVICE, '/engines').then((r) => r.database_engines ?? []))
  const sizes = useAsync<InstanceSize[]>(() => api<{ instance_sizes: InstanceSize[] }>(SERVICE, '/instance-sizes').then((r) => r.instance_sizes ?? []))

  async function submit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError('')
    try {
      const inst = await api<DBInstance>(SERVICE, '/instances', {
        method: 'POST',
        body: JSON.stringify({ project, name, engine, engine_version: engineVersion, size }),
      })
      show('success', `Created ${engine} instance ${name}`)
      onDone(inst.master_password ?? '', name)
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
      <Field label="Instance size">
        <select value={size} onChange={(e) => setSize(e.target.value)} required>
          <option value="">—</option>
          {sizes.data?.map((s) => <option key={s.name} value={s.name}>{s.name} ({s.vcpus} vCPU · {s.memory_gb} GB · {s.storage_gb} GB)</option>)}
        </select>
      </Field>
      {error && <div className="form-errors">{error}</div>}
      <div className="row-end">
        <Button variant="primary" type="submit" disabled={busy}>{busy ? 'Creating…' : 'Create'}</Button>
      </div>
    </form>
  )
}

function CreateSnapshot({ project, instances, onDone }: { project: string; instances: DBInstance[]; onDone: () => void }) {
  const { show } = useToast()
  const [name, setName] = useState('')
  const [instance, setInstance] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  async function submit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError('')
    try {
      await api<DBSnapshot>(SERVICE, '/snapshots', {
        method: 'POST',
        body: JSON.stringify({ project, instance, name }),
      })
      show('success', `Snapshotted ${instance} -> ${name}`)
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
        <Field label="Database instance">
          <select value={instance} onChange={(e) => setInstance(e.target.value)} required>
            <option value="">—</option>
            {instances.map((x) => <option key={x.id} value={x.name}>{x.name} ({x.engine} {x.engine_version})</option>)}
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

function MasterPasswordModal({ secret, onClose }: { secret: { password: string; name: string } | null; onClose: () => void }) {
  return (
    <Modal open={!!secret} onClose={onClose} title={`Master credentials · ${secret?.name ?? ''}`}
      footer={<Button variant="primary" onClick={onClose}>Done</Button>}>
      <p style={{ marginTop: 0 }} className="form-errors">
        The master password is generated at creation and returned only once. Copy it now — Clio never stores or shows it again.
      </p>
      <Field label="Master password">
        <div className="row">
          <input readOnly className="mono" value={secret?.password ?? ''} style={{ flex: 1 }} />
          <CopyButton text={secret?.password ?? ''} label="Copy" />
        </div>
      </Field>
    </Modal>
  )
}

export default function ClioPage() {
  const [project, setProject] = useState(() => sessionStorage.getItem(`olympus.project.${SERVICE}`) || '')
  const [creating, setCreating] = useState(false)
  const [secret, setSecret] = useState<{ password: string; name: string } | null>(null)
  const [snapshotting, setSnapshotting] = useState(false)
  const [snapInstance, setSnapInstance] = useState('')
  const [viewing, setViewing] = useState<DBInstance | null>(null)
  const { show } = useToast()

  const instances = useAsync<DBInstance[]>(() => {
    if (!project) return Promise.resolve([])
    return api<{ instances: DBInstance[] }>(SERVICE, '/instances', { query: { project } }).then((r) => r.instances ?? [])
  })

  // Snapshots are listed per instance.
  const snapshots = useAsync<DBSnapshot[]>(() => {
    if (!project || !snapInstance) return Promise.resolve([])
    return api<{ snapshots: DBSnapshot[] }>(SERVICE, '/snapshots', { query: { project, instance: snapInstance } }).then((r) => r.snapshots ?? [])
  })

  async function action(name: string, act: string) {
    try {
      await api<DBInstance>(SERVICE, `/instance/${encodeURIComponent(project)}/${encodeURIComponent(name)}/${act}`, { method: 'POST' })
      show('success', `${act} ${name}`)
      instances.refetch()
    } catch (err) {
      show('error', err instanceof Error ? err.message : String(err))
    }
  }

  async function delInstance(name: string) {
    try {
      await api(SERVICE, `/instance/${encodeURIComponent(project)}/${encodeURIComponent(name)}`, { method: 'DELETE' })
      show('success', `Deleted instance ${name}`)
      instances.refetch()
    } catch (err) {
      show('error', err instanceof Error ? err.message : String(err))
    }
  }

  async function delSnapshot(instance: string, name: string) {
    try {
      await api(SERVICE, `/snapshot/${encodeURIComponent(project)}/${encodeURIComponent(instance)}/${encodeURIComponent(name)}`, { method: 'DELETE' })
      show('success', `Deleted snapshot ${name}`)
      snapshots.refetch()
    } catch (err) {
      show('error', err instanceof Error ? err.message : String(err))
    }
  }

  return (
    <div>
      <PageHeader icon="🗃️" title="Clio" tagline="Managed relational databases — provision engines, start/stop, snapshot.">
        <Button variant="ghost" onClick={() => { instances.refetch(); snapshots.refetch() }}>Refresh</Button>
        <Button variant="primary" disabled={!project} onClick={() => setCreating(true)}>+ Instance</Button>
      </PageHeader>
      <ProjectPicker service={SERVICE} onSelect={setProject} />

      {!project ? <EmptyState icon="🗃️" title="Select a project" hint="Choose a project to work with its databases." /> : (
        <>
          <Card title={`Instances · ${project}`}>
            {instances.loading ? <p>Loading…</p> : instances.data?.length === 0 ? <EmptyState icon="🗃️" title="No database instances" hint="Provision your first managed database." /> : (
              <table className="data">
                <thead><tr><th>Name</th><th>State</th><th>Engine</th><th>Size</th><th>Storage</th><th>Endpoint</th><th>User</th><th className="right">Actions</th></tr></thead>
                <tbody>
                  {instances.data?.map((inst) => (
                    <tr key={inst.id}>
                      <td><span className="mono">{inst.name}</span></td>
                      <td><StateBadge state={inst.state} /></td>
                      <td><Badge tone="info">{inst.engine} {inst.engine_version}</Badge></td>
                      <td>{inst.size}</td>
                      <td className="num">{inst.allocated_storage_gb} GB</td>
                      <td className="mono">{inst.endpoint || '—'}</td>
                      <td className="mono">{inst.master_username || '—'}</td>
                      <td className="right">
                        <Button variant="ghost" onClick={() => { setSnapInstance(inst.name); snapshots.refetch() }}>Snapshots</Button>
                        {inst.state === 'stopped' && <Button variant="ghost" onClick={() => action(inst.name, 'start')}>Start</Button>}
                        {inst.state === 'active' && <Button variant="ghost" onClick={() => action(inst.name, 'stop')}>Stop</Button>}
                        {inst.state !== 'deleted' && <Button variant="ghost" onClick={() => setViewing(inst)}>Details</Button>}
                        {inst.state !== 'deleted' && <Button variant="danger" onClick={() => delInstance(inst.name)}>Delete</Button>}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </Card>

          <div className="section-gap" />
          <Card
            title={`Snapshots${snapInstance ? ` · ${snapInstance}` : ''}`}
            actions={<Button variant="ghost" onClick={() => setSnapshotting(true)}>+ Snapshot</Button>}
          >
            <div className="row" style={{ marginBottom: 12 }}>
              <label className="field" style={{ margin: 0, minWidth: 240 }}>
                <span className="field-label">Instance</span>
                <select value={snapInstance} onChange={(e) => { setSnapInstance(e.target.value); snapshots.refetch() }}>
                  <option value="">— select instance —</option>
                  {instances.data?.map((x) => <option key={x.id} value={x.name}>{x.name}</option>)}
                </select>
              </label>
            </div>
            {!snapInstance ? (
              <EmptyState icon="📸" title="Select an instance" hint="Snapshots are listed per instance." />
            ) : (
              <div>
                {snapshots.loading ? <p>Loading…</p> : snapshots.error ? <div className="form-errors">{snapshots.error}</div> : (snapshots.data ?? []).length === 0 ? <EmptyState icon="📸" title="No snapshots" hint="Take a snapshot to capture this database." /> : (
                  <table className="data">
                    <thead><tr><th>Name</th><th>State</th><th>Size</th><th>Created</th><th className="right">Actions</th></tr></thead>
                    <tbody>
                      {(snapshots.data ?? []).map((s) => (
                        <tr key={s.id}>
                          <td><span className="mono">{s.name}</span></td>
                          <td><StateBadge state={s.state} /></td>
                          <td className="num">{s.size_gb} GB</td>
                          <td className="muted">{formatTime(s.created_at)}</td>
                          <td className="right"><Button variant="danger" onClick={() => delSnapshot(snapInstance, s.name)}>Delete</Button></td>
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

      <Modal open={creating} onClose={() => setCreating(false)} title="Provision database instance" footer={<Button variant="ghost" onClick={() => setCreating(false)}>Cancel</Button>}>
        <CreateInstance project={project} onDone={(pw, name) => { setCreating(false); setSecret({ password: pw, name }); instances.refetch() }} />
      </Modal>

      <MasterPasswordModal secret={secret} onClose={() => setSecret(null)} />

      <Modal open={snapshotting} onClose={() => setSnapshotting(false)} title="Snapshot a database" footer={<Button variant="ghost" onClick={() => setSnapshotting(false)}>Cancel</Button>}>
        <CreateSnapshot project={project} instances={instances.data ?? []} onDone={() => { setSnapshotting(false); snapshots.refetch() }} />
      </Modal>

      {viewing && (
        <Modal open onClose={() => setViewing(null)} title={`Instance ${viewing.name}`} footer={<Button variant="ghost" onClick={() => setViewing(null)}>Close</Button>}>
          <dl className="kv">
            <div className="kv-row"><dt>State</dt><dd><StateBadge state={viewing.state} /></dd></div>
            <div className="kv-row"><dt>Engine</dt><dd>{viewing.engine} {viewing.engine_version}</dd></div>
            <div className="kv-row"><dt>Size</dt><dd>{viewing.size}</dd></div>
            <div className="kv-row"><dt>Allocated storage</dt><dd>{viewing.allocated_storage_gb} GB</dd></div>
            <div className="kv-row"><dt>Endpoint</dt><dd className="mono">{viewing.endpoint || '—'}</dd></div>
            <div className="kv-row"><dt>Master user</dt><dd className="mono">{viewing.master_username || '—'}</dd></div>
            <div className="kv-row"><dt>Provider ref</dt><dd className="mono">{viewing.provider_ref || '—'}</dd></div>
            <div className="kv-row"><dt>Created</dt><dd>{formatTime(viewing.created_at)}</dd></div>
            <div className="kv-row"><dt>Updated</dt><dd>{formatTime(viewing.updated_at)}</dd></div>
          </dl>
        </Modal>
      )}
    </div>
  )
}