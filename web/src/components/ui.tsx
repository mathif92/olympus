import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from 'react'

// ---- Primitives -------------------------------------------------------------

export function Button({
  variant = 'default',
  className = '',
  ...props
}: React.ButtonHTMLAttributes<HTMLButtonElement> & { variant?: 'default' | 'primary' | 'danger' | 'ghost' }) {
  const styles = {
    default: 'btn',
    primary: 'btn btn-primary',
    danger: 'btn btn-danger',
    ghost: 'btn btn-ghost',
  }[variant]
  return <button className={`${styles} ${className}`} {...props} />
}

export function Badge({ tone = 'neutral', children }: { tone?: 'ok' | 'warn' | 'danger' | 'neutral' | 'info'; children: ReactNode }) {
  return <span className={`badge badge-${tone}`}>{children}</span>
}

export function stateTone(state: string): 'ok' | 'warn' | 'danger' | 'info' | 'neutral' {
  const s = state.toLowerCase()
  if (['active', 'running', 'healthy', 'delivered', 'ok', 'success'].includes(s)) return 'ok'
  if (['failed', 'terminated', 'deleted', 'deleting', 'down'].includes(s)) return 'danger'
  if (['pending', 'creating', 'stopping', 'starting', 'updating', 'in_flight', 'stopped'].includes(s)) return 'warn'
  if (['creating', 'pending'].includes(s)) return 'info'
  return 'neutral'
}

export function StateBadge({ state }: { state: string }) {
  return <Badge tone={stateTone(state)}>{state}</Badge>
}

export function Card({ title, actions, children, className = '' }: { title?: ReactNode; actions?: ReactNode; children: ReactNode; className?: string }) {
  return (
    <div className={`card ${className}`}>
      {title !== undefined && (
        <div className="card-head">
          <h3>{title}</h3>
          {actions && <div className="card-actions">{actions}</div>}
        </div>
      )}
      <div className="card-body">{children}</div>
    </div>
  )
}

export function Field({ label, hint, children }: { label: string; hint?: string; children: ReactNode }) {
  return (
    <label className="field">
      <span className="field-label">{label}</span>
      {children}
      {hint && <span className="field-hint">{hint}</span>}
    </label>
  )
}

export function Spinner() {
  return <span className="spinner" aria-label="loading" />
}

export function EmptyState({ icon = '🗂️', title, hint }: { icon?: string; title: string; hint?: ReactNode }) {
  return (
    <div className="empty">
      <div className="empty-icon">{icon}</div>
      <div className="empty-title">{title}</div>
      {hint && <div className="empty-hint">{hint}</div>}
    </div>
  )
}

// ---- Modal ------------------------------------------------------------------

export function Modal({
  open,
  title,
  onClose,
  children,
  footer,
  wide = false,
}: {
  open: boolean
  title: ReactNode
  onClose: () => void
  children: ReactNode
  footer?: ReactNode
  wide?: boolean
}) {
  if (!open) return null
  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className={`modal ${wide ? 'modal-wide' : ''}`} onClick={(e) => e.stopPropagation()}>
        <div className="modal-head">
          <h3>{title}</h3>
          <button className="btn btn-ghost" onClick={onClose} aria-label="close">✕</button>
        </div>
        <div className="modal-body">{children}</div>
        {footer && <div className="modal-foot">{footer}</div>}
      </div>
    </div>
  )
}

export function ConfirmButton({
  onConfirm,
  label = 'Delete',
  confirmLabel = 'Confirm',
  children,
  busy,
}: {
  onConfirm: () => Promise<void> | void
  label?: string
  confirmLabel?: string
  children?: ReactNode
  busy?: boolean
}) {
  const [open, setOpen] = useState(false)
  const run = async () => {
    await onConfirm()
    setOpen(false)
  }
  return (
    <>
      <Button variant="danger" onClick={() => setOpen(true)} disabled={busy}>{children ?? label}</Button>
      <Modal open={open} onClose={() => setOpen(false)} title={confirmLabel} footer={<>
        <Button variant="ghost" onClick={() => setOpen(false)}>Cancel</Button>
        <Button variant="danger" onClick={run} disabled={busy}>{confirmLabel}</Button>
      </>}>
        <p>Are you sure? This action cannot be undone.</p>
      </Modal>
    </>
  )
}

// ---- Toasts -----------------------------------------------------------------

export type ToastKind = 'info' | 'success' | 'error'
export interface Toast {
  id: number
  kind: ToastKind
  message: string
}

interface ToastCtxType {
  show: (kind: ToastKind, message: string) => void
}
const ToastCtx = createContext<ToastCtxType | null>(null)

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([])
  const show = useCallback((kind: ToastKind, message: string) => {
    const id = Date.now() + Math.random()
    setToasts((t) => [...t, { id, kind, message }])
    setTimeout(() => setToasts((t) => t.filter((x) => x.id !== id)), 5000)
  }, [])
  return (
    <ToastCtx.Provider value={{ show }}>
      {children}
      <div className="toasts">
        {toasts.map((t) => (
          <div key={t.id} className={`toast toast-${t.kind}`}>{t.message}</div>
        ))}
      </div>
    </ToastCtx.Provider>
  )
}

export function useToast() {
  const ctx = useContext(ToastCtx)
  if (!ctx) throw new Error('useToast must be used within ToastProvider')
  return ctx
}

// ---- Async data hook --------------------------------------------------------

export interface AsyncState<T> {
  data: T | null
  loading: boolean
  error: string | null
  refetch: () => void
}

export function useAsync<T>(fn: () => Promise<T>): AsyncState<T> {
  const [state, setState] = useState<{ data: T | null; loading: boolean; error: string | null }>({
    data: null,
    loading: true,
    error: null,
  })
  const [tick, setTick] = useState(0)

  const run = useCallback(() => setTick((t) => t + 1), [])
  const fnRef = useRef(fn)

  useEffect(() => {
    let cancelled = false
    fnRef.current = fn
    setState((s) => ({ ...s, loading: true, error: null }))
    fn().then(
      (data) => {
        if (!cancelled) setState({ data, loading: false, error: null })
      },
      (err) => {
        if (!cancelled) setState({ data: null, loading: false, error: err instanceof Error ? err.message : String(err) })
      },
    )
    return () => {
      cancelled = true
    }
  }, [fn, tick])

  return { ...state, refetch: run }
}