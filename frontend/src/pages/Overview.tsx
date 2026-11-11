import { useMemo } from 'react'
import { AlertTriangle, ArrowRight, CheckCircle2, XCircle } from 'lucide-react'
import { EventList } from '../components/EventList'
import { Meter } from '../components/Meter'
import { UsageChart } from '../components/UsageChart'
import type { ChartPoint } from '../components/UsageChart'
import { Card, PageHeader, TextLink } from '../components/ui'
import { href } from '../hooks/useRoute'
import { formatBytes, formatCPU, percent } from '../services/format'
import { assess, plural, usageLevel } from '../services/health'
import type { Assessment } from '../services/health'
import type { HistoryResponse, Measurement, Overview as OverviewData } from '../types/api'

interface Props {
  overview: OverviewData
  history: HistoryResponse | null
}

export function Overview({ overview, history }: Props) {
  const assessment = useMemo(() => assess(overview), [overview])
  const { cluster, pods, workloads, network, resources } = overview
  const wl = [workloads.deployments, workloads.statefulSets, workloads.daemonSets]
  const wlTotal = wl.reduce((n, c) => n + c.total, 0)
  const wlReady = wl.reduce((n, c) => n + c.ready, 0)

  const samples = history?.samples ?? []
  const maxGapMs = (history?.intervalSeconds ?? 10) * 3000
  const cpuPoints: ChartPoint[] = samples.map((s) => ({
    t: new Date(s.t).getTime(),
    value: percent(s.cpuUsage, s.cpuAllocatable),
    label: `${Math.round(percent(s.cpuUsage, s.cpuAllocatable))}% · ${formatCPU(s.cpuUsage)}`,
  }))
  const memPoints: ChartPoint[] = samples.map((s) => ({
    t: new Date(s.t).getTime(),
    value: percent(s.memoryUsage, s.memoryAllocatable),
    label: `${Math.round(percent(s.memoryUsage, s.memoryAllocatable))}% · ${formatBytes(s.memoryUsage)}`,
  }))

  return (
    <div>
      <PageHeader
        title="Cluster overview"
        subtitle={`Kubernetes ${cluster.version} · ${plural(cluster.nodesTotal, 'node')} · ${cluster.platform}`}
      />

      <HealthSummary assessment={assessment} />

      <div className="mt-6 grid grid-cols-2 gap-4 xl:grid-cols-4">
        <Stat label="Nodes" value={`${cluster.nodesReady}/${cluster.nodesTotal}`} detail="ready" to={href.nodes} />
        <Stat
          label="Pods"
          value={String(pods.running)}
          detail={`running of ${pods.total}${pods.restarts > 0 ? ` · ${plural(pods.restarts, 'restart')}` : ''}`}
          to={href.pods()}
        />
        <Stat label="Workloads" value={`${wlReady}/${wlTotal}`} detail="ready" to={href.workloads()} />
        <Stat
          label="Services"
          value={String(network.services)}
          detail={plural(network.ingresses, 'ingress', 'ingresses')}
          to={href.network()}
        />
      </div>

      <div className="mt-6 grid gap-6 xl:grid-cols-2">
        <ResourceCard
          title="CPU"
          m={resources.cpu}
          format={formatCPU}
          points={cpuPoints}
          color="var(--accent)"
          maxGapMs={maxGapMs}
          metrics={resources.metricsAvailable}
        />
        <ResourceCard
          title="Memory"
          m={resources.memory}
          format={formatBytes}
          points={memPoints}
          color="var(--chart-2)"
          maxGapMs={maxGapMs}
          metrics={resources.metricsAvailable}
        />
      </div>

      <div className="mt-6 grid gap-6 xl:grid-cols-3">
        <Card title="Pods by phase" aside={<TextLink to={href.pods()}>View pods</TextLink>}>
          <PodPhases overview={overview} />
        </Card>
        <Card
          title="Recent events"
          aside={<TextLink to={href.events()}>View all</TextLink>}
          padded={false}
          className="xl:col-span-2"
        >
          {overview.events.length === 0 ? (
            <p className="px-5 py-8 text-center text-fg-2">The cluster has reported no events.</p>
          ) : (
            <EventList events={overview.events.slice(0, 6)} now={new Date(overview.generatedAt).getTime()} />
          )}
        </Card>
      </div>
    </div>
  )
}

const VERDICT = {
  normal: { icon: CheckCircle2, color: 'text-ok', title: 'All systems normal' },
  neutral: { icon: CheckCircle2, color: 'text-ok', title: 'All systems normal' },
  warning: { icon: AlertTriangle, color: 'text-warn', title: 'Needs attention' },
  fault: { icon: XCircle, color: 'text-danger', title: 'Problems detected' },
}

function HealthSummary({ assessment }: { assessment: Assessment }) {
  const { level, issues } = assessment
  const v = VERDICT[level]
  const Icon = v.icon
  return (
    <Card padded={false}>
      <div className="flex flex-col gap-4 p-5 sm:flex-row sm:items-start">
        <div className="flex items-center gap-3 sm:w-72 sm:shrink-0">
          <Icon size={28} strokeWidth={2} className={v.color} aria-hidden="true" />
          <div>
            <h2 className="text-lg font-semibold">{v.title}</h2>
            <p className="text-sm text-fg-2">
              {issues.length === 0 ? 'Nothing needs your attention.' : plural(issues.length, 'issue')}
            </p>
          </div>
        </div>
        {issues.length > 0 && (
          <ul className="grid flex-1 gap-2 sm:grid-cols-2">
            {issues.map((issue) => (
              <li key={issue.text}>
                <a
                  href={issue.to}
                  className="group flex items-center justify-between gap-3 rounded-md border border-line px-3 py-2 transition-colors hover:border-line-strong hover:bg-subtle"
                >
                  <span className="flex items-center gap-2">
                    <span
                      aria-hidden="true"
                      className={`size-2 shrink-0 rounded-full ${issue.level === 'fault' ? 'bg-danger' : 'bg-warn'}`}
                    />
                    {issue.text}
                  </span>
                  <ArrowRight size={15} className="shrink-0 text-muted group-hover:text-fg" aria-hidden="true" />
                </a>
              </li>
            ))}
          </ul>
        )}
      </div>
    </Card>
  )
}

function Stat({ label, value, detail, to }: { label: string; value: string; detail: string; to: string }) {
  return (
    <a
      href={to}
      className="group rounded-lg border border-line bg-surface p-5 shadow-card transition-colors hover:border-line-strong"
    >
      <div className="flex items-center justify-between text-sm font-medium text-fg-2">
        {label}
        <ArrowRight size={15} className="text-muted opacity-0 transition-opacity group-hover:opacity-100" aria-hidden="true" />
      </div>
      <div className="mt-2 text-3xl font-semibold tracking-tight tabular-nums">{value}</div>
      <div className="mt-0.5 text-sm text-muted">{detail}</div>
    </a>
  )
}

interface ResourceCardProps {
  title: string
  m: Measurement
  format: (n: number) => string
  points: ChartPoint[]
  color: string
  maxGapMs: number
  metrics: boolean
}

function ResourceCard({ title, m, format, points, color, maxGapMs, metrics }: ResourceCardProps) {
  const requestedPct = Math.round(percent(m.requests, m.allocatable))
  if (!metrics || m.usage == null) {
    return (
      <Card title={title}>
        <p className="text-3xl font-semibold tabular-nums">{requestedPct}%</p>
        <p className="mb-3 text-sm text-muted">
          requested · {format(m.requests)} of {format(m.allocatable)}
        </p>
        <Meter label={`${title} requested`} percent={requestedPct} />
        <p className="mt-4 text-sm text-fg-2">
          Live usage needs metrics-server, which is not responding. Showing what workloads have requested instead.
        </p>
      </Card>
    )
  }
  const pct = percent(m.usage, m.allocatable)
  const level = usageLevel(pct)
  return (
    <Card title={title} aside="Last hour">
      <div className="mb-4 flex flex-wrap items-end justify-between gap-x-6 gap-y-2">
        <div>
          <p className={`text-3xl font-semibold tabular-nums ${level === 'fault' ? 'text-danger-fg' : level === 'warning' ? 'text-warn-fg' : ''}`}>
            {Math.round(pct)}%
          </p>
          <p className="text-sm text-muted">
            {format(m.usage)} of {format(m.allocatable)} in use
          </p>
        </div>
        <dl className="flex gap-6 text-sm">
          <div>
            <dt className="text-muted">Requested</dt>
            <dd className="font-medium tabular-nums">
              {format(m.requests)} <span className="font-normal text-muted">({requestedPct}%)</span>
            </dd>
          </div>
          <div>
            <dt className="text-muted">Limits</dt>
            <dd className="font-medium tabular-nums">{m.limits > 0 ? format(m.limits) : 'None set'}</dd>
          </div>
        </dl>
      </div>
      <UsageChart name={`${title} usage`} points={points} color={color} maxGapMs={maxGapMs} />
    </Card>
  )
}

function PodPhases({ overview }: { overview: OverviewData }) {
  const { pods } = overview
  const phases = [
    { label: 'Running', count: pods.running, color: 'bg-ok' },
    { label: 'Pending', count: pods.pending, color: 'bg-warn' },
    { label: 'Failed', count: pods.failed, color: 'bg-danger' },
    { label: 'Completed', count: pods.succeeded, color: 'bg-muted' },
    { label: 'Unknown', count: pods.unknown, color: 'bg-line-strong' },
  ]
  if (pods.total === 0) return <p className="text-fg-2">No pods are running yet.</p>
  return (
    <div>
      <div
        className="flex h-2.5 gap-0.5 overflow-hidden rounded-full"
        role="img"
        aria-label={phases.map((p) => `${p.count} ${p.label.toLowerCase()}`).join(', ')}
      >
        {phases
          .filter((p) => p.count > 0)
          .map((p) => (
            <div key={p.label} className={p.color} style={{ flexGrow: p.count, flexBasis: 0 }} title={`${p.count} ${p.label}`} />
          ))}
      </div>
      <dl className="mt-4">
        {phases.map((p) => (
          <div key={p.label} className="flex items-center justify-between border-t border-line py-2 first:border-t-0">
            <dt className="flex items-center gap-2 text-fg-2">
              <span aria-hidden="true" className={`size-2 rounded-full ${p.count > 0 ? p.color : 'bg-subtle'}`} />
              {p.label}
            </dt>
            <dd className="font-medium tabular-nums">{p.count}</dd>
          </div>
        ))}
      </dl>
    </div>
  )
}
