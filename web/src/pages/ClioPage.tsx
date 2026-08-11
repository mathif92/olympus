import { useState, type FormEvent } from 'react'
import { Input } from '@heroui/react'
import { PageHeader, ProjectPicker } from '../components/PageHeader'
import { Card, Field, Button, Modal, SelectField, useToast, StateBadge, useAsync, EmptyState, Badge } from '../components/ui'
import { CopyButton, formatTime, kv } from '../components/format'
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
        <Field label="Name"><Input value={name} onChange={(e) => setName(e.target.value)} required autoFocus /></Field>
        <SelectField
          label="Engine"
          value={engine}
          onChange={(v) => { setEngine(v); setEngineVersion('') }}
          isRequired
          placeholder="—"
          options={Array.from(new Set((engines.data ?? []).map((x) => x.engine))).map((e) => ({ value: e, label: e }))}
        />
      </div>
      <SelectField
        label="Engine version"
        value={engineVersion}
        onChange={setEngineVersion}
        isRequired
        placeholder="—"
        disabled={!engine}
        options={(engines.data ?? []).filter((x) => x.engine === engine).map((x) => ({ value: x.version, label: `${x.version} (${x.status})` }))}
      />
      <SelectField
        label="Instance size"
        value={size}
        onChange={setSize}
        isRequired
        placeholder="—"
        options={(sizes.data ?? []).map((s) => ({ value: s.name, label: `${s.name} (${s.vcpus} vCPU · ${s.memory_gb} GB · ${s.storage_gb} GB)` }))}
      />
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
        <Field label="Snapshot name"><Input value={name} onChange={(e) => setName(e.target.value)} required autoFocus /></Field>
        <SelectField
          label="Database instance"
          value={instance}
          onChange={setInstance}
          isRequired
          placeholder="—"
          options={instances.map((x) => ({ value: x.name, label: `${x.name} (${x.engine} ${x.engine_version})` }))}
        />
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
      footer={<Button variant="primary" onPress={onClose}>Done</Button>}>
      <p className="form-errors" style={{ marginTop: 0 }}>
        The master password is generated at creation and returned only once. Copy it now — Clio never stores or shows it again.
      </p>
      <Field label="Master password">
        <div className="row">
          <Input readOnly className="mono" value={secret?.password ?? ''} style={{ flex: 1 }} />
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
        <Button variant="ghost" onPress={() => { instances.refetch(); snapshots.refetch() }}>Refresh</Button>
        <Button variant="primary" disabled={!project} onPress={() => setCreating(true)}>+ Instance</Button>
      </PageHeader>
      <ProjectPicker service={SERVICE} onSelect={setProject} />

      {!project ? <EmptyState icon="🗃️" title="Select a project" hint="Choose a project to work with its databases." /> : (
        <>
          <Card title={`Instances · ${project}`}>
            {instances.loading ? <p>Loading…</p> : instances.data?.length === 0 ? <EmptyState icon="🗃️" title="No database instances" hint="Provision your first managed database." /> : (
              <table className="data-table">
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
                        <Button variant="ghost" size="sm" onPress={() => { setSnapInstance(inst.name); snapshots.refetch() }}>Snapshots</Button>
                        {inst.state === 'stopped' && <Button variant="ghost" size="sm" onPress={() => action(inst.name, 'start')}>Start</Button>}
                        {inst.state === 'active' && <Button variant="ghost" size="sm" onPress={() => action(inst.name, 'stop')}>Stop</Button>}
                        {inst.state !== 'deleted' && <Button variant="ghost" size="sm" onPress={() => setViewing(inst)}>Details</Button>}
                        {inst.state !== 'deleted' && <Button variant="danger" size="sm" onPress={() => delInstance(inst.name)}>Delete</Button>}
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
            actions={<Button variant="ghost" size="sm" onPress={() => setSnapshotting(true)}>+ Snapshot</Button>}
          >
            <div style={{ marginBottom: 12 }}>
              <SelectField
                label="Instance"
                value={snapInstance}
                onChange={(v) => { setSnapInstance(v); snapshots.refetch() }}
                placeholder="— select instance —"
                options={(instances.data ?? []).map((x) => ({ value: x.name, label: x.name }))}
              />
            </div>
            {!snapInstance ? (
              <EmptyState icon="📸" title="Select an instance" hint="Snapshots are listed per instance." />
            ) : (
              <div>
                {snapshots.loading ? <p>Loading…</p> : snapshots.error ? <div className="form-errors">{snapshots.error}</div> : (snapshots.data ?? []).length === 0 ? <EmptyState icon="📸" title="No snapshots" hint="Take a snapshot to capture this database." /> : (
                  <table className="data-table">
                    <thead><tr><th>Name</th><th>State</th><th>Size</th><th>Created</th><th className="right">Actions</th></tr></thead>
                    <tbody>
                      {(snapshots.data ?? []).map((s) => (
                        <tr key={s.id}>
                          <td><span className="mono">{s.name}</span></td>
                          <td><StateBadge state={s.state} /></td>
                          <td className="num">{s.size_gb} GB</td>
                          <td className="muted">{formatTime(s.created_at)}</td>
                          <td className="right"><Button variant="danger" size="sm" onPress={() => delSnapshot(snapInstance, s.name)}>Delete</Button></td>
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

      <Modal open={creating} onClose={() => setCreating(false)} title="Provision database instance" footer={<Button variant="ghost" onPress={() => setCreating(false)}>Cancel</Button>}>
        <CreateInstance project={project} onDone={(pw, name) => { setCreating(false); setSecret({ password: pw, name }); instances.refetch() }} />
      </Modal>

      <MasterPasswordModal secret={secret} onClose={() => setSecret(null)} />

      <Modal open={snapshotting} onClose={() => setSnapshotting(false)} title="Snapshot a database" footer={<Button variant="ghost" onPress={() => setSnapshotting(false)}>Cancel</Button>}>
        <CreateSnapshot project={project} instances={instances.data ?? []} onDone={() => { setSnapshotting(false); snapshots.refetch() }} />
      </Modal>

      {viewing && (
        <Modal open onClose={() => setViewing(null)} title={`Instance ${viewing.name}`} footer={<Button variant="ghost" onPress={() => setViewing(null)}>Close</Button>}>
          {kv([
            ['State', <StateBadge state={viewing.state} />],
            ['Engine', `${viewing.engine} ${viewing.engine_version}`],
            ['Size', viewing.size],
            ['Allocated storage', `${viewing.allocated_storage_gb} GB`],
            ['Endpoint', <span className="mono">{viewing.endpoint || '—'}</span>],
            ['Master user', <span className="mono">{viewing.master_username || '—'}</span>],
            ['Provider ref', <span className="mono">{viewing.provider_ref || '—'}</span>],
            ['Created', formatTime(viewing.created_at)],
            ['Updated', formatTime(viewing.updated_at)],
          ])}
        </Modal>
      )}
    </div>
  )
}