import { useState, type FormEvent } from 'react'
import { PageHeader, ProjectPicker } from '../components/PageHeader'
import { Card, Field, Button, Modal, useToast, StateBadge, useAsync, EmptyState, Badge } from '../components/ui'
import { formatTime, CopyButton } from '../components/format'
import type { Queue, QueueMessage, Topic, Subscriber, PublishResult } from '../api/types'
import { api } from '../api/client'

const SERVICE = 'iris'

function CreateQueue({ project, onDone }: { project: string; onDone: () => void }) {
  const { show } = useToast()
  const [name, setName] = useState('')
  const [visibility, setVisibility] = useState('30')
  const [retention, setRetention] = useState('86400')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  async function submit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError('')
    try {
      await api<Queue>(SERVICE, '/queues', {
        method: 'POST',
        body: JSON.stringify({ project, name, visibility_timeout_sec: Number(visibility), message_retention_sec: Number(retention) }),
      })
      show('success', `Created queue ${name}`)
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
        <Field label="Queue name"><input value={name} onChange={(e) => setName(e.target.value)} required autoFocus /></Field>
        <Field label="Visibility timeout (sec)">
          <input value={visibility} onChange={(e) => setVisibility(e.target.value)} type="number" min={0} />
        </Field>
      </div>
      <Field label="Message retention (sec)"><input value={retention} onChange={(e) => setRetention(e.target.value)} type="number" min={0} /></Field>
      {error && <div className="form-errors">{error}</div>}
      <div className="row-end">
        <Button variant="primary" type="submit" disabled={busy}>{busy ? 'Creating…' : 'Create'}</Button>
      </div>
    </form>
  )
}

function CreateTopic({ project, onDone }: { project: string; onDone: () => void }) {
  const { show } = useToast()
  const [name, setName] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  async function submit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError('')
    try {
      await api<Topic>(SERVICE, '/topics', {
        method: 'POST',
        body: JSON.stringify({ project, name }),
      })
      show('success', `Created topic ${name}`)
      onDone()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <form onSubmit={submit}>
      <Field label="Topic name"><input value={name} onChange={(e) => setName(e.target.value)} required autoFocus /></Field>
      {error && <div className="form-errors">{error}</div>}
      <div className="row-end">
        <Button variant="primary" type="submit" disabled={busy}>{busy ? 'Creating…' : 'Create'}</Button>
      </div>
    </form>
  )
}

function QueuePanel({ project, queue }: { project: string; queue: Queue }) {
  const { show } = useToast()
  const [messages, setMessages] = useState<QueueMessage[] | null>(null)
  const [sendBody, setSendBody] = useState('')
  const [attrs, setAttrs] = useState('')
  const [poller, setPoller] = useState<ReturnType<typeof setInterval> | null>(null)
  const [busy, setBusy] = useState(false)

  async function send(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    let attributes: Record<string, string> = {}
    if (attrs.trim()) {
      try {
        attributes = JSON.parse(attrs)
      } catch {
        show('error', 'attributes must be valid JSON')
      }
    }
    try {
      await api<QueueMessage>(SERVICE, `/queue/${encodeURIComponent(project)}/${encodeURIComponent(queue.name)}/send`, {
        method: 'POST',
        body: JSON.stringify({ body: sendBody, attributes }),
      })
      show('success', `Sent message to ${queue.name}`)
      setSendBody('')
      setAttrs('')
    } catch (err) {
      show('error', err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }

  async function poll() {
    try {
      const r = await api<{ messages: QueueMessage[] }>(SERVICE, `/queue/${encodeURIComponent(project)}/${encodeURIComponent(queue.name)}/poll`, { method: 'POST' })
      setMessages(r.messages ?? [])
      if (r.messages?.length) show('success', `Polled ${r.messages.length} message(s)`)
    } catch (err) {
      show('error', err instanceof Error ? err.message : String(err))
    }
  }

  async function ack(m: QueueMessage) {
    try {
      await api(SERVICE, `/queue/${encodeURIComponent(project)}/${encodeURIComponent(queue.name)}/ack`, {
        method: 'POST',
        body: JSON.stringify({ message_id: m.id }),
      })
      show('success', `Acked ${m.id.slice(0, 8)}`)
      poll()
    } catch (err) {
      show('error', err instanceof Error ? err.message : String(err))
    }
  }

  function watch() {
    if (poller) return
    setPoller(setInterval(poll, 5000))
  }
  function stopWatch() {
    if (poller) {
      clearInterval(poller)
      setPoller(null)
    }
  }

  return (
    <div>
      <Card title={`Send to ${queue.name}`}>
        <form onSubmit={send}>
          <Field label="Body">
            <textarea value={sendBody} onChange={(e) => setSendBody(e.target.value)} placeholder="Message body" required />
          </Field>
          <Field label="Attributes (JSON)" hint='e.g. {"type":"ci"}'>
            <input value={attrs} onChange={(e) => setAttrs(e.target.value)} placeholder='{}' />
          </Field>
          <div className="row-end">
            <Button variant="primary" type="submit" disabled={busy}>{busy ? 'Sending…' : 'Send'}</Button>
          </div>
        </form>
      </Card>
      <div className="section-gap" />
      <Card
        title="Messages"
        actions={
          <div className="row">
            {poller ? <Button variant="ghost" onClick={stopWatch}>Stop watch</Button> : <Button variant="ghost" onClick={watch}>Watch (5s)</Button>}
            <Button variant="ghost" onClick={poll}>Poll now</Button>
          </div>
        }
      >
        {messages === null ? (
          <EmptyState icon="📨" title="Not polled yet" hint="Poll the queue to pull up to 10 visible messages (they become in_flight for the visibility timeout)." />
        ) : messages.length === 0 ? (
          <EmptyState icon="🌙" title="No visible messages" hint="Nothing pending and visible right now." />
        ) : (
          <table className="data">
            <thead><tr><th>Id</th><th>State</th><th>Attributes</th><th>Body</th><th>Visible at</th><th className="right">Actions</th></tr></thead>
            <tbody>
              {messages.map((m) => (
                <tr key={m.id}>
                  <td><span className="mono">{m.id.slice(0, 12)}…</span></td>
                  <td><StateBadge state={m.state} /></td>
                  <td className="mono" style={{ fontSize: 11 }}>{m.attributes ? JSON.stringify(m.attributes) : '{}'}</td>
                  <td><span className="mono">{m.body}</span></td>
                  <td className="muted">{formatTime(m.visible_at)}</td>
                  <td className="right">
                    {m.state === 'in_flight' && <Button variant="ghost" onClick={() => ack(m)}>Ack (delete)</Button>}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Card>
    </div>
  )
}

function TopicPanel({ project, topic }: { project: string; topic: Topic }) {
  const { show } = useToast()
  const [subs, setSubs] = useState<Subscriber[] | null>(null)
  const [subMode, setSubMode] = useState<'queue' | 'webhook'>('queue')
  const [queueName, setQueueName] = useState('')
  const [webhookUrl, setWebhookUrl] = useState('')
  const [publishBody, setPublishBody] = useState('')
  const [busy, setBusy] = useState(false)

  async function publish(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    try {
      const r = await api<PublishResult>(SERVICE, `/topic/${encodeURIComponent(project)}/${encodeURIComponent(topic.name)}/publish`, {
        method: 'POST',
        body: JSON.stringify({ body: publishBody }),
      })
      show('success', `Published — ${r.queue_copies} queue copies · ${r.webhook_deliveries} webhook deliveries`)
      setPublishBody('')
    } catch (err) {
      show('error', err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }

  async function loadSubs() {
    try {
      const r = await api<{ subscribers: Subscriber[] }>(SERVICE, `/topic/${encodeURIComponent(project)}/${encodeURIComponent(topic.name)}/subscribers`)
      setSubs(r.subscribers ?? [])
    } catch (err) {
      show('error', err instanceof Error ? err.message : String(err))
    }
  }

  async function subscribe(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    try {
      const body = subMode === 'queue' ? { queue_name: queueName } : { webhook_url: webhookUrl }
      await api<Subscriber>(SERVICE, `/topic/${encodeURIComponent(project)}/${encodeURIComponent(topic.name)}/subscribe`, {
        method: 'POST',
        body: JSON.stringify(body),
      })
      show('success', `Subscribed ${subMode === 'queue' ? queueName : webhookUrl}`)
      setQueueName('')
      setWebhookUrl('')
      loadSubs()
    } catch (err) {
      show('error', err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }

  async function unsubscribe(id: string) {
    try {
      await api(SERVICE, `/topic/${encodeURIComponent(project)}/${encodeURIComponent(topic.name)}/unsubscribe`, {
        method: 'POST',
        body: JSON.stringify({ subscriber_id: id }),
      })
      show('success', 'Unsubscribed')
      loadSubs()
    } catch (err) {
      show('error', err instanceof Error ? err.message : String(err))
    }
  }

  return (
    <div>
      <Card title={`Publish to ${topic.name}`}>
        <form onSubmit={publish}>
          <Field label="Message body">
            <textarea value={publishBody} onChange={(e) => setPublishBody(e.target.value)} placeholder="Broadcast content" required />
          </Field>
          <div className="row-end">
            <Button variant="primary" type="submit" disabled={busy}>{busy ? 'Publishing…' : 'Publish'}</Button>
          </div>
        </form>
      </Card>
      <div className="section-gap" />
      <Card title="Subscribers" actions={<Button variant="ghost" onClick={loadSubs}>{subs ? 'Refresh' : 'Load subscribers'}</Button>}>
        {subs === null ? (
          <EmptyState icon="🔗" title="Load subscribers" hint="Attach queues (fan-out targets) or webhook URLs (HTTP push) to this topic." />
        ) : subs.length === 0 ? (
          <EmptyState icon="🔗" title="No subscribers yet" hint="Publish only fans out to subscribers; add at least one." />
        ) : (
          <table className="data">
            <thead><tr><th>Kind</th><th>Target</th><th>Status</th><th>Created</th><th className="right">Actions</th></tr></thead>
            <tbody>
              {subs.map((s) => (
                <tr key={s.id}>
                  <td><Badge tone={s.kind === 'queue' ? 'info' : 'warn'}>{s.kind}</Badge></td>
                  <td><span className="mono">{s.kind === 'queue' ? s.queue_name : s.webhook_url}</span></td>
                  <td><StateBadge state={s.status} /></td>
                  <td className="muted">{formatTime(s.created_at)}</td>
                  <td className="right"><Button variant="danger" onClick={() => unsubscribe(s.id)}>Unsubscribe</Button></td>
                </tr>
              ))}
            </tbody>
          </table>
        )}

        <div className="section-gap" />
        <div className="tabs" style={{ borderBottom: 'none', marginBottom: 8 }}>
          {(['queue', 'webhook'] as const).map((k) => (
            <button key={k} type="button" onClick={() => setSubMode(k)} className={`tab ${subMode === k ? 'active' : ''}`} style={{ border: 'none', background: 'none', cursor: 'pointer', fontFamily: 'inherit', fontSize: 14 }}>
              {k}
            </button>
          ))}
        </div>
        <form onSubmit={subscribe} className="row" style={{ alignItems: 'flex-start' }}>
          {subMode === 'queue' ? (
            <input value={queueName} onChange={(e) => setQueueName(e.target.value)} placeholder="queue name" style={{ flex: 1 }} required />
          ) : (
            <input value={webhookUrl} onChange={(e) => setWebhookUrl(e.target.value)} placeholder="https://example.com/hooks/iris" style={{ flex: 1 }} required />
          )}
          <Button variant="primary" type="submit" disabled={busy}>Subscribe</Button>
        </form>
      </Card>
    </div>
  )
}

export default function IrisPage() {
  const [project, setProject] = useState(() => sessionStorage.getItem(`olympus.project.${SERVICE}`) || '')
  const [tab, setTab] = useState<'queues' | 'topics'>('queues')
  const [creating, setCreating] = useState(false)
  const [openQueue, setOpenQueue] = useState<Queue | null>(null)
  const [openTopic, setOpenTopic] = useState<Topic | null>(null)
  const { show } = useToast()

  const queues = useAsync<Queue[]>(() => {
    if (!project) return Promise.resolve([])
    return api<{ queues: Queue[] }>(SERVICE, '/queues', { query: { project } }).then((r) => r.queues ?? [])
  })
  const topics = useAsync<Topic[]>(() => {
    if (!project) return Promise.resolve([])
    return api<{ topics: Topic[] }>(SERVICE, '/topics', { query: { project } }).then((r) => r.topics ?? [])
  })

  async function delQueue(name: string) {
    try {
      await api(SERVICE, `/queue/${encodeURIComponent(project)}/${encodeURIComponent(name)}`, { method: 'DELETE' })
      show('success', `Deleted queue ${name}`)
      queues.refetch()
    } catch (err) {
      show('error', err instanceof Error ? err.message : String(err))
    }
  }

  async function delTopic(name: string) {
    try {
      await api(SERVICE, `/topic/${encodeURIComponent(project)}/${encodeURIComponent(name)}`, { method: 'DELETE' })
      show('success', `Deleted topic ${name}`)
      topics.refetch()
    } catch (err) {
      show('error', err instanceof Error ? err.message : String(err))
    }
  }

  return (
    <div>
      <PageHeader icon="📨" title="Iris" tagline="Messaging broker — SQS-style queues and SNS-style topics with fan-out and webhooks.">
        <Button variant="ghost" onClick={() => { queues.refetch(); topics.refetch() }}>Refresh</Button>
        <Button variant="primary" disabled={!project} onClick={() => setCreating(true)}>+ {tab === 'queues' ? 'Queue' : 'Topic'}</Button>
      </PageHeader>
      <ProjectPicker service={SERVICE} onSelect={setProject} />

      <div className="tabs">
        {(['queues', 'topics'] as const).map((t) => (
          <button key={t} type="button" className={`tab ${tab === t ? 'active' : ''}`} onClick={() => setTab(t)} style={{ border: 'none', background: 'none', cursor: 'pointer', fontFamily: 'inherit', fontSize: 14 }}>
            {t}
          </button>
        ))}
      </div>

      {!project && <EmptyState icon="📨" title="Select a project" hint="Choose a project to work with its queues and topics." />}

      {tab === 'queues' && project && (
        <Card title={`Queues · ${project}`}>
          {queues.loading ? <p>Loading…</p> : queues.data?.length === 0 ? <EmptyState icon="📨" title="No queues" hint="Create a queue to start sending and polling messages." /> : (
            <table className="data">
              <thead><tr><th>Name</th><th>State</th><th>Visibility</th><th>Retention</th><th className="right">Actions</th></tr></thead>
              <tbody>
                {queues.data?.map((q) => (
                  <tr key={q.id}>
                    <td><span className="mono">{q.name}</span></td>
                    <td><StateBadge state={q.state} /></td>
                    <td className="num">{q.visibility_timeout_sec}s</td>
                    <td className="num">{q.message_retention_sec}s</td>
                    <td className="right">
                      <Button variant="ghost" onClick={() => setOpenQueue(q)}>Send / Poll</Button>
                      <Button variant="danger" onClick={() => delQueue(q.name)}>Delete</Button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </Card>
      )}

      {tab === 'topics' && project && (
        <Card title={`Topics · ${project}`}>
          {topics.loading ? <p>Loading…</p> : topics.data?.length === 0 ? <EmptyState icon="📣" title="No topics" hint="Create a topic to publish and subscribe." /> : (
            <table className="data">
              <thead><tr><th>Name</th><th>State</th><th className="right">Actions</th></tr></thead>
              <tbody>
                {topics.data?.map((t) => (
                  <tr key={t.id}>
                    <td><span className="mono">{t.name}</span></td>
                    <td><StateBadge state={t.state} /></td>
                    <td className="right">
                      <Button variant="ghost" onClick={() => setOpenTopic(t)}>Publish / Subscribers</Button>
                      <Button variant="danger" onClick={() => delTopic(t.name)}>Delete</Button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </Card>
      )}

      <Modal open={creating} onClose={() => setCreating(false)} title={tab === 'queues' ? 'Create queue' : 'Create topic'} footer={<Button variant="ghost" onClick={() => setCreating(false)}>Cancel</Button>}>
        {tab === 'queues' ? <CreateQueue project={project} onDone={() => { setCreating(false); queues.refetch() }} /> : <CreateTopic project={project} onDone={() => { setCreating(false); topics.refetch() }} />}
      </Modal>

      {openQueue && (
        <Modal open wide onClose={() => setOpenQueue(null)} title={`Queue ${openQueue.name}`} footer={
          <div className="row-end">
            <CopyButton text={openQueue.id} label="Copy id" />
            <Button variant="ghost" onClick={() => setOpenQueue(null)}>Close</Button>
          </div>
        }>
          <QueuePanel project={project} queue={openQueue} />
        </Modal>
      )}

      {openTopic && (
        <Modal open wide onClose={() => setOpenTopic(null)} title={`Topic ${openTopic.name}`} footer={
          <div className="row-end">
            <CopyButton text={openTopic.id} label="Copy id" />
            <Button variant="ghost" onClick={() => setOpenTopic(null)}>Close</Button>
          </div>
        }>
          <TopicPanel project={project} topic={openTopic} />
        </Modal>
      )}
    </div>
  )
}