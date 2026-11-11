import { Meter } from '../components/Meter'
import { NUM, ROW_LINK, STRETCHED_LINK, TD, TH } from '../components/styles'
import { Card, Notice, PageHeader, Skeleton, Status, TableScroll } from '../components/ui'
import { usePolling } from '../hooks/usePolling'
import { href } from '../hooks/useRoute'
import { api } from '../services/api'
import { formatAge, formatBytes, formatCPU, percent } from '../services/format'
import { plural } from '../services/health'
import type { NamespaceUsage, Overview } from '../types/api'

/** Every namespace with what it is using, against the whole cluster. */
export function Namespaces({ overview }: { overview: Overview | null }) {
  const { data, error, loading } = usePolling(api.namespacesUsage, 10_000)
  // Bars compare against the cluster when it is known (users with access to
  // every namespace), otherwise against the busiest visible namespace.
  const busiest = (pick: (n: NamespaceUsage) => number) => Math.max(1, ...(data ?? []).map(pick))
  const cpuTotal = overview?.resources.cpu.allocatable ?? busiest((n) => n.cpuUsage ?? n.cpuRequests)
  const memTotal = overview?.resources.memory.allocatable ?? busiest((n) => n.memoryUsage ?? n.memoryRequests)
  // Heaviest first: the question here is usually "who is using the cluster?"
  const rows = [...(data ?? [])].sort((a, b) => (b.memoryUsage ?? b.memoryRequests) - (a.memoryUsage ?? a.memoryRequests))

  return (
    <div>
      <PageHeader
        title="Namespaces"
        subtitle={
          overview
            ? `${plural(rows.length, 'namespace')} · share of the cluster's ${formatCPU(cpuTotal)} and ${formatBytes(memTotal)}`
            : `${plural(rows.length, 'namespace')} you have access to`
        }
      />
      {error && <Notice level="warning">Could not refresh: {error}</Notice>}
      <Card padded={false}>
        {loading ? (
          <Skeleton />
        ) : (
          <TableScroll>
            <thead>
              <tr>
                <th className={TH}>Name</th>
                <th className={TH}>Status</th>
                <th className={`${TH} ${NUM}`}>Pods</th>
                <th className={`${TH} min-w-52`}>CPU</th>
                <th className={`${TH} min-w-52`}>Memory</th>
                <th className={`${TH} ${NUM}`}>Age</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((ns) => (
                <tr key={ns.name} className={ROW_LINK}>
                  <td className={TD}>
                    <a href={href.namespace(ns.name)} className={STRETCHED_LINK}>
                      {ns.name}
                    </a>
                  </td>
                  <td className={TD}>
                    <Status level={ns.status === 'Active' ? 'normal' : 'warning'}>{ns.status}</Status>
                  </td>
                  <td className={`${TD} ${NUM}`}>
                    {ns.runningPods}
                    <span className="text-muted"> / {ns.pods}</span>
                  </td>
                  <td className={TD}>
                    <Share used={ns.cpuUsage} requested={ns.cpuRequests} total={cpuTotal} format={formatCPU} name={`${ns.name} CPU`} />
                  </td>
                  <td className={TD}>
                    <Share used={ns.memoryUsage} requested={ns.memoryRequests} total={memTotal} format={formatBytes} name={`${ns.name} memory`} />
                  </td>
                  <td className={`${TD} ${NUM} text-fg-2`}>{formatAge(ns.createdAt)}</td>
                </tr>
              ))}
            </tbody>
          </TableScroll>
        )}
      </Card>
    </div>
  )
}

interface ShareProps {
  used: number | null
  requested: number
  total: number
  format: (n: number) => string
  name: string
}

/** Usage (or requests, without metrics) as a share of the cluster. */
function Share({ used, requested, total, format, name }: ShareProps) {
  const value = used ?? requested
  const pct = percent(value, total)
  return (
    <div>
      <div className="mb-1.5 flex items-baseline justify-between gap-3 text-sm tabular-nums">
        <span className="font-medium">{format(value)}</span>
        <span className="text-muted">
          {pct < 1 && value > 0 ? '<1' : Math.round(pct)}% {used == null && 'requested'}
        </span>
      </div>
      <Meter tone="accent" label={`${name} share of cluster`} percent={pct} title={`Requested ${format(requested)}`} />
    </div>
  )
}
