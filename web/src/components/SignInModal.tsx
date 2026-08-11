import { useState, type FormEvent } from 'react'
import { Input } from '@heroui/react'
import { Modal, Field, Button, useToast } from './ui'
import { api, setAuth, setTenant, SERVICES, type StoredAuth } from '../api/client'
import type { TokenResponse } from '../api/types'

// SignInModal exchanges a Themis access key for a bearer JWT. The key belongs
// to a Themis user in a project; the resulting token is scoped to that user and
// project, and is attached to every service call so each backend can authorize
// the request via Themis.
export default function SignInModal({ open, onClose }: { open: boolean; onClose: () => void }) {
  const { show } = useToast()
  const [accessKeyId, setAccessKeyId] = useState('')
  const [secret, setSecret] = useState('')
  const [project, setProject] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  async function submit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError('')
    try {
      const tok = await api<TokenResponse>('themis', '/tokens', {
        method: 'POST',
        body: JSON.stringify({ access_key_id: accessKeyId, secret_access_key: secret }),
      })
      const auth: StoredAuth = {
        token: tok.token,
        subject: tok.claims.sub,
        principal_type: tok.claims.principal_type,
        account: tok.claims.account,
        project: tok.claims.project,
        expires_at: tok.expires_at,
      }
      setAuth(auth)
      setTenant(auth.account)
      // Align the console's active project with the token's project so service
      // requests hit the same namespace the token is scoped to.
      const projName = await resolveProjectName(auth.project)
      if (projName) {
        for (const s of SERVICES) sessionStorage.setItem(`olympus.project.${s}`, projName)
      }
      show('success', `Signed in as ${auth.subject}${projName ? ` (${projName})` : ''}`)
      setAccessKeyId('')
      setSecret('')
      setProject('')
      onClose()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }

  async function resolveProjectName(projectId: string): Promise<string | null> {
    try {
      const res = await api<{ projects: { id: string; name: string }[] }>('themis', '/projects')
      const found = (res.projects ?? []).find((p) => p.id === projectId)
      return found ? found.name : null
    } catch {
      return null
    }
  }

  return (
    <Modal
      open={open}
      onClose={onClose}
      title="Sign in with a Themis access key"
      footer={<Button variant="ghost" onPress={onClose}>Cancel</Button>}
    >
      <form onSubmit={submit}>
        <p className="form-errors" style={{ marginTop: 0 }}>
          Use an access key from Themis → users → a user → "Access keys". The key
          identifies who you are; its project becomes the scope for every service.
        </p>
        <Field label="Access key id"><Input className="mono" value={accessKeyId} onChange={(e) => setAccessKeyId(e.target.value)} required autoFocus /></Field>
        <Field label="Secret access key"><Input type="password" className="mono" value={secret} onChange={(e) => setSecret(e.target.value)} required /></Field>
        <Field label="Project name" hint="Must match the project the key's user belongs to.">
          <Input value={project} onChange={(e) => setProject(e.target.value)} required />
        </Field>
        {error && <div className="form-errors">{error}</div>}
        <div className="row-end mt-2">
          <Button variant="primary" type="submit" disabled={busy}>{busy ? 'Signing in…' : 'Sign in'}</Button>
        </div>
      </form>
    </Modal>
  )
}
