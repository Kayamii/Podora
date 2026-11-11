import type { ButtonHTMLAttributes, ReactNode } from 'react'
import type { Level } from '../services/health'

// --- Page structure ----------------------------------------------------------

interface PageHeaderProps {
  title: ReactNode
  /** One line under the title. */
  subtitle?: ReactNode
  /** Path back to the parent list. */
  breadcrumb?: { label: string; to: string }[]
  /** Badge shown beside the title. */
  status?: ReactNode
  /** Buttons at the right. */
  actions?: ReactNode
}

export function PageHeader({ title, subtitle, breadcrumb, status, actions }: PageHeaderProps) {
  return (
    <header className="mb-6">
      {breadcrumb && (
        <nav aria-label="Breadcrumb" className="mb-2 flex flex-wrap items-center gap-1.5 text-sm text-muted">
          {breadcrumb.map((b) => (
            <span key={b.to} className="flex items-center gap-1.5">
              <a href={b.to} className="hover:text-fg hover:underline">
                {b.label}
              </a>
              <span aria-hidden="true">/</span>
            </span>
          ))}
        </nav>
      )}
      <div className="flex flex-wrap items-start justify-between gap-x-6 gap-y-3">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
            <h1 className="text-2xl font-semibold tracking-tight break-all">{title}</h1>
            {status}
          </div>
          {subtitle && <p className="mt-1 text-fg-2">{subtitle}</p>}
        </div>
        {actions && <div className="flex shrink-0 flex-wrap gap-2">{actions}</div>}
      </div>
    </header>
  )
}

interface CardProps {
  title?: string
  /** Link or note at the right of the card header. */
  aside?: ReactNode
  children: ReactNode
  /** Set false for content that runs edge to edge, such as a table. */
  padded?: boolean
  className?: string
}

export function Card({ title, aside, children, padded = true, className = '' }: CardProps) {
  return (
    <section className={`rounded-lg border border-line bg-surface shadow-card ${className}`}>
      {title && (
        <div className="flex items-center justify-between gap-4 border-b border-line px-5 py-3">
          <h2 className="font-semibold">{title}</h2>
          {aside && <div className="text-sm text-muted">{aside}</div>}
        </div>
      )}
      <div className={padded ? 'p-5' : ''}>{children}</div>
    </section>
  )
}

/** Wraps a table so it scrolls sideways on narrow screens instead of the page. */
export function TableScroll({ children }: { children: ReactNode }) {
  return (
    <div className="overflow-x-auto">
      <table className="w-full">{children}</table>
    </div>
  )
}

export function Empty({ children }: { children: ReactNode }) {
  return <p className="px-5 py-8 text-center text-fg-2">{children}</p>
}

/** Two-column list of facts about one resource. */
export function Facts({ rows }: { rows: Record<string, ReactNode> }) {
  return (
    <dl className="-my-2.5">
      {Object.entries(rows).map(([label, value]) => (
        <div key={label} className="flex items-baseline justify-between gap-6 border-t border-line py-2.5 first:border-t-0">
          <dt className="shrink-0 text-fg-2">{label}</dt>
          <dd className="min-w-0 text-right font-medium break-words">{value}</dd>
        </div>
      ))}
    </dl>
  )
}

export function Labels({ labels }: { labels: Record<string, string> }) {
  const entries = Object.entries(labels)
  if (entries.length === 0) return <span className="font-normal text-muted">None</span>
  return (
    <ul className="flex flex-wrap justify-end gap-1.5">
      {entries.map(([k, v]) => (
        <li key={k} className="rounded bg-subtle px-1.5 py-0.5 font-mono text-[0.8125rem] font-normal break-all">
          {k}={v}
        </li>
      ))}
    </ul>
  )
}

// --- Status ------------------------------------------------------------------

const BADGE: Record<Level, { dot: string; pill: string }> = {
  normal: { dot: 'bg-ok', pill: 'bg-ok-soft text-ok-fg' },
  warning: { dot: 'bg-warn', pill: 'bg-warn-soft text-warn-fg' },
  fault: { dot: 'bg-danger', pill: 'bg-danger-soft text-danger-fg' },
  neutral: { dot: 'bg-muted', pill: 'bg-subtle text-fg-2' },
}

/** State as a coloured dot plus its name: for table cells. */
export function Status({ level, children }: { level: Level; children: ReactNode }) {
  return (
    <span className="inline-flex items-center gap-2 whitespace-nowrap">
      <span aria-hidden="true" className={`size-2 shrink-0 rounded-full ${BADGE[level].dot}`} />
      {children}
    </span>
  )
}

/** State as a pill: for page headers and summaries. */
export function Badge({ level, children }: { level: Level; children: ReactNode }) {
  return (
    <span
      className={`inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-sm font-medium whitespace-nowrap ${BADGE[level].pill}`}
    >
      <span aria-hidden="true" className={`size-1.5 rounded-full ${BADGE[level].dot}`} />
      {children}
    </span>
  )
}

const NOTICE: Record<'warning' | 'fault' | 'normal', string> = {
  warning: 'border-warn/40 bg-warn-soft text-warn-fg',
  fault: 'border-danger/40 bg-danger-soft text-danger-fg',
  normal: 'border-ok/40 bg-ok-soft text-ok-fg',
}

/** An inline message about something that happened or went wrong. */
export function Notice({ level, children }: { level: 'warning' | 'fault' | 'normal'; children: ReactNode }) {
  return (
    <div role={level === 'normal' ? 'status' : 'alert'} className={`mb-5 rounded-lg border px-4 py-3 ${NOTICE[level]}`}>
      {children}
    </div>
  )
}

// --- Controls ----------------------------------------------------------------

const BUTTON = {
  primary: 'border-transparent bg-accent text-white hover:bg-accent-hover',
  secondary: 'border-line-strong bg-surface text-fg hover:bg-subtle',
  danger: 'border-transparent bg-danger text-white hover:brightness-110',
  dangerOutline: 'border-line-strong bg-surface text-danger-fg hover:bg-danger-soft',
}

interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: keyof typeof BUTTON
  icon?: ReactNode
}

export function Button({ variant = 'secondary', icon, children, className = '', ...rest }: ButtonProps) {
  return (
    <button
      type="button"
      {...rest}
      className={`inline-flex h-9 cursor-pointer items-center gap-2 rounded-md border px-3 text-sm font-medium whitespace-nowrap transition-colors duration-100 disabled:cursor-not-allowed disabled:opacity-50 ${BUTTON[variant]} ${className}`}
    >
      {icon}
      {children}
    </button>
  )
}

interface SegmentedProps<T extends string | number> {
  /** Names the group for assistive tech. */
  label: string
  value: T
  options: readonly { value: T; label: string; disabled?: boolean; title?: string }[]
  onChange: (value: T) => void
}

/** A compact set of options where exactly one is selected. */
export function Segmented<T extends string | number>({ label, value, options, onChange }: SegmentedProps<T>) {
  return (
    <div role="group" aria-label={label} className="inline-flex h-9 rounded-md border border-line-strong bg-subtle p-0.5">
      {options.map((o) => (
        <button
          key={o.value}
          type="button"
          aria-pressed={o.value === value}
          disabled={o.disabled}
          title={o.title}
          onClick={() => onChange(o.value)}
          className={`cursor-pointer rounded px-2.5 text-sm font-medium whitespace-nowrap transition-colors duration-100 disabled:cursor-not-allowed disabled:opacity-45 ${
            o.value === value ? 'bg-surface text-fg shadow-card' : 'text-fg-2 hover:text-fg'
          }`}
        >
          {o.label}
        </button>
      ))}
    </div>
  )
}

/** A labelled on/off switch. */
export function Switch({ checked, onChange, children }: { checked: boolean; onChange: (v: boolean) => void; children: ReactNode }) {
  return (
    <label className="inline-flex h-9 cursor-pointer items-center gap-2 text-sm font-medium select-none">
      <input type="checkbox" role="switch" checked={checked} onChange={(e) => onChange(e.target.checked)} className="peer sr-only" />
      <span
        aria-hidden="true"
        className="relative h-5 w-9 rounded-full bg-line-strong transition-colors duration-150 peer-checked:bg-accent peer-focus-visible:outline-2 peer-focus-visible:outline-offset-2 peer-focus-visible:outline-accent after:absolute after:top-0.5 after:left-0.5 after:size-4 after:rounded-full after:bg-white after:transition-transform after:duration-150 peer-checked:after:translate-x-4"
      />
      {children}
    </label>
  )
}

/** A control with its label above it. */
export function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <label className="flex flex-col gap-1">
      <span className="text-[0.8125rem] font-medium text-muted">{label}</span>
      {children}
    </label>
  )
}

export function TextLink({ to, children, className = '' }: { to: string; children: ReactNode; className?: string }) {
  return (
    <a href={to} className={`text-accent hover:underline ${className}`}>
      {children}
    </a>
  )
}

// --- Navigation and loading --------------------------------------------------

interface TabsProps<T extends string> {
  label: string
  value: T
  tabs: readonly { value: T; label: string; count?: number }[]
  onChange: (value: T) => void
}

/** Underlined tabs for switching between views of one resource. */
export function Tabs<T extends string>({ label, value, tabs, onChange }: TabsProps<T>) {
  return (
    <div role="tablist" aria-label={label} className="mb-6 flex gap-1 overflow-x-auto border-b border-line">
      {tabs.map((t) => {
        const selected = t.value === value
        return (
          <button
            key={t.value}
            type="button"
            role="tab"
            aria-selected={selected}
            onClick={() => onChange(t.value)}
            className={`relative -mb-px flex cursor-pointer items-center gap-2 border-b-2 px-3 pt-1 pb-2.5 text-sm font-medium whitespace-nowrap transition-colors ${
              selected ? 'border-accent text-fg' : 'border-transparent text-fg-2 hover:text-fg'
            }`}
          >
            {t.label}
            {t.count != null && (
              <span className="rounded-full bg-subtle px-1.5 py-px text-xs text-fg-2 tabular-nums">{t.count}</span>
            )}
          </button>
        )
      })}
    </div>
  )
}

/** Grey placeholder bars shown while a list loads. */
export function Skeleton({ rows = 5 }: { rows?: number }) {
  return (
    <div aria-busy="true" aria-label="Loading" className="space-y-3 p-5">
      {Array.from({ length: rows }, (_, i) => (
        <div key={i} className="flex animate-pulse gap-4">
          <div className="h-4 w-1/4 rounded bg-subtle" />
          <div className="h-4 w-1/6 rounded bg-subtle" />
          <div className="h-4 flex-1 rounded bg-subtle" />
        </div>
      ))}
    </div>
  )
}

/** A friendly empty state with an optional call to action. */
export function EmptyState({ icon, title, children, action }: { icon: ReactNode; title: string; children?: ReactNode; action?: ReactNode }) {
  return (
    <div className="flex flex-col items-center px-6 py-14 text-center">
      <div className="mb-4 flex size-12 items-center justify-center rounded-full bg-subtle text-muted">{icon}</div>
      <h3 className="font-semibold">{title}</h3>
      {children && <p className="mt-1 max-w-md text-fg-2">{children}</p>}
      {action && <div className="mt-5">{action}</div>}
    </div>
  )
}
