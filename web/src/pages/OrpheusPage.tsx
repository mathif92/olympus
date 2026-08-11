import { useState, type FormEvent } from 'react'
import { Input } from '@heroui/react'
import { PageHeader, ProjectPicker } from '../components/PageHeader'
import { Card, Field, Button, Modal, SelectField, useToast, StateBadge, useAsync, EmptyState, Badge } from '../components/ui'
import { kv } from '../components/format'
import type { Cluster, KubernetesVersion, NodeSize, NodeGroup } from '../api/types'
import { api } from '../api/client'

const SERVICE = 'orpheus'

function CreateCluster({ project, onDone }: { project: string; onDone: () => void }) {
  const { show } = useToast()
  const [name, setName] = useState('')
  const [version, setVersion] = useState('')
  const [region, setRegion] = useState('eu-west-1')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  const versions = useAsync<KubernetesVersion[]>(() => api<{ kubernetes_versions: KubernetesVersion[] }>(SERVICE, '/versions').then((r) => r.kubernetes_versions ?? []))

  async function submit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError('')
    try {
      await api<Cluster>(SERVICE, '/clusters', {
        method: 'POST',
        body: JSON.stringify({ project, name, kubernetes_version: version, region }),
      })
      show('success', `Created cluster ${name}`)
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
        <Field label="Name"><Input value={name} onChange={(e) => setName(e.target.value)} required autoFocus /></Field>
        <SelectField
          label="Kubernetes version"
          value={version}
          onChange={setVersion}
          isRequired
          placeholder="—"
          options={(versions.data ?? []).map((v) => ({ value: v.version, label: `${v.version} (${v.channel})` }))}
        />
      </div>
      <Field label="Region"><Input value={region} onChange={(e) => setRegion(e.target.value)} /></Field>
      {error && <div className="form-errors">{error}</div>}
      <div className="row-end">
        <Button variant="primary" type="submit" disabled={busy}>{busy ? 'Creating…' : 'Create'}</Button>
      </div>
    </form>
  )
}

function CreateNodeGroup({ project, clusterId, onDone }: { project: string; clusterId: string; onDone: () => void }) {
  const { show } = useToast()
  const [name, setName] = useState('')
  const [nodeSize, setNodeSize] = useState('')
  const [minSize, setMinSize] = useState('1')
  const [desiredSize, setDesiredSize] = useState('1')
  const [maxSize, setMaxSize] = useState('3')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  const sizes = useAsync<NodeSize[]>(() => api<{ node_sizes: NodeSize[] }>(SERVICE, '/node-sizes').then((r) => r.node_sizes ?? []))

  async function submit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError('')
    try {
      await api<NodeGroup>(SERVICE, '/nodegroups', {
        method: 'POST',
        body: JSON.stringify({
          project,
          cluster: clusterId,
          name,
          node_size: nodeSize,
          min_size: Number(minSize),
          desired_size: Number(desiredSize),
          max_size: Number(maxSize),
        }),
      })
      show('success', `Created node group ${name}`)
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
        <Field label="Name"><Input value={name} onChange={(e) => setName(e.target.value)} required autoFocus /></Field>
        <SelectField
          label="Node size"
          value={nodeSize}
          onChange={setNodeSize}
          isRequired
          placeholder="—"
          options={(sizes.data ?? []).map((s) => ({ value: s.name, label: `${s.name} (${s.vcpus} vCPU · ${s.memory_gb} GB)` }))}
        />
      </div>
      <div className="form-grid">
        <Field label="Min size"><Input value={minSize} onChange={(e) => setMinSize(e.target.value)} type="number" min={0} /></Field>
        <Field label="Desired size"><Input value={desiredSize} onChange={(e) => setDesiredSize(e.target.value)} type="number" min={0} /></Field>
      </div>
      <Field label="Max size"><Input value={maxSize} onChange={(e) => setMaxSize(e.target.value)} type="number" min={0} /></Field>
      {error && <div className="form-errors">{error}</div>}
      <div className="row-end">
        <Button variant="primary" type="submit" disabled={busy}>{busy ? 'Creating…' : 'Create'}</Button>
      </div>
    </form>
  )
}

export default function OrpheusPage() {
  const [project, setProject] = useState(() => sessionStorage.getItem(`olympus.project.${SERVICE}`) || '')
  const [creating, setCreating] = useState(false)
  const [viewCluster, setViewCluster] = useState<Cluster | null>(null)
  const [creatingNG, setCreatingNG] = useState(false)
  const [scaling, setScaling] = useState<{ cluster: Cluster; ng: NodeGroup } | null>(null)
  const { show } = useToast()

  const clusters = useAsync<Cluster[]>(() => {
    if (!project) return Promise.resolve([])
    return api<{ clusters: Cluster[] }>(SERVICE, '/clusters', { query: { project } }).then((r) => r.clusters ?? [])
  })
  const versions = useAsync<KubernetesVersion[]>(() => api<{ kubernetes_versions: KubernetesVersion[] }>(SERVICE, '/versions').then((r) => r.kubernetes_versions ?? []))
  const sizes = useAsync<NodeSize[]>(() => api<{ node_sizes: NodeSize[] }>(SERVICE, '/node-sizes').then((r) => r.node_sizes ?? []))

  async function delCluster(name: string) {
    try {
      await api(SERVICE, `/cluster/${encodeURIComponent(project)}/${encodeURIComponent(name)}`, { method: 'DELETE' })
      show('success', `Deleted cluster ${name}`)
      clusters.refetch()
    } catch (err) {
      show('error', err instanceof Error ? err.message : String(err))
    }
  }

  async function downloadKubeconfig(cluster: Cluster) {
    try {
      const res = await fetch(`/api/orpheus/cluster/${encodeURIComponent(project)}/${encodeURIComponent(cluster.name)}/kubeconfig`, { credentials: 'same-origin' })
      if (!res.ok) throw new Error(`kubeconfig unavailable (${res.status})`)
      const text = await res.text()
      const blob = new Blob([text], { type: 'application/yaml' })
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = `${cluster.name}-kubeconfig.yaml`
      document.body.appendChild(a)
      a.click()
      a.remove()
      setTimeout(() => URL.revokeObjectURL(url), 5000)
      show('success', `Downloaded kubeconfig for ${cluster.name}`)
    } catch (err) {
      show('error', err instanceof Error ? err.message : String(err))
    }
  }

  const nodeGroups = useAsync<NodeGroup[]>(() => {
    if (!project || !viewCluster) return Promise.resolve([])
    return api<{ node_groups: NodeGroup[] }>(SERVICE, '/nodegroups', { query: { project, cluster: viewCluster.name } }).then((r) => r.node_groups ?? [])
  })

  async function openCluster(c: Cluster) {
    setViewCluster(c)
    nodeGroups.refetch()
  }

  async function delNodeGroup(clusterName: string, name: string) {
    try {
      await api(SERVICE, `/nodegroup/${encodeURIComponent(project)}/${encodeURIComponent(clusterName)}/${encodeURIComponent(name)}`, { method: 'DELETE' })
      show('success', `Deleted node group ${name}`)
      nodeGroups.refetch()
    } catch (err) {
      show('error', err instanceof Error ? err.message : String(err))
    }
  }

  async function scaleNodeGroup(clusterName: string, ng: NodeGroup, desired: string) {
    try {
      await api<NodeGroup>(SERVICE, `/nodegroup/${encodeURIComponent(project)}/${encodeURIComponent(clusterName)}/${encodeURIComponent(ng.name)}/scale`, {
        method: 'POST',
        body: JSON.stringify({ desired_size: Number(desired) }),
      })
      show('success', `Scaled ${ng.name} to ${desired}`)
      setScaling(null)
      nodeGroups.refetch()
    } catch (err) {
      show('error', err instanceof Error ? err.message : String(err))
    }
  }

  return (
    <div>
      <PageHeader icon="☸️" title="Orpheus" tagline="Managed Kubernetes — provision clusters, fetch kubeconfigs, scale node groups.">
        <Button variant="ghost" onPress={() => { clusters.refetch(); versions.refetch(); sizes.refetch() }}>Refresh</Button>
        <Button variant="primary" disabled={!project} onPress={() => setCreating(true)}>+ Cluster</Button>
      </PageHeader>
      <ProjectPicker service={SERVICE} onSelect={setProject} />

      <Card title="Catalogs">
        <div className="row" style={{ gap: 16 }}>
          <Field label="Kubernetes versions" className="m-0" >
            <div className="row">{versions.data?.map((v) => <Badge key={v.version} tone={v.status === 'active' ? 'ok' : 'warn'}>{v.version} {v.channel}</Badge>)}</div>
          </Field>
          <Field label="Node sizes" className="m-0" >
            <div className="row">{sizes.data?.map((s) => <Badge key={s.name} tone="info">{s.name} ({s.vcpus}v · {s.memory_gb}GB)</Badge>)}</div>
          </Field>
        </div>
      </Card>
      <div className="section-gap" />

      {!project ? <EmptyState icon="☸️" title="Select a project" hint="Choose a project to see its clusters." /> : (
        <Card title={`Clusters · ${project}`}>
          {clusters.loading ? <p>Loading…</p> : clusters.data?.length === 0 ? <EmptyState icon="☸️" title="No clusters" hint="Provision your first managed Kubernetes cluster." /> : (
            <table className="data-table">
              <thead><tr><th>Name</th><th>State</th><th>Version</th><th>Region</th><th>Endpoint</th><th className="right">Actions</th></tr></thead>
              <tbody>
                {clusters.data?.map((c) => (
                  <tr key={c.id}>
                    <td><span className="mono">{c.name}</span></td>
                    <td><StateBadge state={c.state} /></td>
                    <td>{c.kubernetes_version}</td>
                    <td className="muted">{c.region}</td>
                    <td className="mono">{c.endpoint || '—'}</td>
                    <td className="right">
                      <Button variant="ghost" size="sm" onPress={() => openCluster(c)}>Manage</Button>
                      {c.state === 'active' && <Button variant="ghost" size="sm" onPress={() => downloadKubeconfig(c)}>kubeconfig</Button>}
                      <Button variant="danger" size="sm" onPress={() => delCluster(c.name)}>Delete</Button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </Card>
      )}

      <Modal open={creating} onClose={() => setCreating(false)} title="Create cluster" footer={<Button variant="ghost" onPress={() => setCreating(false)}>Cancel</Button>}>
        <CreateCluster project={project} onDone={() => { setCreating(false); clusters.refetch() }} />
      </Modal>

      {viewCluster && (
        <Modal open wide onClose={() => setViewCluster(null)}
          title={`Cluster ${viewCluster.name}`}
          footer={
            <div className="row-end">
              <Button variant="ghost" onPress={() => setCreatingNG(true)}>+ Node group</Button>
              <Button variant="ghost" onPress={() => setViewCluster(null)}>Close</Button>
            </div>
          }>
          {kv([
            ['State', <StateBadge state={viewCluster.state} />],
            ['Version', viewCluster.kubernetes_version],
            ['Region', viewCluster.region],
            ['Endpoint', <span className="mono">{viewCluster.endpoint || '—'}</span>],
            ['Provider ref', <span className="mono">{viewCluster.provider_ref || '—'}</span>],
          ])}
          <div className="section-gap" />
          <Card title="Node groups" actions={<Button variant="ghost" size="sm" onPress={() => setCreatingNG(true)}>+ Node group</Button>}>
            {nodeGroups.loading ? <p>Loading…</p> : !nodeGroups.data?.length ? <EmptyState icon="🧩" title="No node groups" hint="Add a node group to run your workloads." /> : (
              <table className="data-table">
                <thead><tr><th>Name</th><th>State</th><th>Size</th><th>Min</th><th>Desired</th><th>Max</th><th className="right">Actions</th></tr></thead>
                <tbody>
                  {nodeGroups.data.map((ng) => (
                    <tr key={ng.id}>
                      <td><span className="mono">{ng.name}</span></td>
                      <td><StateBadge state={ng.state} /></td>
                      <td>{ng.node_size}</td>
                      <td className="num">{ng.min_size}</td>
                      <td className="num">{ng.desired_size}</td>
                      <td className="num">{ng.max_size}</td>
                      <td className="right">
                        <Button variant="ghost" size="sm" onPress={() => setScaling({ cluster: viewCluster, ng })}>Scale</Button>
                        <Button variant="danger" size="sm" onPress={() => delNodeGroup(viewCluster.name, ng.name)}>Delete</Button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </Card>
        </Modal>
      )}

      <Modal open={creatingNG} onClose={() => setCreatingNG(false)} title="Create node group" footer={<Button variant="ghost" onPress={() => setCreatingNG(false)}>Cancel</Button>}>
        {viewCluster && <CreateNodeGroup project={project} clusterId={viewCluster.name} onDone={() => { setCreatingNG(false); nodeGroups.refetch() }} />}
      </Modal>

      {scaling && (
        <Modal open onClose={() => setScaling(null)} title={`Scale ${scaling.ng.name}`} footer={<Button variant="ghost" onPress={() => setScaling(null)}>Cancel</Button>}>
          <form
            onSubmit={(e) => {
              e.preventDefault()
              const v = (e.currentTarget.elements.namedItem('desired') as HTMLInputElement).value
              scaleNodeGroup(scaling.cluster.name, scaling.ng, v)
            }}
          >
            <Field label="Desired size" hint={`min ${scaling.ng.min_size} · max ${scaling.ng.max_size}`}>
              <Input name="desired" type="number" defaultValue={scaling.ng.desired_size} />
            </Field>
            <div className="row-end">
              <Button variant="primary" type="submit">Apply</Button>
            </div>
          </form>
        </Modal>
      )}
    </div>
  )
}