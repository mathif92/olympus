import { useMemo, useState, type ReactNode } from 'react'
import { Input } from '@heroui/react'
import { useAsync, Card, EmptyState, Spinner, SelectField, Button, Field } from './ui'
import type { ApiProject } from '../api/types'
import { api, type Service } from '../api/client'

// PageHeader renders a service page title with its tagline.
export function PageHeader({ icon, title, tagline, children }: { icon: string; title: string; tagline: string; children?: ReactNode }) {
  return (
    <div className="mb-5 flex flex-wrap items-end justify-between gap-4">
      <div>
        <h1 className="m-0 text-[22px] font-semibold text-foreground">{icon} {title}</h1>
        <p className="mt-1 mb-0 text-muted">{tagline}</p>
      </div>
      <div className="flex gap-2">{children}</div>
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
      <div className="mb-4 max-w-[420px]">
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
            <Field label="Project">
              <Input name="project" placeholder="Type a project name…" />
            </Field>
            <Button variant="primary" type="submit">Select</Button>
          </form>
        </Card>
      </div>
    )
  }

  return (
    <div className="mb-4 max-w-[420px]">
      <SelectField
        label="Project"
        value={candidates.includes(selected) ? selected : ''}
        onChange={pick}
        options={candidates.map((c) => ({ value: c, label: c }))}
        placeholder="— select project —"
      />
    </div>
  )
}

export { useAsync }
