import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from 'react'
import {
  Button as HeroButton,
  Card as HeroCard,
  Chip,
  Label,
  ListBox,
  Modal as HeroModal,
  Select as HeroSelect,
  Spinner as HeroSpinner,
  Tabs as HeroTabs,
  toast,
  type Key,
} from '@heroui/react'

// ---- Primitives -------------------------------------------------------------

type ButtonVariant = 'default' | 'primary' | 'danger' | 'ghost'
type ButtonSize = 'sm' | 'md' | 'lg'

const buttonVariant: Record<ButtonVariant, 'primary' | 'secondary' | 'danger' | 'ghost'> = {
  default: 'secondary',
  primary: 'primary',
  danger: 'danger',
  ghost: 'ghost',
}

export function Button({
  variant = 'default',
  size,
  className = '',
  disabled,
  fullWidth,
  type = 'button',
  onPress,
  children,
  style,
  ariaLabel,
}: {
  variant?: ButtonVariant
  size?: ButtonSize
  className?: string
  disabled?: boolean
  fullWidth?: boolean
  type?: 'button' | 'submit' | 'reset'
  onPress?: () => void
  children?: ReactNode
  style?: React.CSSProperties
  ariaLabel?: string
}) {
  return (
    <HeroButton
      variant={buttonVariant[variant]}
      size={size}
      isDisabled={disabled}
      fullWidth={fullWidth}
      type={type}
      onPress={onPress}
      style={style}
      aria-label={ariaLabel}
      className={className}
    >
      {children}
    </HeroButton>
  )
}

export function Badge({
  tone = 'neutral',
  children,
  className,
}: {
  tone?: 'ok' | 'warn' | 'danger' | 'neutral' | 'info'
  children: ReactNode
  className?: string
}) {
  const color = ({
    ok: 'success',
    warn: 'warning',
    danger: 'danger',
    info: 'accent',
    neutral: 'default',
  } as const)[tone]
  return (
    <Chip color={color} size="sm" className={className}>
      {children}
    </Chip>
  )
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

export function Card({
  title,
  actions,
  children,
  className = '',
}: {
  title?: ReactNode
  actions?: ReactNode
  children: ReactNode
  className?: string
}) {
  return (
    <HeroCard className={className}>
      {title !== undefined && (
        <HeroCard.Header className="flex items-center justify-between gap-3 border-b border-border">
          <HeroCard.Title>{title}</HeroCard.Title>
          {actions && <div className="flex items-center gap-2">{actions}</div>}
        </HeroCard.Header>
      )}
      <HeroCard.Content>{children}</HeroCard.Content>
    </HeroCard>
  )
}

export function Field({
  label,
  hint,
  children,
  className = '',
}: {
  label?: ReactNode
  hint?: ReactNode
  children: ReactNode
  className?: string
}) {
  return (
    <div className={`mb-3 flex flex-col gap-1.5 ${className}`}>
      {label && <span className="text-xs font-semibold text-muted">{label}</span>}
      {children}
      {hint && <span className="text-xs text-muted">{hint}</span>}
    </div>
  )
}

export interface SelectOption {
  value: string
  label: ReactNode
}

// Thin wrapper over HeroUI's compound Select so pages can keep a simple
// `value`/`onChange` string contract (empty string means "nothing selected").
export function SelectField({
  label,
  hint,
  value,
  onChange,
  options,
  placeholder = 'Select…',
  disabled,
  isRequired,
  name,
  className = '',
}: {
  label: string
  hint?: string
  value: string
  onChange: (v: string) => void
  options: SelectOption[]
  placeholder?: string
  disabled?: boolean
  isRequired?: boolean
  name?: string
  className?: string
}) {
  return (
    <div className="flex flex-col gap-1.5">
      <HeroSelect
        className={className}
        placeholder={placeholder}
        value={(value || null) as Key | null}
        onChange={(v) => onChange(v === null ? '' : String(v))}
        isDisabled={disabled}
        isRequired={isRequired}
        name={name}
      >
        <Label>{label}</Label>
        <HeroSelect.Trigger>
          <HeroSelect.Value />
          <HeroSelect.Indicator />
        </HeroSelect.Trigger>
        <HeroSelect.Popover>
          <ListBox>
            {options.map((o) => (
              <ListBox.Item key={o.value} id={o.value} textValue={String(o.label)}>
                {o.label}
                <ListBox.ItemIndicator />
              </ListBox.Item>
            ))}
          </ListBox>
        </HeroSelect.Popover>
      </HeroSelect>
      {hint && <span className="text-xs text-muted">{hint}</span>}
    </div>
  )
}

export function Spinner() {
  return <HeroSpinner size="sm" />
}

export function EmptyState({ icon = '🗂️', title, hint }: { icon?: string; title: string; hint?: ReactNode }) {
  return (
    <div className="py-7 px-4 text-center text-muted">
      <div className="mb-2 text-[32px]">{icon}</div>
      <div className="font-semibold text-foreground">{title}</div>
      {hint && <div className="mt-1 text-[13px]">{hint}</div>}
    </div>
  )
}

// ---- Segmented tabs (no panels; content is rendered by the caller) ----------

export function SegmentedTabs({
  tabs,
  selected,
  onSelect,
  ariaLabel,
}: {
  tabs: readonly string[]
  selected: string
  onSelect: (t: string) => void
  ariaLabel?: string
}) {
  return (
    <HeroTabs
      variant="secondary"
      selectedKey={selected}
      onSelectionChange={(k) => onSelect(String(k))}
      aria-label={ariaLabel}
    >
      <HeroTabs.ListContainer>
        <HeroTabs.List>
          {tabs.map((t) => (
            <HeroTabs.Tab key={t} id={t}>
              {t}
              <HeroTabs.Indicator />
            </HeroTabs.Tab>
          ))}
        </HeroTabs.List>
      </HeroTabs.ListContainer>
    </HeroTabs>
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
  return (
    <HeroModal.Backdrop isOpen={open} onOpenChange={(v) => { if (!v) onClose() }}>
      <HeroModal.Container size={wide ? 'lg' : 'md'}>
        <HeroModal.Dialog>
          <HeroModal.CloseTrigger />
          <HeroModal.Header>
            <HeroModal.Heading>{title}</HeroModal.Heading>
          </HeroModal.Header>
          <HeroModal.Body>{children}</HeroModal.Body>
          {footer !== undefined && <HeroModal.Footer>{footer}</HeroModal.Footer>}
        </HeroModal.Dialog>
      </HeroModal.Container>
    </HeroModal.Backdrop>
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
      <Button variant="danger" onPress={() => setOpen(true)} disabled={busy}>{children ?? label}</Button>
      <Modal open={open} onClose={() => setOpen(false)} title={confirmLabel} footer={<>
        <Button variant="ghost" onPress={() => setOpen(false)}>Cancel</Button>
        <Button variant="danger" onPress={run} disabled={busy}>{confirmLabel}</Button>
      </>}>
        <p>Are you sure? This action cannot be undone.</p>
      </Modal>
    </>
  )
}

// ---- Toasts (HeroUI `toast()` + <Toast.Provider /> mounted in AppShell) -----

export type ToastKind = 'info' | 'success' | 'error'

export function useToast() {
  return {
    show: (kind: ToastKind, message: string) => {
      if (kind === 'success') toast.success(message)
      else if (kind === 'error') toast.danger(message)
      else toast.info(message)
    },
  }
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
