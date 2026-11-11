import { usageLevel } from '../services/health'

interface MeterProps {
  /** Accessible name, e.g. "CPU usage". */
  label: string
  /** 0–100. Values outside the range are clamped for drawing. */
  percent: number
  /** Exact figures shown on hover, e.g. "73m of 4 cores". */
  title?: string
  /** Colour by severity (usage against a limit) or a plain accent fill. */
  tone?: 'severity' | 'accent'
}

const FILL = { normal: 'bg-accent', neutral: 'bg-accent', warning: 'bg-warn', fault: 'bg-danger' }

/** A thin horizontal bar showing a share of a whole. */
export function Meter({ label, percent, title, tone = 'severity' }: MeterProps) {
  const clamped = Math.min(100, Math.max(0, percent))
  const fill = tone === 'accent' ? 'bg-accent' : FILL[usageLevel(clamped)]
  return (
    <div
      role="meter"
      aria-label={label}
      aria-valuemin={0}
      aria-valuemax={100}
      aria-valuenow={Math.round(clamped)}
      title={title}
      className="h-1.5 w-full overflow-hidden rounded-full bg-subtle"
    >
      <div className={`h-full rounded-full transition-[width] duration-500 ease-out ${fill}`} style={{ width: `${clamped}%` }} />
    </div>
  )
}
