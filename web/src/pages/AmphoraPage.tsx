import { useEffect, useState, type FormEvent } from 'react'
import { Input } from '@heroui/react'
import { PageHeader } from '../components/PageHeader'
import { Card, Field, Button, useToast, EmptyState } from '../components/ui'
import { formatTime } from '../components/format'

// Amphora has no list/delete endpoints over HTTP (only PUT/GET by bucket/key),
// so the console keeps a per-bucket registry in localStorage of objects
// uploaded from this browser, making them re-downloadable here.

interface RegEntry {
  bucket: string
  key: string
  version: string
  etag: string
  contentType: string
  uploadedAt: string
}

const REG = 'olympus.amphora.objects'

function loadRegistry(bucket: string): RegEntry[] {
  try {
    const all = JSON.parse(localStorage.getItem(REG) || '[]') as RegEntry[]
    return all.filter((e) => e.bucket === bucket).sort((a, b) => (a.key < b.key ? -1 : 1))
  } catch {
    return []
  }
}

function saveRegistry(bucket: string, entries: RegEntry[]) {
  try {
    const all = JSON.parse(localStorage.getItem(REG) || '[]') as RegEntry[]
    localStorage.setItem(REG, JSON.stringify([...all.filter((e) => e.bucket !== bucket), ...entries]))
  } catch {
    /* ignore quota errors */
  }
}

function useRegistry(bucket: string, tick: number) {
  const [registry, setRegistry] = useState<RegEntry[]>(() => loadRegistry(bucket))
  useEffect(() => {
    setRegistry(loadRegistry(bucket))
  }, [bucket, tick])
  return registry
}

function UploadForm({ bucket, onUploaded }: { bucket: string; onUploaded: () => void }) {
  const { show } = useToast()
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  async function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    const form = e.currentTarget
    const fd = new FormData(form)
    const key = String(fd.get('key') || '').trim()
    const version = String(fd.get('version') || '').trim()
    const file = fd.get('file') as File | null
    if (!key) return setError('key is required')
    if (!file) return setError('choose a file to upload')
    setBusy(true)
    setError('')
    try {
      const headers: Record<string, string> = {
        'X-Object-Filename': key,
        'Content-Type': file.type || 'application/octet-stream',
      }
      if (version) headers['X-Object-Version-Id'] = version
      const res = await fetch(
        `/api/amphora/object/${encodeURIComponent(bucket)}/${encodeURIComponent(key)}`,
        { method: 'PUT', headers, body: file, credentials: 'same-origin' },
      )
      const text = await res.text()
      if (!res.ok) throw new Error(text || `upload failed (${res.status})`)

      const etag = res.headers.get('ETag') ?? ''
      const versionId = version || 'LATEST'
      const entry: RegEntry = {
        bucket,
        key,
        version: versionId,
        etag: etag || '—',
        contentType: file.type || 'application/octet-stream',
        uploadedAt: new Date().toISOString(),
      }
      saveRegistry(
        bucket,
        loadRegistry(bucket).filter((e) => !(e.key === key && e.version === versionId)).concat(entry),
      )
      show('success', `Uploaded ${key}`)
      onUploaded()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <form onSubmit={submit}>
      <Card title="Upload object">
        <div className="form-grid">
          <Field label="Key (filename)">
            <Input name="key" placeholder="reports/june.csv" autoComplete="off" />
          </Field>
          <Field label="Version id" hint="defaults to LATEST">
            <Input name="version" placeholder="e.g. v1" autoComplete="off" />
          </Field>
        </div>
        <Field label="File">
          <Input name="file" type="file" />
        </Field>
        {error && <div className="form-errors">{error}</div>}
        <Button variant="primary" type="submit" disabled={busy}>
          {busy ? 'Uploading…' : 'Upload'}
        </Button>
      </Card>
    </form>
  )
}

export default function AmphoraPage() {
  const [bucket, setBucket] = useState<string>(() => sessionStorage.getItem('olympus.bucket') || 'demo')
  const [tick, setTick] = useState(0)
  const registry = useRegistry(bucket, tick)
  const { show } = useToast()
  const [downloading, setDownloading] = useState<string | null>(null)

  useEffect(() => {
    if (bucket) sessionStorage.setItem('olympus.bucket', bucket)
  }, [bucket])

  async function download(entry: RegEntry) {
    setDownloading(entry.key)
    try {
      const res = await fetch(
        `/api/amphora/object/${encodeURIComponent(entry.bucket)}/${encodeURIComponent(entry.key)}`,
        { credentials: 'same-origin' },
      )
      if (!res.ok) throw new Error(`download failed (${res.status})`)
      const blob = await res.blob()
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = (entry.key.split('/').pop() || entry.key)
      document.body.appendChild(a)
      a.click()
      a.remove()
      setTimeout(() => URL.revokeObjectURL(url), 5000)
      show('success', `Downloading ${entry.key}`)
    } catch (err) {
      show('error', err instanceof Error ? err.message : String(err))
    } finally {
      setDownloading(null)
    }
  }

  return (
    <div>
      <PageHeader icon="🗄️" title="Amphora" tagline="Object storage — stream files up and down with a SHA-256 ETag.">
        <Field label="Bucket" className="mb-0 min-w-[200px]">
          <Input value={bucket} onChange={(e) => setBucket(e.target.value.trim() || 'demo')} placeholder="demo" />
        </Field>
      </PageHeader>

      <UploadForm bucket={bucket} onUploaded={() => setTick((n) => n + 1)} />

      <div className="section-gap" />

      <Card title={`Objects in ${bucket || 'demo'}`}>
        {registry.length === 0 ? (
          <EmptyState icon="🗂️" title="No objects uploaded from this console" hint="Uploads appear here automatically. Amphora stores the file under /object/{bucket}/{key}." />
        ) : (
          <table className="data-table">
            <thead>
              <tr>
                <th>Key</th>
                <th>Version</th>
                <th>Content type</th>
                <th>ETag</th>
                <th>Uploaded</th>
                <th className="right">Actions</th>
              </tr>
            </thead>
            <tbody>
              {registry.map((entry) => (
                <tr key={`${entry.key}-${entry.version}`}>
                  <td><span className="mono">{entry.key}</span></td>
                  <td>{entry.version}</td>
                  <td className="muted">{entry.contentType}</td>
                  <td><span className="mono">{entry.etag}</span></td>
                  <td className="muted">{formatTime(entry.uploadedAt)}</td>
                  <td className="right">
                    <Button variant="ghost" size="sm" disabled={downloading === entry.key} onPress={() => download(entry)}>
                      {downloading === entry.key ? '…' : 'Download'}
                    </Button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Card>

      <div className="section-gap" />
      <Card title="How it works">
        <p className="m-0 text-muted">
          Amphora exposes <span className="mono">PUT /object/{'{bucket}'}/{'{key}'}</span> (streams upload with
          SHA-256) and <span className="mono">GET /object/{'{bucket}'}/{'{key}'}</span> (streams download with range
          support); there is no list or delete endpoint. The ETag is the hex SHA-256 of the content. The object list
          above is a per-bucket registry kept by this browser; downloads always read the{' '}
          <span className="mono">LATEST</span> version.
        </p>
      </Card>
    </div>
  )
}
