import { useState, type FormEvent } from 'react'
import { PageHeader, ProjectPicker } from '../components/PageHeader'
import { Card, Field, Button, Modal, useToast, StateBadge, useAsync, EmptyState, Badge } from '../components/ui'
import { CopyButton, formatTime } from '../components/format'
import type { Instance, InstanceType, KeyPair, KeyPairCreated, SecurityGroup, Volume, Snapshot } from '../api/types'
import { api } from '../api/client'

const SERVICE = 'hephaestus'

function CreateInstance({ project, onDone }: { project: string; onDone: () => void }) {
  const { show } = useToast()
  const [name, setName] = useState('')
  const [type, setType] = useState('')
  const [imageId, setImageId] = useState('olympus-ami-linux-2')
  const [keyPair, setKeyPair] = useState('')
  const [volumeGb, setVolumeGb] = useState('')
  const [secGroups, setSecGroups] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  const types = useAsync<InstanceType[]>(() => api<{ instance_types: InstanceType[] }>(SERVICE, '/types').then((r) => r.instance_types ?? []))
  const keypairs = useAsync<KeyPair[]>(() => api<{ key_pairs: KeyPair[] }>(SERVICE, `/keypairs/${encodeURIComponent(project)}`).then((r) => r.key_pairs ?? []))
  const groups = useAsync<SecurityGroup[]>(() => api<{ security_groups: SecurityGroup[] }>(SERVICE, `/security-groups/${encodeURIComponent(project)}`).then((r) => r.security_groups ?? []))

  async function submit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError('')
    try {
      await api<Instance>(SERVICE, '/instances', {
        method: 'POST',
        body: JSON.stringify({
          project,
          name,
          type,
          image_id: imageId,
          key_pair: keyPair || undefined,
          volume_gb: volumeGb ? Number(volumeGb) : undefined,
          security_groups: secGroups.split(',').map((s) => s.trim()).filter(Boolean),
        }),
      })
      show('success', `Launched instance ${name}`)
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
        <Field label="Instance type">
          <select value={type} onChange={(e) => setType(e.target.value)} required>
            <option value="">—</option>
            {types.data?.map((t) => (
              <option key={t.name} value={t.name}>
                {t.name} ({t.vcpus} vCPU · {t.memory_gb} GB · {t.storage_gb} GB)
              </option>
            ))}
          </select>
        </Field>
      </div>
      <div className="form-grid">
        <Field label="Image id"><input value={imageId} onChange={(e) => setImageId(e.target.value)} /></Field>
        <Field label="Key pair">
          <select value={keyPair} onChange={(e) => setKeyPair(e.target.value)}>
            <option value="">— none —</option>
            {keypairs.data?.map((k) => <option key={k.id} value={k.name}>{k.name}</option>)}
          </select>
        </Field>
      </div>
      <div className="form-grid">
        <Field label="Boot volume (GB)" hint="defaults to the type's storage"><input value={volumeGb} onChange={(e) => setVolumeGb(e.target.value)} type="number" min={1} /></Field>
        <Field label="Security groups (comma-separated)">
          <input value={secGroups} onChange={(e) => setSecGroups(e.target.value)} placeholder="web,db" list="hph-secgroups" />
          <datalist id="hph-secgroups">
            {groups.data?.map((g) => <option key={g.id} value={g.name} />)}
          </datalist>
        </Field>
      </div>
      {error && <div className="form-errors">{error}</div>}
      <div className="row-end" style={{ marginTop: 8 }}>
        <Button variant="primary" type="submit" disabled={busy}>{busy ? 'Launching…' : 'Launch'}</Button>
      </div>
    </form>
  )
}

function CreateKeyPair({ project, onDone }: { project: string; onDone: () => void }) {
  const { show } = useToast()
  const [name, setName] = useState('')
  const [result, setResult] = useState<KeyPairCreated | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  async function submit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError('')
    try {
      const kp = await api<KeyPairCreated>(SERVICE, '/keypairs', {
        method: 'POST',
        body: JSON.stringify({ project, name }),
      })
      setResult(kp)
      show('success', `Generated key pair ${name}`)
      onDone()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <form onSubmit={submit}>
      <Field label="Key pair name"><input value={name} onChange={(e) => setName(e.target.value)} required autoFocus /></Field>
      {error && <div className="form-errors">{error}</div>}
      <div className="row-end">
        <Button variant="primary" type="submit" disabled={busy || !!result}>{busy ? 'Generating…' : 'Generate'}</Button>
      </div>
      {result && (
        <div className="section-gap" />
      )}
      {result && (
        <Card title="Private key (shown once)" >
          <p style={{ marginTop: 0 }} className="form-errors">
            Copy this now — Hephaestus persists only the public key; this private key will not be returned again.
          </p>
          <div className="code-block">{result.private_key}</div>
          <div className="row" style={{ marginTop: 8 }}>
            <CopyButton text={result.private_key} label="Copy private key" />
          </div>
        </Card>
      )}
    </form>
  )
}

function CreateSecurityGroup({ project, onDone }: { project: string; onDone: () => void }) {
  const { show } = useToast()
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [rulesText, setRulesText] = useState('22:0.0.0.0/0')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  async function submit(e: FormEvent) {
    e.preventDefault()
    const rules = rulesText.split('\n').map((l) => l.trim()).filter(Boolean).map((l) => {
      const [port, cidr] = l.split(':')
      return { port: Number(port), cidr: cidr || '0.0.0.0/0' }
    }).filter((r) => !isNaN(r.port))
    setBusy(true)
    setError('')
    try {
      await api<SecurityGroup>(SERVICE, '/security-groups', {
        method: 'POST',
        body: JSON.stringify({ project, name, description, rules }),
      })
      show('success', `Created security group ${name}`)
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
        <Field label="Description"><input value={description} onChange={(e) => setDescription(e.target.value)} /></Field>
      </div>
      <Field label="Rules" hint='one per line: "port:cidr" e.g. 22:0.0.0.0/0 — leave empty for the default SSH rule'>
        <textarea value={rulesText} onChange={(e) => setRulesText(e.target.value)} />
      </Field>
      {error && <div className="form-errors">{error}</div>}
      <div className="row-end">
        <Button variant="primary" type="submit" disabled={busy}>{busy ? 'Creating…' : 'Create'}</Button>
      </div>
    </form>
  )
}

function CreateVolume({ project, onDone }: { project: string; onDone: () => void }) {
  const { show } = useToast()
  const [name, setName] = useState('')
  const [sizeGb, setSizeGb] = useState('10')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  async function submit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError('')
    try {
      await api<Volume>(SERVICE, '/volumes', {
        method: 'POST',
        body: JSON.stringify({ project, name, size_gb: Number(sizeGb), type: 'gp2' }),
      })
      show('success', `Created volume ${name}`)
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
        <Field label="Size (GB)"><input value={sizeGb} onChange={(e) => setSizeGb(e.target.value)} type="number" min={1} /></Field>
      </div>
      {error && <div className="form-errors">{error}</div>}
      <div className="row-end">
        <Button variant="primary" type="submit" disabled={busy}>{busy ? 'Creating…' : 'Create'}</Button>
      </div>
    </form>
  )
}

function CreateSnapshot({ project, volumes, onDone }: { project: string; volumes: Volume[]; onDone: () => void }) {
  const { show } = useToast()
  const [name, setName] = useState('')
  const [volume, setVolume] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  async function submit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError('')
    try {
      await api<Snapshot>(SERVICE, '/snapshots', {
        method: 'POST',
        body: JSON.stringify({ project, name, volume }),
      })
      show('success', `Snapshotted ${volume} -> ${name}`)
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
        <Field label="Volume">
          <select value={volume} onChange={(e) => setVolume(e.target.value)} required>
            <option value="">—</option>
            {volumes.map((v) => <option key={v.id} value={v.name}>{v.name} ({v.size_gb} GB)</option>)}
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

export default function HephaestusPage() {
  const [project, setProject] = useState(() => sessionStorage.getItem(`olympus.project.${SERVICE}`) || '')
  const [tab, setTab] = useState<'instances' | 'keypairs' | 'groups' | 'volumes' | 'snapshots'>('instances')
  const [creating, setCreating] = useState(false)
  const [viewing, setViewing] = useState<Instance | null>(null)
  const { show } = useToast()

  const instances = useAsync<Instance[]>(() => {
    if (!project) return Promise.resolve([])
    return api<{ instances: Instance[] }>(SERVICE, '/instances', { query: { project } }).then((r) => r.instances ?? [])
  })
  const types = useAsync<InstanceType[]>(() => api<{ instance_types: InstanceType[] }>(SERVICE, '/types').then((r) => r.instance_types ?? []))
  const keypairs = useAsync<KeyPair[]>(() => {
    if (!project) return Promise.resolve([])
    return api<{ key_pairs: KeyPair[] }>(SERVICE, `/keypairs/${encodeURIComponent(project)}`).then((r) => r.key_pairs ?? [])
  })
  const groups = useAsync<SecurityGroup[]>(() => {
    if (!project) return Promise.resolve([])
    return api<{ security_groups: SecurityGroup[] }>(SERVICE, `/security-groups/${encodeURIComponent(project)}`).then((r) => r.security_groups ?? [])
  })
  const volumes = useAsync<Volume[]>(() => {
    if (!project) return Promise.resolve([])
    return api<{ volumes: Volume[] }>(SERVICE, '/volumes', { query: { project } }).then((r) => r.volumes ?? [])
  })
  const snapshots = useAsync<Snapshot[]>(() => {
    if (!project) return Promise.resolve([])
    return api<{ snapshots: Snapshot[] }>(SERVICE, '/snapshots', { query: { project } }).then((r) => r.snapshots ?? [])
  })

  async function instanceAction(name: string, action: string) {
    try {
      await api<Instance>(SERVICE, `/instance/${encodeURIComponent(project)}/${encodeURIComponent(name)}/${action}`, { method: 'POST' })
      show('success', `${action} ${name}`)
      instances.refetch()
    } catch (err) {
      show('error', err instanceof Error ? err.message : String(err))
    }
  }

  async function terminate(name: string) {
    try {
      await api(SERVICE, `/instance/${encodeURIComponent(project)}/${encodeURIComponent(name)}`, { method: 'DELETE' })
      show('success', `Terminated ${name}`)
      instances.refetch()
    } catch (err) {
      show('error', err instanceof Error ? err.message : String(err))
    }
  }

  async function deleteVolume(name: string) {
    try {
      await api(SERVICE, `/volume/${encodeURIComponent(project)}/${encodeURIComponent(name)}`, { method: 'DELETE' })
      show('success', `Deleted volume ${name}`)
      volumes.refetch()
    } catch (err) {
      show('error', err instanceof Error ? err.message : String(err))
    }
  }

  async function refreshAll() {
    instances.refetch(); types.refetch(); keypairs.refetch(); groups.refetch(); volumes.refetch(); snapshots.refetch()
  }

  const createButton = () => {
    if (!project) return null
    const labels: Record<string, string> = {
      instances: '+ Launch instance',
      keypairs: '+ Key pair',
      groups: '+ Security group',
      volumes: '+ Volume',
      snapshots: '+ Snapshot',
    }
    return <Button variant="primary" onClick={() => { setCreating(true) }}>{labels[tab]}</Button>
  }

  return (
    <div>
      <PageHeader icon="⚙️" title="Hephaestus" tagline="Compute control plane — launch instances, issue key pairs, manage volumes.">
        <Button variant="ghost" onClick={refreshAll}>Refresh</Button>
        {createButton()}
      </PageHeader>
      <ProjectPicker service={SERVICE} onSelect={setProject} />

      <div className="tabs">
        {(['instances', 'keypairs', 'groups', 'volumes', 'snapshots'] as const).map((t) => (
          <button key={t} type="button" className={`tab ${tab === t ? 'active' : ''}`} onClick={() => setTab(t)} style={{ border: 'none', background: 'none', cursor: 'pointer', fontFamily: 'inherit', fontSize: 14 }}>
            {t}
          </button>
        ))}
      </div>

      {!project && <EmptyState icon="🖥️" title="Select a project" hint="Choose a project to operate on or create one in the picker above." />}

      {tab === 'instances' && project && (
        <Card title={`Instances · ${project}`}>
          {instances.loading ? <p>Loading…</p> : instances.data?.length === 0 ? <EmptyState icon="🖥️" title="No instances" hint="Launch your first instance." /> : (
            <table className="data">
              <thead>
                <tr><th>Name</th><th>State</th><th>Type</th><th>Image</th><th>IPs</th><th>Key</th><th className="right">Actions</th></tr>
              </thead>
              <tbody>
                {instances.data?.map((i) => (
                  <tr key={i.id}>
                    <td><span className="mono">{i.name}</span></td>
                    <td><StateBadge state={i.state} /></td>
                    <td>{i.instance_type}</td>
                    <td className="muted">{i.image_id}</td>
                    <td><span className="mono">{i.private_ip || ''}{i.public_ip ? ` / ${i.public_ip}` : ''}</span></td>
                    <td className="muted">{i.key_pair_name || '—'}</td>
                    <td className="right">
                      {i.state === 'stopped' && <Button variant="ghost" onClick={() => instanceAction(i.name, 'start')}>Start</Button>}
                      {i.state === 'running' && <Button variant="ghost" onClick={() => instanceAction(i.name, 'stop')}>Stop</Button>}
                      {i.state !== 'terminated' && <Button variant="ghost" onClick={() => setViewing(i)}>View</Button>}
                      {i.state !== 'terminated' && <Button variant="danger" onClick={() => terminate(i.name)}>Terminate</Button>}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </Card>
      )}

      {tab === 'keypairs' && project && (
        <Card title={`Key pairs · ${project}`}>
          {keypairs.loading ? <p>Loading…</p> : keypairs.data?.length === 0 ? <EmptyState icon="🔑" title="No key pairs" hint="Generate an SSH key pair to use with instances." /> : (
            <table className="data">
              <thead><tr><th>Name</th><th>Fingerprint</th><th>Public key</th><th>Created</th></tr></thead>
              <tbody>
                {keypairs.data?.map((k) => (
                  <tr key={k.id}>
                    <td><span className="mono">{k.name}</span></td>
                    <td><span className="mono">{k.fingerprint}</span></td>
                    <td><span className="mono" style={{ fontSize: 11 }}>{k.public_key.slice(0, 48)}…</span></td>
                    <td className="muted">{formatTime(k.created_at)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </Card>
      )}

      {tab === 'groups' && project && (
        <Card title={`Security groups · ${project}`}>
          {groups.loading ? <p>Loading…</p> : groups.data?.length === 0 ? <EmptyState icon="🛡️" title="No security groups" hint="Create a group to open ports." /> : (
            <table className="data">
              <thead><tr><th>Name</th><th>Description</th><th>Rules</th></tr></thead>
              <tbody>
                {groups.data?.map((g) => (
                  <tr key={g.id}>
                    <td><span className="mono">{g.name}</span></td>
                    <td className="muted">{g.description || '—'}</td>
                    <td>
                      {g.rules.map((r) => (
                        <Badge key={`${r.port}-${r.cidr}`} tone="info">{r.port} / {r.cidr}</Badge>
                      ))}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </Card>
      )}

      {tab === 'volumes' && project && (
        <Card title={`Volumes · ${project}`}>
          {volumes.loading ? <p>Loading…</p> : volumes.data?.length === 0 ? <EmptyState icon="💽" title="No volumes" hint="Create a block volume." /> : (
            <table className="data">
              <thead><tr><th>Name</th><th>State</th><th>Size</th><th>Type</th><th>Attached to</th><th className="right">Actions</th></tr></thead>
              <tbody>
                {volumes.data?.map((v) => (
                  <tr key={v.id}>
                    <td><span className="mono">{v.name}</span></td>
                    <td><StateBadge state={v.state} /></td>
                    <td className="num">{v.size_gb} GB</td>
                    <td className="muted">{v.volume_type}</td>
                    <td className="muted">{v.instance_id ? <span className="mono">{v.instance_id.slice(0, 12)}…</span> : '—'}</td>
                    <td className="right">
                      {!v.instance_id && <Button variant="danger" onClick={() => deleteVolume(v.name)}>Delete</Button>}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </Card>
      )}

      {tab === 'snapshots' && project && (
        <Card title={`Snapshots · ${project}`}>
          {snapshots.loading ? <p>Loading…</p> : snapshots.data?.length === 0 ? <EmptyState icon="📸" title="No snapshots" hint="Snapshot a volume to capture its state." /> : (
            <table className="data">
              <thead><tr><th>Name</th><th>State</th><th>Size</th><th>Volume</th><th>Created</th></tr></thead>
              <tbody>
                {snapshots.data?.map((s) => (
                  <tr key={s.id}>
                    <td><span className="mono">{s.name}</span></td>
                    <td><StateBadge state={s.state} /></td>
                    <td className="num">{s.size_gb} GB</td>
                    <td className="muted">{s.volume_id ? <span className="mono">{s.volume_id.slice(0, 12)}…</span> : '—'}</td>
                    <td className="muted">{formatTime(s.created_at)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </Card>
      )}

      <Modal
        open={creating}
        onClose={() => setCreating(false)}
        title={tab === 'instances' ? 'Launch instance' : tab === 'keypairs' ? 'Generate key pair' : tab === 'groups' ? 'Create security group' : tab === 'volumes' ? 'Create volume' : 'Create snapshot'}
        footer={<Button variant="ghost" onClick={() => setCreating(false)}>Cancel</Button>}
      >
        {tab === 'instances' && <CreateInstance project={project} onDone={() => { setCreating(false); instances.refetch() }} />}
        {tab === 'keypairs' && <CreateKeyPair project={project} onDone={() => { setCreating(false); keypairs.refetch() }} />}
        {tab === 'groups' && <CreateSecurityGroup project={project} onDone={() => { setCreating(false); groups.refetch() }} />}
        {tab === 'volumes' && <CreateVolume project={project} onDone={() => { setCreating(false); volumes.refetch() }} />}
        {tab === 'snapshots' && <CreateSnapshot project={project} volumes={volumes.data ?? []} onDone={() => { setCreating(false); snapshots.refetch() }} />}
      </Modal>

      {viewing && (
        <Modal open onClose={() => setViewing(null)} title={`Instance ${viewing.name}`}
          footer={<Button variant="ghost" onClick={() => setViewing(null)}>Close</Button>}>
          <dl className="kv">
            <div className="kv-row"><dt>State</dt><dd><StateBadge state={viewing.state} /></dd></div>
            <div className="kv-row"><dt>Instance type</dt><dd>{viewing.instance_type}</dd></div>
            <div className="kv-row"><dt>Image</dt><dd>{viewing.image_id}</dd></div>
            <div className="kv-row"><dt>Private IP</dt><dd className="mono">{viewing.private_ip || '—'}</dd></div>
            <div className="kv-row"><dt>Public IP</dt><dd className="mono">{viewing.public_ip || '—'}</dd></div>
            <div className="kv-row"><dt>Key pair</dt><dd>{viewing.key_pair_name || '—'}</dd></div>
            <div className="kv-row"><dt>Provider ref</dt><dd className="mono">{viewing.provider_ref || '—'}</dd></div>
            <div className="kv-row"><dt>Launched by</dt><dd>{viewing.launched_by}</dd></div>
            <div className="kv-row"><dt>Launched</dt><dd>{formatTime(viewing.launched_at)}</dd></div>
            <div className="kv-row"><dt>Terminated</dt><dd>{formatTime(viewing.terminated_at)}</dd></div>
            <div className="kv-row"><dt>Updated</dt><dd>{formatTime(viewing.updated_at)}</dd></div>
          </dl>
        </Modal>
      )}
    </div>
  )
}