import { useEffect, useId, type ComponentType, type InputHTMLAttributes, type ReactNode, type SelectHTMLAttributes, type SVGProps, type TextareaHTMLAttributes } from 'react'
import { createPortal } from 'react-dom'

type IconComponent = ComponentType<SVGProps<SVGSVGElement>>

export function PageHeader({
  title,
  description,
  actions,
}: {
  title: string
  description: string
  actions?: ReactNode
}) {
  return (
    <div className="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
      <div className="max-w-2xl">
        <h1 className="text-2xl font-semibold tracking-[-0.025em] text-ink sm:text-[1.75rem]">
          {title}
        </h1>
        <p className="mt-1.5 text-sm leading-6 text-muted">{description}</p>
      </div>
      {actions ? <div className="flex shrink-0 items-center gap-2">{actions}</div> : null}
    </div>
  )
}

export function Panel({
  title,
  description,
  action,
  children,
  className = '',
  bodyClassName = '',
}: {
  title?: string
  description?: string
  action?: ReactNode
  children: ReactNode
  className?: string
  bodyClassName?: string
}) {
  return (
    <section className={`overflow-hidden rounded-[10px] border border-line bg-surface ${className}`}>
      {title ? (
        <header className="flex flex-wrap items-start justify-between gap-3 border-b border-line px-4 py-3.5 sm:px-5">
          <div>
            <h2 className="text-sm font-semibold tracking-[-0.01em] text-ink">{title}</h2>
            {description ? <p className="mt-0.5 text-xs leading-5 text-muted">{description}</p> : null}
          </div>
          {action}
        </header>
      ) : null}
      <div className={bodyClassName}>{children}</div>
    </section>
  )
}

export function StatusPill({
  tone,
  children,
  dot = true,
}: {
  tone: 'ok' | 'warn' | 'error' | 'neutral'
  children: ReactNode
  dot?: boolean
}) {
  const tones = {
    ok: 'border-accent/25 bg-accent-soft text-accent',
    warn: 'border-signal/25 bg-signal/10 text-signal',
    error: 'border-danger/25 bg-danger/10 text-danger',
    neutral: 'border-line bg-surface-2 text-muted',
  }
  const dots = {
    ok: 'bg-accent',
    warn: 'bg-signal',
    error: 'bg-danger',
    neutral: 'bg-faint',
  }
  return (
    <span
      className={`inline-flex items-center gap-1.5 rounded-full border px-2 py-0.5 text-[11px] font-medium leading-5 ${tones[tone]}`}
    >
      {dot ? <span className={`size-1.5 rounded-full ${dots[tone]}`} aria-hidden="true" /> : null}
      {children}
    </span>
  )
}

export function EmptyState({
  icon: Icon,
  title,
  description,
  action,
  compact = false,
}: {
  icon: IconComponent
  title: string
  description: string
  action?: ReactNode
  compact?: boolean
}) {
  return (
    <div
      className={`flex flex-col items-center justify-center rounded-lg border border-dashed border-line-strong bg-surface-2/45 text-center ${
        compact ? 'px-5 py-8' : 'px-6 py-14'
      }`}
    >
      <span className="grid size-10 place-items-center rounded-full border border-line bg-surface text-muted">
        <Icon className="size-5" />
      </span>
      <h3 className="mt-3 text-sm font-semibold text-ink">{title}</h3>
      <p className="mt-1 max-w-md text-xs leading-5 text-muted">{description}</p>
      {action ? <div className="mt-4">{action}</div> : null}
    </div>
  )
}

export function PrimaryButton({
  children,
  disabled,
  title,
  onClick,
  type = 'button',
}: {
  children: ReactNode
  disabled?: boolean
  title?: string
  onClick?: () => void
  type?: 'button' | 'submit'
}) {
  return (
    <button
      type={type}
      onClick={onClick}
      disabled={disabled}
      title={title}
      className="inline-flex h-9 items-center justify-center gap-2 rounded-md bg-accent px-3 text-xs font-semibold text-canvas transition-colors hover:bg-accent/90 disabled:cursor-not-allowed disabled:opacity-45"
    >
      {children}
    </button>
  )
}

export function SecondaryButton({
  children,
  disabled,
  title,
  onClick,
  type = 'button',
}: {
  children: ReactNode
  disabled?: boolean
  title?: string
  onClick?: () => void
  type?: 'button' | 'submit'
}) {
  return (
    <button
      type={type}
      onClick={onClick}
      disabled={disabled}
      title={title}
      className="inline-flex h-9 items-center justify-center gap-2 rounded-md border border-line bg-surface px-3 text-xs font-semibold text-ink transition-colors hover:bg-surface-2 disabled:cursor-not-allowed disabled:opacity-45"
    >
      {children}
    </button>
  )
}

export function DefinitionRow({
  label,
  children,
}: {
  label: string
  children: ReactNode
}) {
  return (
    <div className="grid gap-1 border-b border-line py-3 last:border-b-0 sm:grid-cols-[11rem_1fr] sm:gap-4">
      <dt className="text-xs font-medium text-muted">{label}</dt>
      <dd className="text-xs leading-5 text-ink">{children}</dd>
    </div>
  )
}

export function Banner({
  tone = 'neutral',
  title,
  children,
}: {
  tone?: 'ok' | 'warn' | 'error' | 'neutral'
  title?: string
  children: ReactNode
}) {
  const styles = {
    ok: 'border-accent/30 bg-accent-soft/45 text-ink',
    warn: 'border-signal/30 bg-signal/[0.08] text-ink',
    error: 'border-danger/30 bg-danger/[0.08] text-ink',
    neutral: 'border-line bg-surface-2/60 text-ink',
  }
  return (
    <div className={`rounded-md border px-3 py-2.5 text-[11px] leading-5 ${styles[tone]}`} role={tone === 'error' ? 'alert' : 'status'}>
      {title ? <p className="font-semibold">{title}</p> : null}
      <div className={title ? 'mt-0.5 text-muted' : 'text-muted'}>{children}</div>
    </div>
  )
}

export function FormField({
  label,
  hint,
  children,
  className = '',
}: {
  label: string
  hint?: string
  children: ReactNode
  className?: string
}) {
  return (
    <label className={`block ${className}`}>
      <span className="mb-1.5 block text-[11px] font-semibold text-ink">{label}</span>
      {children}
      {hint ? <span className="mt-1 block text-[10px] leading-4 text-faint">{hint}</span> : null}
    </label>
  )
}

export function TextInput({ className = '', ...props }: InputHTMLAttributes<HTMLInputElement>) {
  return (
    <input
      {...props}
      className={`h-9 w-full rounded-md border border-line bg-canvas px-2.5 text-xs text-ink placeholder:text-faint disabled:cursor-not-allowed disabled:opacity-50 ${className}`}
    />
  )
}

export function Select({ className = '', ...props }: SelectHTMLAttributes<HTMLSelectElement>) {
  return (
    <select
      {...props}
      className={`h-9 w-full rounded-md border border-line bg-canvas px-2 text-xs text-ink disabled:cursor-not-allowed disabled:opacity-50 ${className}`}
    />
  )
}

export function Textarea({ className = '', ...props }: TextareaHTMLAttributes<HTMLTextAreaElement>) {
  return (
    <textarea
      {...props}
      className={`min-h-20 w-full rounded-md border border-line bg-canvas px-2.5 py-2 text-xs leading-5 text-ink placeholder:text-faint disabled:cursor-not-allowed disabled:opacity-50 ${className}`}
    />
  )
}

export function Checkbox({
  label,
  hint,
  ...props
}: InputHTMLAttributes<HTMLInputElement> & { label: string; hint?: string }) {
  const id = useId()
  return (
    <div className="flex items-start gap-2.5">
      <input
        id={id}
        type="checkbox"
        {...props}
        className="mt-0.5 size-3.5 rounded border-line bg-canvas accent-accent"
      />
      <label htmlFor={id} className="text-[11px] leading-5 text-ink">
        <span className="font-medium">{label}</span>
        {hint ? <span className="mt-0.5 block text-[10px] text-faint">{hint}</span> : null}
      </label>
    </div>
  )
}

export function Dialog({
  open,
  onClose,
  title,
  description,
  children,
  footer,
  wide = false,
}: {
  open: boolean
  onClose: () => void
  title: string
  description?: string
  children: ReactNode
  footer?: ReactNode
  wide?: boolean
}) {
  useEffect(() => {
    if (!open) return
    const previous = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    const onKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', onKey)
    return () => {
      document.body.style.overflow = previous
      window.removeEventListener('keydown', onKey)
    }
  }, [open, onClose])

  if (!open) return null
  return createPortal(
    <div className="fixed inset-0 z-50 flex items-end justify-center p-0 sm:items-center sm:p-6">
      <button
        type="button"
        className="absolute inset-0 bg-canvas/80 backdrop-blur-[2px]"
        aria-label="Close dialog"
        onClick={onClose}
      />
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="dialog-title"
        className={`relative max-h-[92dvh] w-full overflow-y-auto rounded-t-[12px] border border-line bg-surface shadow-2xl sm:rounded-[12px] ${
          wide ? 'sm:max-w-3xl' : 'sm:max-w-lg'
        }`}
      >
        <header className="sticky top-0 z-10 flex items-start justify-between gap-4 border-b border-line bg-surface px-4 py-3.5 sm:px-5">
          <div>
            <h2 id="dialog-title" className="text-sm font-semibold tracking-[-0.01em] text-ink">
              {title}
            </h2>
            {description ? <p className="mt-0.5 text-[11px] leading-5 text-muted">{description}</p> : null}
          </div>
          <button
            type="button"
            onClick={onClose}
            className="grid size-7 shrink-0 place-items-center rounded-md border border-line bg-surface text-muted transition-colors hover:bg-surface-2 hover:text-ink"
            aria-label="Close"
          >
            <span aria-hidden="true" className="text-base leading-none">×</span>
          </button>
        </header>
        <div className="px-4 py-4 sm:px-5">{children}</div>
        {footer ? (
          <footer className="flex items-center justify-end gap-2 border-t border-line bg-surface-2/40 px-4 py-3 sm:px-5">
            {footer}
          </footer>
        ) : null}
      </div>
    </div>,
    document.body,
  )
}

export function Spinner({ label = 'Loading' }: { label?: string }) {
  return (
    <div className="flex items-center justify-center gap-2 px-4 py-10 text-[11px] text-muted" role="status">
      <span className="size-3.5 animate-spin rounded-full border-2 border-line border-t-accent" aria-hidden="true" />
      {label}
    </div>
  )
}

export function DangerButton({
  children,
  disabled,
  title,
  onClick,
  type = 'button',
}: {
  children: ReactNode
  disabled?: boolean
  title?: string
  onClick?: () => void
  type?: 'button' | 'submit'
}) {
  return (
    <button
      type={type}
      onClick={onClick}
      disabled={disabled}
      title={title}
      className="inline-flex h-9 items-center justify-center gap-2 rounded-md border border-danger/35 bg-danger/10 px-3 text-xs font-semibold text-danger transition-colors hover:bg-danger/15 disabled:cursor-not-allowed disabled:opacity-45"
    >
      {children}
    </button>
  )
}
