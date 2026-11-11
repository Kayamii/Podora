import { usePolling } from '../hooks/usePolling'
import { api } from '../services/api'
import { formatBytes, formatCPU } from '../services/format'
import { UsageChart } from './UsageChart'
import type { ChartPoint } from './UsageChart'
import { Card } from './ui'

interface Props {
  namespace: string
  /** One pod in the namespace; omit for the whole namespace. */
  pod?: string
  /** Shown under each current value, e.g. "Requested 200m · limit 1 core". */
  cpuNote?: string
  memoryNote?: string
}

/** CPU and memory over the last hour, for a namespace or a single pod. */
export function UsagePanel({ namespace, pod = '', cpuNote, memoryNote }: Props) {
  const { data } = usePolling((signal) => api.usageHistory(namespace, pod, signal), 10_000)
  const points = data?.points ?? []
  const maxGapMs = (data?.intervalSeconds ?? 10) * 3000
  const last = points.at(-1)

  const cpu: ChartPoint[] = points.map((p) => ({ t: new Date(p.t).getTime(), value: p.cpu, label: formatCPU(p.cpu) }))
  const memory: ChartPoint[] = points.map((p) => ({ t: new Date(p.t).getTime(), value: p.memory, label: formatBytes(p.memory) }))

  return (
    <div className="grid gap-6 xl:grid-cols-2">
      <Card title="CPU" aside="Last hour">
        <Reading value={last ? formatCPU(last.cpu) : '—'} note={cpuNote} />
        <UsageChart name="CPU usage" points={cpu} color="var(--accent)" maxGapMs={maxGapMs} formatAxis={formatCPU} height={120} />
      </Card>
      <Card title="Memory" aside="Last hour">
        <Reading value={last ? formatBytes(last.memory) : '—'} note={memoryNote} />
        <UsageChart
          name="Memory usage"
          points={memory}
          color="var(--chart-2)"
          maxGapMs={maxGapMs}
          formatAxis={formatBytes}
          height={120}
        />
      </Card>
    </div>
  )
}

function Reading({ value, note }: { value: string; note?: string }) {
  return (
    <div className="mb-3">
      <p className="text-2xl font-semibold tracking-tight tabular-nums">{value}</p>
      {note && <p className="text-sm text-muted">{note}</p>}
    </div>
  )
}
