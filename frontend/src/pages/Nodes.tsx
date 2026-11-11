import { Meter } from '../components/Meter'
import { NUM, ROW, TD, TH } from '../components/styles'
import { Card, PageHeader, Status, TableScroll } from '../components/ui'
import { formatAge, formatBytes, formatCPU, percent } from '../services/format'
import { plural } from '../services/health'
import type { NodeUsage, Overview } from '../types/api'

export function Nodes({ overview }: { overview: Overview }) {
  const { nodes, cluster } = overview
  return (
    <div>
      <PageHeader title="Nodes" subtitle={`${cluster.nodesReady} of ${plural(cluster.nodesTotal, 'node')} ready`} />
      <Card padded={false}>
        <TableScroll>
          <thead>
            <tr>
              <th className={TH}>Name</th>
              <th className={TH}>Status</th>
              <th className={TH}>Roles</th>
              <th className={`${TH} min-w-44`}>CPU</th>
              <th className={`${TH} min-w-44`}>Memory</th>
              <th className={`${TH} ${NUM}`}>Pods</th>
              <th className={TH}>Version</th>
              <th className={`${TH} ${NUM}`}>Age</th>
            </tr>
          </thead>
          <tbody>
            {nodes.map((n) => (
              <tr key={n.name} className={ROW}>
                <td className={TD}>
                  <div className="font-medium">{n.name}</div>
                  <div className="text-sm text-muted">
                    {n.internalIP} · {n.osImage}
                  </div>
                </td>
                <td className={TD}>
                  <Status level={!n.ready ? 'fault' : n.schedulable ? 'normal' : 'warning'}>
                    {!n.ready ? 'Not ready' : n.schedulable ? 'Ready' : 'Cordoned'}
                  </Status>
                </td>
                <td className={`${TD} text-fg-2`}>{n.roles.length > 0 ? n.roles.join(', ') : 'worker'}</td>
                <td className={TD}>
                  <Usage name={`${n.name} CPU`} u={n.cpu} format={formatCPU} />
                </td>
                <td className={TD}>
                  <Usage name={`${n.name} memory`} u={n.memory} format={formatBytes} />
                </td>
                <td className={`${TD} ${NUM} whitespace-nowrap`}>
                  {n.pods} <span className="text-muted">/ {n.podCapacity}</span>
                </td>
                <td className={`${TD} whitespace-nowrap text-fg-2`}>{n.kubeletVersion}</td>
                <td className={`${TD} ${NUM} text-fg-2`}>{formatAge(n.createdAt)}</td>
              </tr>
            ))}
          </tbody>
        </TableScroll>
      </Card>
    </div>
  )
}

function Usage({ name, u, format }: { name: string; u: NodeUsage; format: (n: number) => string }) {
  if (u.usage == null) return <span className="text-fg-2">{format(u.allocatable)} allocatable</span>
  const pct = percent(u.usage, u.allocatable)
  return (
    <div>
      <div className="mb-1.5 flex items-baseline justify-between gap-3 text-sm tabular-nums">
        <span className="font-medium">{Math.round(pct)}%</span>
        <span className="whitespace-nowrap text-muted">
          {format(u.usage)} / {format(u.allocatable)}
        </span>
      </div>
      <Meter label={`${name} usage`} percent={pct} title={`${format(u.usage)} used of ${format(u.allocatable)}`} />
    </div>
  )
}
