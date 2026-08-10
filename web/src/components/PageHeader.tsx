import { useMemo, useState, type ReactNode } from 'react'
import { useAsync, Card, EmptyState, Spinner } from './ui'
import type { ApiProject } from '../api/types'
import { api, type Service } from '../api/client'

// PageHeader renders a service page title with its tagline.
export function PageHeader({ icon, title, tagline, children }: { icon: string; title: string; tagline: string; children?: ReactNode }) {
  return (
    <div className="page-head">
      <div className="page-head-text">
        <h1>{icon} {title}</h1>
        <p>{tagline}</p>
      </div>
      <div className="page-head-actions">{children}</div>
    </div>
  )
}

// ProjectPicker selects a project from the service; the selection is stored per
// service in sessionStorage. Auto-picks the first when the list loads.
export function ProjectPicker({ service, onSelect }: { service: Service; onSelect: (project: string) => void }) {
  const key = `olympus.project.${service}`
  const [stored] = useState(() => sessionStorage.getItem(key) || '')
  const [selected, setSelected] = useState(stored)

  const projects = useAsync<ApiProject[]>(() =>
    api<{ projects: ApiProject[] }>(service, '/projects').then((r) => r.projects ?? []),
  )

  const candidates = useMemo(() => {
    if (!projects.data) return []
    const names = projects.data.map((p) => p.name)
    if (selected && !names.includes(selected)) return [...names, selected].sort()
    return names
  }, [projects.data, selected])

  const pick = (name: string) => {
    setSelected(name)
    sessionStorage.setItem(key, name)
    onSelect(name)
  }

  if (projects.loading) return <Spinner />

  if (candidates.length === 0) {
    return (
      <div className="project-picker">
        <Card>
          <EmptyState icon="🏗️" title="No projects yet" hint={<span>Create a project to get started, or type a project name below.</span>} />
          <form
            className="row-end"
            onSubmit={(e) => {
              e.preventDefault()
              const v = (e.currentTarget.elements.namedItem('project') as HTMLInputElement).value.trim()
              if (v) pick(v)
            }}
          >
            <input name="project" placeholder="Type a project name…" />
            <button className="btn btn-primary" type="submit">Select</button>
          </form>
        </Card>
      </div>
    )
  }

  return (
    <div className="project-picker">
      <label className="field">
        <span className="field-label">Project</span>
        <select value={selected} onChange={(e) => pick(e.target.value)}>
          {!candidates.includes(selected) && <option value="">—</option>}
          {candidates.map((c) => (
            <option key={c} value={c}>{c}</option>
          ))}
        </select>
      </label>
    </div>
  )
}

export { useAsync }