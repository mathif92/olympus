import { useState, type FormEvent } from 'react'
import { Input, TextArea } from '@heroui/react'
import { PageHeader, ProjectPicker } from '../components/PageHeader'
import { Card, Field, Button, Modal, SelectField, useToast, StateBadge, useAsync, EmptyState, Badge, SegmentedTabs } from '../components/ui'
import { CopyButton, formatTime, kv } from '../components/format'
import type {
  ThemisUser, ThemisGroup, ThemisRole, ThemisPolicy, GroupMembership, PolicyAttachment,
  ThemisAccessKey, EvaluationDecision, TokenResponse,
} from '../api/types'
import { api } from '../api/client'

const SERVICE = 'themis'

const POLICY_TEMPLATE = `{
  "Version": "2012-10-17",
  "Statement": [
    { "Sid": "FullAccess", "Effect": "Allow", "Action": ["*"], "Resource": ["*"] }
  ]
}`

function CreateUserForm({ project, onDone }: { project: string; onDone: () => void }) {
  const { show } = useToast()
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [path, setPath] = useState('/')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  async function submit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError('')
    try {
      await api<ThemisUser>(SERVICE, '/users', {
        method: 'POST',
        body: JSON.stringify({ project, name, description, path }),
      })
      show('success', `Created user ${name}`)
      onDone()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <form onSubmit={submit}>
      <Field label="User name"><Input value={name} onChange={(e) => setName(e.target.value)} required autoFocus /></Field>
      <div className="form-grid">
        <Field label="Path" hint="e.g. /ci/ or /service-accounts/"><Input value={path} onChange={(e) => setPath(e.target.value)} /></Field>
        <Field label="Description"><Input value={description} onChange={(e) => setDescription(e.target.value)} /></Field>
      </div>
      {error && <div className="form-errors">{error}</div>}
      <div className="row-end mt-2">
        <Button variant="primary" type="submit" disabled={busy}>{busy ? 'Creating…' : 'Create user'}</Button>
      </div>
    </form>
  )
}

function CreateGroupForm({ project, onDone }: { project: string; onDone: () => void }) {
  const { show } = useToast()
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  async function submit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError('')
    try {
      await api<ThemisGroup>(SERVICE, '/groups', {
        method: 'POST',
        body: JSON.stringify({ project, name, description }),
      })
      show('success', `Created group ${name}`)
      onDone()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <form onSubmit={submit}>
      <Field label="Group name"><Input value={name} onChange={(e) => setName(e.target.value)} required autoFocus /></Field>
      <Field label="Description"><Input value={description} onChange={(e) => setDescription(e.target.value)} /></Field>
      {error && <div className="form-errors">{error}</div>}
      <div className="row-end mt-2">
        <Button variant="primary" type="submit" disabled={busy}>{busy ? 'Creating…' : 'Create group'}</Button>
      </div>
    </form>
  )
}

function CreateRoleForm({ project, onDone }: { project: string; onDone: () => void }) {
  const { show } = useToast()
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  async function submit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError('')
    try {
      await api<ThemisRole>(SERVICE, '/roles', {
        method: 'POST',
        body: JSON.stringify({ project, name, description }),
      })
      show('success', `Created role ${name}`)
      onDone()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <form onSubmit={submit}>
      <Field label="Role name"><Input value={name} onChange={(e) => setName(e.target.value)} required autoFocus /></Field>
      <Field label="Description"><Input value={description} onChange={(e) => setDescription(e.target.value)} /></Field>
      {error && <div className="form-errors">{error}</div>}
      <div className="row-end mt-2">
        <Button variant="primary" type="submit" disabled={busy}>{busy ? 'Creating…' : 'Create role'}</Button>
      </div>
    </form>
  )
}

function CreatePolicyForm({ project, onDone }: { project: string; onDone: () => void }) {
  const { show } = useToast()
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [document, setDocument] = useState(POLICY_TEMPLATE)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  async function submit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError('')
    try {
      await api<ThemisPolicy>(SERVICE, '/policies', {
        method: 'POST',
        body: JSON.stringify({ project, name, description, document }),
      })
      show('success', `Created policy ${name}`)
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
        <Field label="Policy name"><Input value={name} onChange={(e) => setName(e.target.value)} required autoFocus /></Field>
        <Field label="Description"><Input value={description} onChange={(e) => setDescription(e.target.value)} /></Field>
      </div>
      <Field label="Document (JSON)" hint="Effect must be Allow or Deny; wildcards like iam:* supported">
        <TextArea value={document} onChange={(e) => setDocument(e.target.value)} className="font-mono" />
      </Field>
      {error && <div className="form-errors">{error}</div>}
      <div className="row-end mt-2">
        <Button variant="primary" type="submit" disabled={busy}>{busy ? 'Creating…' : 'Create policy'}</Button>
      </div>
    </form>
  )
}

function SecretReveal({ accessKeyId, secret, onDone }: { accessKeyId: string; secret: string; onDone: () => void }) {
  return (
    <Modal open onClose={onDone} title="Access key created" footer={<Button variant="primary" onPress={onDone}>Done</Button>}>
      <p className="form-errors" style={{ marginTop: 0 }}>
        This secret is shown only once — copy it now. Themis stores just the SHA-256 hash of it.
      </p>
      <Field label="Access key id">
        <div className="row">
          <Input readOnly className="mono" value={accessKeyId} style={{ flex: 1 }} />
          <CopyButton text={accessKeyId} label="Copy" />
        </div>
      </Field>
      <Field label="Secret access key">
        <div className="row">
          <Input readOnly className="mono" value={secret} style={{ flex: 1 }} />
          <CopyButton text={secret} label="Copy" />
        </div>
      </Field>
    </Modal>
  )
}

function TokenMintModal({ user, onClose }: { user: ThemisUser; onClose: () => void }) {
  const { show } = useToast()
  const [accessKeyId, setAccessKeyId] = useState('')
  const [secret, setSecret] = useState('')
  const [result, setResult] = useState<TokenResponse | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  async function submit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError('')
    try {
      const r = await api<TokenResponse>(SERVICE, '/tokens', {
        method: 'POST',
        body: JSON.stringify({ access_key_id: accessKeyId, secret_access_key: secret }),
      })
      setResult(r)
      show('success', 'Minted token')
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <Modal open onClose={onClose} title={`Mint token · ${user.name}`} footer={<Button variant="ghost" onPress={onClose}>Close</Button>}>
      {result ? (
        <>
          <Field label="JWT (HS256, valid 1h)">
            <div className="code-block">{result.token}</div>
          </Field>
          <div className="row">
            <CopyButton text={result.token} label="Copy token" />
          </div>
          <div className="section-gap" />
          {kv([['Subject', result.claims.sub], ['Principal', result.claims.principal_type], ['Project', result.claims.project], ['Expires', formatTime(result.expires_at)]])}
        </>
      ) : (
        <form onSubmit={submit}>
          <Field label="Access key id"><Input value={accessKeyId} onChange={(e) => setAccessKeyId(e.target.value)} className="mono" required autoFocus /></Field>
          <Field label="Secret access key"><Input value={secret} onChange={(e) => setSecret(e.target.value)} className="mono" required /></Field>
          {error && <div className="form-errors">{error}</div>}
          <div className="row-end mt-2">
            <Button variant="primary" type="submit" disabled={busy}>{busy ? 'Minting…' : 'Mint token'}</Button>
          </div>
        </form>
      )}
    </Modal>
  )
}

function KeysModal({ project, user, onClose }: { project: string; user: ThemisUser; onClose: () => void }) {
  const { show } = useToast()
  const [keys, setKeys] = useState<ThemisAccessKey[] | null>(null)
  const [reveal, setReveal] = useState<ThemisAccessKey | null>(null)
  const [minting, setMinting] = useState(false)
  const [busy, setBusy] = useState(false)

  async function load() {
    const r = await api<{ access_keys: ThemisAccessKey[] }>(SERVICE, `/user/${encodeURIComponent(user.name)}/keys`, { query: { project } })
    setKeys(r.access_keys ?? [])
  }

  async function create() {
    setBusy(true)
    try {
      const k = await api<ThemisAccessKey>(SERVICE, `/user/${encodeURIComponent(user.name)}/keys`, { method: 'POST', body: JSON.stringify({ project }) })
      setReveal(k)
      await load()
    } catch (err) {
      show('error', err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }

  async function setStatus(k: ThemisAccessKey, status: string) {
    try {
      await api<ThemisAccessKey>(SERVICE, `/user/${encodeURIComponent(user.name)}/keys/${k.id}/status`, {
        method: 'PATCH', body: JSON.stringify({ status }),
      })
      show('success', `${k.id} ${status}`)
      load()
    } catch (err) {
      show('error', err instanceof Error ? err.message : String(err))
    }
  }

  async function del(k: ThemisAccessKey) {
    try {
      await api(SERVICE, `/user/${encodeURIComponent(user.name)}/keys/${k.id}`, { method: 'DELETE' })
      show('success', `Deleted key ${k.id}`)
      load()
    } catch (err) {
      show('error', err instanceof Error ? err.message : String(err))
    }
  }

  return (
    <Modal open wide onClose={onClose} title={`Access keys · ${user.name}`} footer={<Button variant="ghost" onPress={onClose}>Close</Button>}>
      <div className="row mb-3">
        <Button variant="primary" size="sm" onPress={create} disabled={busy}>+ Create access key</Button>
        <Button variant="ghost" size="sm" onPress={load}>Refresh</Button>
        <Button variant="ghost" size="sm" onPress={() => setMinting(true)}>Mint token…</Button>
      </div>
      {keys === null ? (
        <p className="muted">Loading…</p>
      ) : keys.length === 0 ? (
        <EmptyState icon="🔑" title="No access keys" hint="Create a key to get an AKIA-style credential pair for this user." />
      ) : (
        <table className="data-table">
          <thead><tr><th>Access key id</th><th>Status</th><th>Last used</th><th>Created</th><th className="right">Actions</th></tr></thead>
          <tbody>
            {keys.map((k) => (
              <tr key={k.id}>
                <td><span className="mono">{k.id}</span></td>
                <td><StateBadge state={k.status} /></td>
                <td className="muted">{formatTime(k.last_used_at ?? undefined)}</td>
                <td className="muted">{formatTime(k.created_at)}</td>
                <td className="right">
                  <div className="row">
                    {k.status === 'active'
                      ? <Button variant="ghost" size="sm" onPress={() => setStatus(k, 'inactive')}>Deactivate</Button>
                      : <Button variant="ghost" size="sm" onPress={() => setStatus(k, 'active')}>Activate</Button>}
                    <Button variant="danger" size="sm" onPress={() => del(k)}>Delete</Button>
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      {reveal && <SecretReveal accessKeyId={reveal.id} secret={reveal.secret_access_key ?? ''} onDone={() => setReveal(null)} />}
      {minting && <TokenMintModal user={user} onClose={() => setMinting(false)} />}
    </Modal>
  )
}

function MembersModal({ project, group, onClose }: { project: string; group: ThemisGroup; onClose: () => void }) {
  const { show } = useToast()
  const [members, setMembers] = useState<GroupMembership[] | null>(null)
  const [addUser, setAddUser] = useState('')
  const [users, setUsers] = useState<ThemisUser[]>([])

  async function load() {
    const [r, u] = await Promise.all([
      api<{ members: GroupMembership[] }>(SERVICE, `/group/${encodeURIComponent(group.name)}/members`, { query: { project } }),
      api<{ users: ThemisUser[] }>(SERVICE, '/users', { query: { project } }),
    ])
    setMembers(r.members ?? [])
    setUsers(u.users ?? [])
  }

  async function add() {
    try {
      await api(SERVICE, `/group/${encodeURIComponent(group.name)}/members`, {
        method: 'POST', body: JSON.stringify({ user: addUser }),
      })
      show('success', `Added ${addUser} to ${group.name}`)
      setAddUser('')
      load()
    } catch (err) {
      show('error', err instanceof Error ? err.message : String(err))
    }
  }

  async function remove(name: string) {
    try {
      await api(SERVICE, `/group/${encodeURIComponent(group.name)}/members`, {
        method: 'DELETE', query: { user: name },
      })
      show('success', `Removed ${name}`)
      load()
    } catch (err) {
      show('error', err instanceof Error ? err.message : String(err))
    }
  }

  const inGroup = new Set((members ?? []).map((m) => m.user_name))
  const available = users.filter((u) => !inGroup.has(u.name))

  return (
    <Modal open wide onClose={onClose} title={`Members · ${group.name}`} footer={<Button variant="ghost" onPress={onClose}>Close</Button>}>
      <div className="row mb-3" style={{ alignItems: 'flex-end' }}>
        <div style={{ minWidth: 220, flex: 1 }}>
          <SelectField
            label="Add user"
            value={addUser}
            onChange={setAddUser}
            placeholder="— select user —"
            options={available.map((u) => ({ value: u.name, label: u.name }))}
          />
        </div>
        <Button variant="primary" size="sm" onPress={add} disabled={!addUser}>Add</Button>
      </div>
      {members === null ? (
        <p className="muted">Loading…</p>
      ) : members.length === 0 ? (
        <EmptyState icon="👥" title="No members" hint="Users in this group inherit its policies." />
      ) : (
        <table className="data-table">
          <thead><tr><th>User</th><th>Added</th><th className="right">Actions</th></tr></thead>
          <tbody>
            {members.map((m) => (
              <tr key={m.user_id}>
                <td><span className="mono">{m.user_name}</span></td>
                <td className="muted">{formatTime(m.created_at)}</td>
                <td className="right"><Button variant="danger" size="sm" onPress={() => remove(m.user_name)}>Remove</Button></td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </Modal>
  )
}

function AttachPolicyModal({
  project, defaultType, defaultName, onDone,
}: { project: string; defaultType: 'user' | 'group' | 'role'; defaultName: string; onDone: () => void }) {
  const { show } = useToast()
  const [principalType, setPrincipalType] = useState(defaultType)
  const [principalName, setPrincipalName] = useState(defaultName)
  const [policyName, setPolicyName] = useState('')
  const [names, setNames] = useState<string[]>([])
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  const policies = useAsync<ThemisPolicy[]>(() => api<{ policies: ThemisPolicy[] }>(SERVICE, '/policies', { query: { project } }).then((r) => r.policies ?? []))

  async function loadPrincipals() {
    const key = principalType === 'user' ? 'users' : principalType === 'group' ? 'groups' : 'roles'
    const r = await api<Record<string, { name: string }[]>>(SERVICE, `/${key}`, { query: { project } })
    setNames((r[key] ?? []).map((x) => x.name))
  }

  async function submit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError('')
    try {
      await api<PolicyAttachment>(SERVICE, '/attachments', {
        method: 'POST',
        body: JSON.stringify({ project, principal_type: principalType, principal_name: principalName, policy_name: policyName }),
      })
      show('success', `Attached ${policyName} to ${principalType} ${principalName}`)
      onDone()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <Modal open onClose={onDone} title="Attach policy" footer={<Button variant="ghost" onPress={onDone}>Cancel</Button>}>
      <form onSubmit={submit}>
        <div className="form-grid">
          <SelectField
            label="Principal type"
            value={principalType}
            onChange={(v) => { setPrincipalType(v as 'user' | 'group' | 'role'); setPrincipalName(''); loadPrincipals() }}
            options={[
              { value: 'user', label: 'user' },
              { value: 'group', label: 'group' },
              { value: 'role', label: 'role' },
            ]}
          />
          <Field label="Principal">
            <Input value={principalName} onChange={(e) => setPrincipalName(e.target.value)} required placeholder="name" onFocus={loadPrincipals} list={`themis-${principalType}s`} />
            <datalist id={`themis-${principalType}s`}>
              {names.map((n) => <option key={n} value={n} />)}
            </datalist>
          </Field>
        </div>
        <SelectField
          label="Policy"
          value={policyName}
          onChange={setPolicyName}
          isRequired
          placeholder="— select policy —"
          options={(policies.data ?? []).map((p) => ({ value: p.name, label: p.name }))}
        />
        {error && <div className="form-errors">{error}</div>}
        <div className="row-end mt-2">
          <Button variant="primary" type="submit" disabled={busy}>{busy ? 'Attaching…' : 'Attach'}</Button>
        </div>
      </form>
    </Modal>
  )
}

function ViewPolicyModal({ policy, onClose }: { policy: ThemisPolicy; onClose: () => void }) {
  return (
    <Modal open wide onClose={onClose} title={<span>Policy <span className="mono">{policy.name}</span></span>} footer={<Button variant="ghost" onPress={onClose}>Close</Button>}>
      {kv([
        ['Version', String(policy.version)],
        ['Status', <StateBadge state={policy.status} />],
        ['Description', policy.description || '—'],
      ])}
      <div className="section-gap" />
      <Field label="Document">
        <div className="code-block">{JSON.stringify(policy.document, null, 2)}</div>
      </Field>
    </Modal>
  )
}

function EvaluateCard({ project }: { project: string }) {
  const [principalType, setPrincipalType] = useState<'user' | 'group' | 'role'>('user')
  const [principalName, setPrincipalName] = useState('')
  const [action, setAction] = useState('s3:GetObject')
  const [resource, setResource] = useState('arn:aws:s3:::assets/*')
  const [decision, setDecision] = useState<EvaluationDecision | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  async function run(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError('')
    setDecision(null)
    try {
      const d = await api<EvaluationDecision>(SERVICE, '/authorize', {
        method: 'POST',
        body: JSON.stringify({ project, principal_type: principalType, principal_name: principalName, action, resource }),
      })
      setDecision(d)
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <Card title="Evaluate an authorization request">
      <form onSubmit={run}>
        <div className="form-grid">
          <SelectField
            label="Principal type"
            value={principalType}
            onChange={(v) => setPrincipalType(v as 'user' | 'group' | 'role')}
            options={[
              { value: 'user', label: 'user' },
              { value: 'group', label: 'group' },
              { value: 'role', label: 'role' },
            ]}
          />
          <Field label="Principal name"><Input value={principalName} onChange={(e) => setPrincipalName(e.target.value)} required placeholder="deploy" /></Field>
        </div>
        <div className="form-grid">
          <Field label="Action"><Input value={action} onChange={(e) => setAction(e.target.value)} className="mono" required /></Field>
          <Field label="Resource"><Input value={resource} onChange={(e) => setResource(e.target.value)} className="mono" required /></Field>
        </div>
        {error && <div className="form-errors">{error}</div>}
        <div className="row-end mt-2">
          <Button variant="primary" type="submit" disabled={busy || !project}>{busy ? 'Evaluating…' : 'Evaluate'}</Button>
        </div>
      </form>
      {decision && (
        <div className="mt-4">
          <div className="row">
            <Badge tone={decision.allowed ? 'ok' : 'danger'}>{decision.allowed ? 'ALLOW' : 'DENY'}</Badge>
            <span className="muted">{decision.principal} · {decision.action} on <span className="mono">{decision.resource}</span></span>
          </div>
          {decision.matched_statements.length > 0 && (
            <p className="muted mt-2">Matched statements: {decision.matched_statements.join(', ')}</p>
          )}
          {!decision.allowed && decision.matched_statements.length === 0 && (
            <p className="muted mt-2">No statement matched — implicit deny.</p>
          )}
        </div>
      )}
    </Card>
  )
}

export default function ThemisPage() {
  const [project, setProject] = useState(() => sessionStorage.getItem(`olympus.project.${SERVICE}`) || '')
  const [tab, setTab] = useState<'users' | 'groups' | 'roles' | 'policies' | 'evaluate'>('users')
  const [creating, setCreating] = useState(false)
  const [attaching, setAttaching] = useState<{ type: 'user' | 'group' | 'role'; name: string } | null>(null)
  const [keysUser, setKeysUser] = useState<ThemisUser | null>(null)
  const [membersGroup, setMembersGroup] = useState<ThemisGroup | null>(null)
  const [viewing, setViewing] = useState<ThemisPolicy | null>(null)
  const { show } = useToast()

  const users = useAsync<ThemisUser[]>(() => {
    if (!project) return Promise.resolve([])
    return api<{ users: ThemisUser[] }>(SERVICE, '/users', { query: { project } }).then((r) => r.users ?? [])
  })
  const groups = useAsync<ThemisGroup[]>(() => {
    if (!project) return Promise.resolve([])
    return api<{ groups: ThemisGroup[] }>(SERVICE, '/groups', { query: { project } }).then((r) => r.groups ?? [])
  })
  const roles = useAsync<ThemisRole[]>(() => {
    if (!project) return Promise.resolve([])
    return api<{ roles: ThemisRole[] }>(SERVICE, '/roles', { query: { project } }).then((r) => r.roles ?? [])
  })
  const policies = useAsync<ThemisPolicy[]>(() => {
    if (!project) return Promise.resolve([])
    return api<{ policies: ThemisPolicy[] }>(SERVICE, '/policies', { query: { project } }).then((r) => r.policies ?? [])
  })

  async function delUser(u: ThemisUser) {
    try {
      await api(SERVICE, `/user/${encodeURIComponent(u.name)}`, { method: 'DELETE', query: { project } })
      show('success', `Deleted user ${u.name}`)
      users.refetch()
    } catch (err) {
      show('error', err instanceof Error ? err.message : String(err))
    }
  }
  async function delGroup(g: ThemisGroup) {
    try {
      await api(SERVICE, `/group/${encodeURIComponent(g.name)}`, { method: 'DELETE', query: { project } })
      show('success', `Deleted group ${g.name}`)
      groups.refetch()
    } catch (err) {
      show('error', err instanceof Error ? err.message : String(err))
    }
  }
  async function delRole(r: ThemisRole) {
    try {
      await api(SERVICE, `/role/${encodeURIComponent(r.name)}`, { method: 'DELETE', query: { project } })
      show('success', `Deleted role ${r.name}`)
      roles.refetch()
    } catch (err) {
      show('error', err instanceof Error ? err.message : String(err))
    }
  }
  async function delPolicy(p: ThemisPolicy) {
    try {
      await api(SERVICE, `/policy/${encodeURIComponent(p.name)}`, { method: 'DELETE', query: { project } })
      show('success', `Deleted policy ${p.name}`)
      policies.refetch()
    } catch (err) {
      show('error', err instanceof Error ? err.message : String(err))
    }
  }

  const createLabel: Record<string, string> = {
    users: '+ User', groups: '+ Group', roles: '+ Role', policies: '+ Policy',
  }

  const tabDefs = ['users', 'groups', 'roles', 'policies', 'evaluate'] as const

  return (
    <div>
      <PageHeader icon="⚖️" title="Themis" tagline="Identity & access — users, groups, roles, policies, keys, JWTs.">
        <Button variant="ghost" onPress={() => { users.refetch(); groups.refetch(); roles.refetch(); policies.refetch() }}>Refresh</Button>
        {tab !== 'evaluate' && <Button variant="primary" disabled={!project} onPress={() => setCreating(true)}>{createLabel[tab]}</Button>}
      </PageHeader>
      <ProjectPicker service={SERVICE} onSelect={setProject} />

      <div className="mb-4">
        <SegmentedTabs tabs={tabDefs} selected={tab} onSelect={(t) => setTab(t as typeof tab)} ariaLabel="Themis resources" />
      </div>

      {!project && <EmptyState icon="⚖️" title="Select a project" hint="Choose a project to manage its identities and policies." />}

      {tab === 'users' && project && (
        <Card title={`Users · ${project}`}>
          {users.loading ? <p>Loading…</p> : (users.data ?? []).length === 0 ? <EmptyState icon="👤" title="No users" hint="Create an IAM user to issue access keys." /> : (
            <table className="data-table">
              <thead><tr><th>Name</th><th>Path</th><th>Status</th><th>Created</th><th className="right">Actions</th></tr></thead>
              <tbody>
                {users.data?.map((u) => (
                  <tr key={u.id}>
                    <td><span className="mono">{u.name}</span></td>
                    <td className="muted">{u.path || '/'}</td>
                    <td><StateBadge state={u.status} /></td>
                    <td className="muted">{formatTime(u.created_at)}</td>
                    <td className="right">
                      <div className="row">
                        <Button variant="ghost" size="sm" onPress={() => setKeysUser(u)}>Keys</Button>
                        <Button variant="ghost" size="sm" onPress={() => setAttaching({ type: 'user', name: u.name })}>Attach policy</Button>
                        <Button variant="danger" size="sm" onPress={() => delUser(u)}>Delete</Button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </Card>
      )}

      {tab === 'groups' && project && (
        <Card title={`Groups · ${project}`}>
          {groups.loading ? <p>Loading…</p> : (groups.data ?? []).length === 0 ? <EmptyState icon="👥" title="No groups" hint="Create a group and add users; policies attached to it apply to all members." /> : (
            <table className="data-table">
              <thead><tr><th>Name</th><th>Description</th><th>Status</th><th className="right">Actions</th></tr></thead>
              <tbody>
                {groups.data?.map((g) => (
                  <tr key={g.id}>
                    <td><span className="mono">{g.name}</span></td>
                    <td className="muted">{g.description || '—'}</td>
                    <td><StateBadge state={g.status} /></td>
                    <td className="right">
                      <div className="row">
                        <Button variant="ghost" size="sm" onPress={() => setMembersGroup(g)}>Members</Button>
                        <Button variant="ghost" size="sm" onPress={() => setAttaching({ type: 'group', name: g.name })}>Attach policy</Button>
                        <Button variant="danger" size="sm" onPress={() => delGroup(g)}>Delete</Button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </Card>
      )}

      {tab === 'roles' && project && (
        <Card title={`Roles · ${project}`}>
          {roles.loading ? <p>Loading…</p> : (roles.data ?? []).length === 0 ? <EmptyState icon="🎭" title="No roles" hint="Create a role to group policies for assumed identities." /> : (
            <table className="data-table">
              <thead><tr><th>Name</th><th>Description</th><th>Status</th><th className="right">Actions</th></tr></thead>
              <tbody>
                {roles.data?.map((r) => (
                  <tr key={r.id}>
                    <td><span className="mono">{r.name}</span></td>
                    <td className="muted">{r.description || '—'}</td>
                    <td><StateBadge state={r.status} /></td>
                    <td className="right">
                      <div className="row">
                        <Button variant="ghost" size="sm" onPress={() => setAttaching({ type: 'role', name: r.name })}>Attach policy</Button>
                        <Button variant="danger" size="sm" onPress={() => delRole(r)}>Delete</Button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </Card>
      )}

      {tab === 'policies' && project && (
        <Card title={`Policies · ${project}`}>
          {policies.loading ? <p>Loading…</p> : (policies.data ?? []).length === 0 ? <EmptyState icon="📜" title="No policies" hint="Create an IAM policy document and attach it to users, groups or roles." /> : (
            <table className="data-table">
              <thead><tr><th>Name</th><th>Version</th><th>Status</th><th>Description</th><th className="right">Actions</th></tr></thead>
              <tbody>
                {policies.data?.map((p) => (
                  <tr key={p.id}>
                    <td><span className="mono">{p.name}</span></td>
                    <td><span className="num">{p.version}</span></td>
                    <td><StateBadge state={p.status} /></td>
                    <td className="muted">{p.description || '—'}</td>
                    <td className="right">
                      <div className="row">
                        <Button variant="ghost" size="sm" onPress={() => setViewing(p)}>View</Button>
                        <Button variant="danger" size="sm" onPress={() => delPolicy(p)}>Delete</Button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </Card>
      )}

      {tab === 'evaluate' && project && <EvaluateCard project={project} />}

      <Modal open={creating} onClose={() => setCreating(false)}
        title={tab === 'users' ? 'Create user' : tab === 'groups' ? 'Create group' : tab === 'roles' ? 'Create role' : 'Create policy'}
        footer={<Button variant="ghost" onPress={() => setCreating(false)}>Cancel</Button>}>
        {tab === 'users' && <CreateUserForm project={project} onDone={() => { setCreating(false); users.refetch() }} />}
        {tab === 'groups' && <CreateGroupForm project={project} onDone={() => { setCreating(false); groups.refetch() }} />}
        {tab === 'roles' && <CreateRoleForm project={project} onDone={() => { setCreating(false); roles.refetch() }} />}
        {tab === 'policies' && <CreatePolicyForm project={project} onDone={() => { setCreating(false); policies.refetch() }} />}
      </Modal>

      {attaching && <AttachPolicyModal project={project} defaultType={attaching.type} defaultName={attaching.name} onDone={() => setAttaching(null)} />}
      {keysUser && <KeysModal project={project} user={keysUser} onClose={() => setKeysUser(null)} />}
      {membersGroup && <MembersModal project={project} group={membersGroup} onClose={() => setMembersGroup(null)} />}
      {viewing && <ViewPolicyModal policy={viewing} onClose={() => setViewing(null)} />}
    </div>
  )
}