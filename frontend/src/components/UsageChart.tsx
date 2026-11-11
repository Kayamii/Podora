import { useLayoutEffect, useMemo, useRef, useState } from 'react'
import type { PointerEvent } from 'react'

export interface ChartPoint {
  /** Milliseconds since epoch. */
  t: number
  /** The plotted value: a percentage, or an amount such as millicores. */
  value: number
  /** The same reading in real units, for the tooltip, e.g. "84m". */
  label: string
}

interface Props {
  /** Names the series for assistive tech, e.g. "CPU usage". */
  name: string
  points: ChartPoint[]
  /** CSS colour of the line. */
  color: string
  /** A gap longer than this between samples breaks the line. */
  maxGapMs: number
  /**
   * How to label the y-axis. Without it, values are percentages on a
   * 0–100 scale; with it, the axis fits the data.
   */
  formatAxis?: (value: number) => string
  height?: number
}

const PAD = { top: 8, right: 52, bottom: 22, left: 0 }
/** Candidate tops for the y-axis; the smallest one that leaves headroom wins. */
const SCALES = [5, 10, 25, 50, 100]

/** Rounds up to 1, 2, 2.5 or 5 times a power of ten, for tidy axis labels. */
function niceCeil(v: number): number {
  if (v <= 0) return 1
  const power = 10 ** Math.floor(Math.log10(v))
  return ([1, 2, 2.5, 5, 10].find((m) => m * power >= v) ?? 10) * power
}

function clock(t: number): string {
  return new Date(t).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
}

/** A single-series area chart of usage over time, with a hover readout. */
export function UsageChart({ name, points, color, maxGapMs, formatAxis, height = 140 }: Props) {
  const HEIGHT = height
  const percent = formatAxis === undefined
  const axis = formatAxis ?? ((v: number) => `${v}%`)
  const box = useRef<HTMLDivElement>(null)
  const [width, setWidth] = useState(0)
  const [hover, setHover] = useState<number | null>(null)

  useLayoutEffect(() => {
    const el = box.current
    if (!el) return
    const observer = new ResizeObserver(([entry]) => setWidth(entry.contentRect.width))
    observer.observe(el)
    return () => observer.disconnect()
  }, [])

  const geometry = useMemo(() => {
    if (points.length < 2 || width === 0) return null
    const t0 = points[0].t
    const t1 = points[points.length - 1].t
    const peak = Math.max(...points.map((p) => p.value))
    const top = percent ? (SCALES.find((s) => s >= peak * 1.15) ?? 100) : niceCeil(peak * 1.15)

    const innerW = width - PAD.left - PAD.right
    const innerH = HEIGHT - PAD.top - PAD.bottom
    const x = (t: number) => PAD.left + ((t - t0) / (t1 - t0)) * innerW
    const y = (v: number) => PAD.top + (1 - Math.min(v, top) / top) * innerH
    const baseline = y(0)

    // Split into runs wherever sampling stopped, so a gap is drawn as a gap.
    const runs: ChartPoint[][] = [[points[0]]]
    for (let i = 1; i < points.length; i++) {
      if (points[i].t - points[i - 1].t > maxGapMs) runs.push([])
      runs[runs.length - 1].push(points[i])
    }

    const lines: string[] = []
    const areas: string[] = []
    for (const run of runs) {
      const path = run.map((p, i) => `${i === 0 ? 'M' : 'L'}${x(p.t).toFixed(1)},${y(p.value).toFixed(1)}`).join(' ')
      lines.push(path)
      const first = x(run[0].t).toFixed(1)
      const last = x(run[run.length - 1].t).toFixed(1)
      areas.push(`${path} L${last},${baseline} L${first},${baseline} Z`)
    }
    return { t0, top, x, y, baseline, lines, areas }
  }, [points, width, maxGapMs, percent, HEIGHT])

  function onMove(e: PointerEvent<SVGSVGElement>) {
    if (!geometry) return
    const px = e.clientX - e.currentTarget.getBoundingClientRect().left
    let nearest = 0
    let best = Infinity
    points.forEach((p, i) => {
      const d = Math.abs(geometry.x(p.t) - px)
      if (d < best) {
        best = d
        nearest = i
      }
    })
    setHover(nearest)
  }

  const hovered = hover != null && geometry ? points[Math.min(hover, points.length - 1)] : null
  const summary =
    points.length > 0
      ? `${name} from ${clock(points[0].t)} to ${clock(points[points.length - 1].t)}: now ${points[points.length - 1].label}.`
      : `${name}: no readings yet.`

  return (
    <div ref={box} className="relative" style={{ height: HEIGHT }}>
      <p className="sr-only">{summary}</p>

      {!geometry ? (
        <p className="flex h-full items-center justify-center text-sm text-muted">
          Collecting data. The chart appears after a few readings.
        </p>
      ) : (
        <>
          <svg
            width={width}
            height={HEIGHT}
            aria-hidden="true"
            className="block touch-none"
            onPointerMove={onMove}
            onPointerLeave={() => setHover(null)}
          >
            {[0, 0.5, 1].map((f) => {
              const yy = geometry.y(geometry.top * f)
              return (
                <g key={f}>
                  <line x1={PAD.left} x2={width - PAD.right} y1={yy} y2={yy} stroke="var(--line)" strokeWidth={1} />
                  <text x={width - PAD.right + 6} y={yy + 4} fontSize={11} fill="var(--muted)">
                    {axis(geometry.top * f)}
                  </text>
                </g>
              )
            })}

            {geometry.areas.map((d, i) => (
              <path key={i} d={d} fill={color} opacity={0.1} />
            ))}
            {geometry.lines.map((d, i) => (
              <path key={i} d={d} fill="none" stroke={color} strokeWidth={2} strokeLinejoin="round" strokeLinecap="round" />
            ))}

            {hovered && (
              <g>
                <line
                  x1={geometry.x(hovered.t)}
                  x2={geometry.x(hovered.t)}
                  y1={PAD.top}
                  y2={geometry.baseline}
                  stroke="var(--line-strong)"
                  strokeWidth={1}
                />
                <circle
                  cx={geometry.x(hovered.t)}
                  cy={geometry.y(hovered.value)}
                  r={4}
                  fill={color}
                  stroke="var(--surface)"
                  strokeWidth={2}
                />
              </g>
            )}

            <text x={PAD.left} y={HEIGHT - 4} fontSize={12} fill="var(--muted)">
              {clock(geometry.t0)}
            </text>
            <text x={width - PAD.right} y={HEIGHT - 4} fontSize={12} fill="var(--muted)" textAnchor="end">
              Now
            </text>
          </svg>

          {hovered && (
            <div
              className="pointer-events-none absolute top-0 z-10 rounded-md border border-line bg-surface px-2.5 py-1.5 text-sm whitespace-nowrap shadow-lg"
              style={
                geometry.x(hovered.t) > width / 2
                  ? { right: width - geometry.x(hovered.t) + 10 }
                  : { left: geometry.x(hovered.t) + 10 }
              }
            >
              <span className="font-semibold tabular-nums">{hovered.label}</span>
              <span className="ml-2 text-muted tabular-nums">{new Date(hovered.t).toLocaleTimeString()}</span>
            </div>
          )}
        </>
      )}
    </div>
  )
}
