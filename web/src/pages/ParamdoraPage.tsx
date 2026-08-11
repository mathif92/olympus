import { useState, type FormEvent } from 'react'
import { Input, TextArea } from '@heroui/react'
import { PageHeader, ProjectPicker } from '../components/PageHeader'
import { Card, Field, Button, Modal, SelectField, useToast, StateBadge, useAsync, EmptyState, Badge } from '../components/ui'
import { CopyButton, formatTime, kv } from '../components/format'
import type { Parameter, ParameterPut } from '../api/types'
import { api } from '../api/client'

const SERVICE = 'paramdora'

function ParamForm({ project, param, onDone }: { project?: string; param?: Parameter; onDone?: () => void }) {
  const { show } = useToast()
  const [name] = useState(param?.name ?? '')
  const [value, setValue] = useState(param?.value ?? '')
  const [type, setType] = useState(param?.data_type ?? 'secure_string')
  const [description, setDescription] = useState(param?.description ?? '')
  const [tier, setTier] = useState(param?.tier ?? 'standard')
  const [tagsText, setTagsText] = useState(param ? JSON.stringify(param.tags ?? {}) : '')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  async function submit(e: FormEvent) {
    e.preventDefault()
    let tags: Record<string, string> = {}
    if (tagsText.trim()) {
      try {
        tags = JSON.parse(tagsText)
      } catch {
        return setError('tags must be valid JSON, e.g. {"env":"prod"}')
      }
    }
    setBusy(true)
    setError('')
    const body: ParameterPut = { value, type, description, tier, tags }
    try {
      await api<Parameter>(SERVICE, `/parameter/${encodeURIComponent(project!)}/${encodeURIComponent(name)}`, {
        method: 'PUT',
        body: JSON.stringify(body),
      })
      show('success', `${param ? 'Updated' : 'Created'} parameter ${name}`)
      onDone?.()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <form onSubmit={submit}>
      <div className="form-grid">
        <Field label="Name" hint="may be hierarchical, e.g. /app/db/pass">
          <Input value={name} disabled={!!param} placeholder="app/db/pass" />
        </Field>
        <SelectField
          label="Type"
          value={type}
          onChange={setType}
          options={[
            { value: 'secure_string', label: 'secure_string' },
            { value: 'string', label: 'string' },
            { value: 'string_list', label: 'string_list' },
          ]}
        />
      </div>
      <Field label={type === 'secure_string' ? 'Value (encrypted at rest)' : 'Value'}>
        <TextArea value={value} onChange={(e) => setValue(e.target.value)} placeholder="value" />
      </Field>
      <div className="form-grid">
        <Field label="Description">
          <Input value={description} onChange={(e) => setDescription(e.target.value)} />
        </Field>
        <SelectField
          label="Tier"
          value={tier}
          onChange={setTier}
          options={[
            { value: 'standard', label: 'standard' },
            { value: 'advanced', label: 'advanced' },
          ]}
        />
      </div>
      <Field label="Tags (JSON)" hint='e.g. {"env":"prod"}'>
        <Input value={tagsText} onChange={(e) => setTagsText(e.target.value)} placeholder='{"env":"prod"}' />
      </Field>
      {error && <div className="form-errors">{error}</div>}
      <div className="row-end mt-2">
        <Button variant="primary" type="submit" disabled={busy}>
          {busy ? 'Saving…' : param ? 'Save' : 'Create'}
        </Button>
      </div>
    </form>
  )
}

function decodeTags(tags: Record<string, string>): string {
  try {
    return JSON.stringify(tags ?? {})
  } catch {
    return '{}'
  }
}

function ParamDetail({ project, param, onClose }: { project: string; param: Parameter; onClose: () => void }) {
  const { show } = useToast()
  const [decrypt, setDecrypt] = useState(false)
  const [secret, setSecret] = useState<string | null>(null)
  const [history, setHistory] = useState<Parameter[] | null>(null)
  const [showSecret, setShowSecret] = useState(false)
  const [busy, setBusy] = useState(false)

  async function toggleDecrypt() {
    if (decrypt && secret) {
      setDecrypt(false)
      setSecret(null)
      setShowSecret(false)
      return
    }
    setBusy(true)
    try {
      const p = await api<Parameter>(SERVICE, `/parameter/${encodeURIComponent(project)}/${encodeURIComponent(param.name)}`, {
        query: { decrypt: 'true' },
      })
      setSecret(p.value)
      setShowSecret(true)
      setDecrypt(true)
    } catch (err) {
      show('error', err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }

  async function loadHistory() {
    if (history) return
    setBusy(true)
    try {
      const r = await api<{ versions: Parameter[] }>(SERVICE, `/parameter/${encodeURIComponent(project)}/${encodeURIComponent(param.name)}`, {
        query: { history: 'true' },
      })
      setHistory(r.versions ?? [])
    } catch (err) {
      show('error', err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <Modal
      open
      wide
      onClose={onClose}
      title={<span>Parameter <span className="mono">{param.name}</span></span>}
      footer={<Button variant="ghost" onPress={onClose}>Close</Button>}
    >
      {kv([
        ['Status', <StateBadge state={param.status} />],
        ['Type', <Badge tone="info">{param.data_type}</Badge>],
        ['Version', String(param.version)],
        ['Tier', param.tier],
        ['Encrypted', param.is_encrypted ? <Badge tone="warn">yes</Badge> : 'no'],
        ['Key id', <span className="mono">{param.key_id || '—'}</span>],
        ['Description', param.description || '—'],
        ['Updated', formatTime(param.updated_at)],
        ['Tags', decodeTags(param.tags)],
      ])}
      {param.data_type === 'secure_string' && (
        <div className="row mt-3">
          <Button variant="ghost" onPress={toggleDecrypt} disabled={busy}>
            {decrypt || showSecret ? 'Hide value' : 'Reveal value'}
          </Button>
          {showSecret && secret !== null && (
            <span className="row">
              <span className="mono code-block" style={{ padding: '6px 10px' }}>{secret}</span>
              <CopyButton text={secret} />
              <Button variant="ghost" size="sm" onPress={() => setShowSecret(false)}>hide</Button>
            </span>
          )}
        </div>
      )}
      <div className="section-gap" />
      <Card
        title="Version history"
        actions={<Button variant="ghost" size="sm" onPress={loadHistory} disabled={busy}>Load history</Button>}
      >
        {history === null ? (
          <p className="muted">Every update creates a new immutable version.</p>
        ) : (
          <table className="data-table">
            <thead>
              <tr>
                <th>Version</th>
                <th>Type</th>
                <th>Value</th>
                <th>Tier</th>
                <th>Modified</th>
              </tr>
            </thead>
            <tbody>
              {history.map((v) => (
                <tr key={v.version}>
                  <td><span className="num">{v.version}</span></td>
                  <td>{v.data_type}</td>
                  <td><span className="mono">{v.data_type === 'secure_string' ? (v.is_encrypted ? '••••••' : v.value) : v.value}</span></td>
                  <td>{v.tier}</td>
                  <td className="muted">{formatTime(v.updated_at)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Card>
    </Modal>
  )
}

export default function ParamdoraPage() {
  const [project, setProject] = useState(() => sessionStorage.getItem(`olympus.project.${SERVICE}`) || '')
  const [prefix, setPrefix] = useState('')
  const [creating, setCreating] = useState(false)
  const [editing, setEditing] = useState<Parameter | null>(null)
  const [viewing, setViewing] = useState<Parameter | null>(null)

  const params = useAsync<Parameter[]>(() => {
    if (!project) return Promise.resolve([])
    return api<{ parameters: Parameter[] }>(SERVICE, `/parameters/${encodeURIComponent(project)}`, {
      query: { prefix },
    }).then((r) => r.parameters ?? [])
  })

  async function del(p: Parameter) {
    await api(SERVICE, `/parameter/${encodeURIComponent(project)}/${encodeURIComponent(p.name)}`, { method: 'DELETE' })
    params.refetch()
  }

  return (
    <div>
      <PageHeader icon="🔐" title="Paramdora" tagline="Parameter store with versioning and AES-256-GCM encryption.">
        <Button variant="primary" disabled={!project} onPress={() => setCreating(true)}>+ Parameter</Button>
      </PageHeader>
      <ProjectPicker service={SERVICE} onSelect={setProject} />

      <Card
        title={`Parameters${project ? ` · ${project}` : ''}`}
        actions={
          <Input
            className="w-[180px]"
            variant="secondary"
            value={prefix}
            onChange={(e) => setPrefix(e.target.value)}
            placeholder="Prefix filter…"
          />
        }
      >
        {params.loading ? (
          <p>Loading…</p>
        ) : !project ? (
          <p className="muted">Select a project to browse parameters.</p>
        ) : params.error ? (
          <div className="form-errors">{params.error}</div>
        ) : (params.data ?? []).length === 0 ? (
          <EmptyState icon="🔑" title="No parameters" hint="Create a parameter to store a value (secure_string is encrypted at rest)." />
        ) : (
          <table className="data-table">
            <thead>
              <tr>
                <th>Name</th>
                <th>Type</th>
                <th>Version</th>
                <th>Value</th>
                <th>Tier</th>
                <th>Updated</th>
                <th className="right">Actions</th>
              </tr>
            </thead>
            <tbody>
              {(params.data ?? []).map((p) => (
                <tr key={p.id} style={{ cursor: 'pointer' }} onClick={() => setViewing(p)}>
                  <td><span className="mono">{p.name}</span></td>
                  <td><Badge tone={p.is_encrypted ? 'warn' : 'info'}>{p.data_type}</Badge></td>
                  <td><span className="num">{p.version}</span></td>
                  <td>
                    <span className="mono">
                      {p.is_encrypted ? '••••••••' : p.value.length > 40 ? p.value.slice(0, 40) + '…' : p.value}
                    </span>
                  </td>
                  <td className="muted">{p.tier}</td>
                  <td className="muted">{formatTime(p.updated_at)}</td>
                  <td className="right" onClick={(e) => e.stopPropagation()}>
                    <div className="row">
                      <Button variant="ghost" size="sm" onPress={() => setViewing(p)}>View</Button>
                      <Button variant="ghost" size="sm" onPress={() => setEditing(p)}>Edit</Button>
                      <Button variant="danger" size="sm" onPress={() => del(p)}>Delete</Button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Card>

      <Modal open={creating} onClose={() => setCreating(false)} title="Create parameter" footer={<Button variant="ghost" onPress={() => setCreating(false)}>Cancel</Button>}>
        <ParamForm project={project} onDone={() => { setCreating(false); params.refetch() }} />
      </Modal>
      <Modal open={!!editing} onClose={() => setEditing(null)} title="Edit parameter" footer={<Button variant="ghost" onPress={() => setEditing(null)}>Cancel</Button>}>
        {editing && <ParamForm project={project} param={editing} onDone={() => { setEditing(null); params.refetch() }} />}
      </Modal>
      {viewing && <ParamDetail project={project} param={viewing} onClose={() => setViewing(null)} />}
    </div>
  )
}
