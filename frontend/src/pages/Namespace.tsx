import { PodTable } from '../components/PodTable'
import { UsagePanel } from '../components/UsagePanel'
import { NUM, ROW_LINK, STRETCHED_LINK, TD, TH } from '../components/styles'
import { Badge, Card, Notice, PageHeader, Skeleton, Status, TableScroll, TextLink } from '../components/ui'
import { usePolling } from '../hooks/usePolling'
import { href } from '../hooks/useRoute'
import { api } from '../services/api'
import { formatBytes, formatCPU } from '../services/format'
import { plural, workloadStatus } from '../services/health'
import type { NamespaceUsage } from '../types/api'

/** One namespace: what it uses, what runs in it, and its heaviest pods. */
export function Namespace({ name }: { name: string }) {
  const usage = usePolling(api.namespacesUsage, 10_000)
  const pods = usePolling(api.pods)
  const workloads = usePolling(api.workloads)

  const ns = usage.data?.find((n) => n.name === name)
  const nsPods = (pods.data ?? []).filter((p) => p.namespace === name)
  const nsWorkloads = (workloads.data ?? []).filter((w) => w.namespace === name)
  const unhealthy = nsPods.filter((p) => !p.healthy).length

  if (usage.data && !ns) {
    return (
      <div>
        <PageHeader title={name} breadcrumb={[{ label: 'Namespaces', to: href.namespaces }]} />
        <Notice level="fault">This namespace does not exist.</Notice>
      </div>
    )
  }

  return (
    <div>
      <PageHeader
        title={name}
        breadcrumb={[{ label: 'Namespaces', to: href.namespaces }]}
        status={ns && <Badge level={ns.status === 'Active' ? 'normal' : 'warning'}>{ns.status}</Badge>}
        subtitle={
          ns
            ? `${plural(ns.pods, 'pod')}, ${unhealthy === 0 ? 'all healthy' : `${unhealthy} not healthy`} · ${plural(nsWorkloads.length, 'workload')}`
            : 'Loading…'
        }
        actions={
          <div className="flex flex-wrap gap-x-4 gap-y-1 self-center text-sm">
            <TextLink to={href.network(name)}>Services</TextLink>
            <TextLink to={href.events(name)}>Events</TextLink>
          </div>
        }
      />

      <UsagePanel namespace={name} cpuNote={ns && requestsNote(ns, 'cpu')} memoryNote={ns && requestsNote(ns, 'memory')} />

      <div className="mt-6 grid gap-6 xl:grid-cols-3">
        <Card title="Heaviest pods" aside="Sorted by CPU · click a header to sort" padded={false} className="xl:col-span-2">
          {pods.loading ? (
            <Skeleton rows={4} />
          ) : (
            <PodTable pods={nsPods} empty="No pods in this namespace." initialSort="cpu" />
          )}
        </Card>

        <Card title="Workloads" padded={false}>
          {workloads.loading ? (
            <Skeleton rows={3} />
          ) : nsWorkloads.length === 0 ? (
            <p className="px-5 py-8 text-center text-fg-2">No workloads in this namespace.</p>
          ) : (
            <TableScroll>
              <thead>
                <tr>
                  <th className={TH}>Name</th>
                  <th className={TH}>Status</th>
                  <th className={`${TH} ${NUM}`}>Ready</th>
                </tr>
              </thead>
              <tbody>
                {nsWorkloads.map((w) => {
                  const s = workloadStatus(w)
                  return (
                    <tr key={`${w.kind}/${w.name}`} className={ROW_LINK}>
                      <td className={TD}>
                        <a href={href.workload(w.kind, w.namespace, w.name)} className={STRETCHED_LINK}>
                          {w.name}
                        </a>
                        <div className="text-sm text-muted">{w.kind}</div>
                      </td>
                      <td className={TD}>
                        <Status level={s.level}>{s.label}</Status>
                      </td>
                      <td className={`${TD} ${NUM}`}>
                        {w.ready}/{w.desired}
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </TableScroll>
          )}
        </Card>
      </div>
    </div>
  )
}

function requestsNote(ns: NamespaceUsage, kind: 'cpu' | 'memory'): string {
  const fmt = kind === 'cpu' ? formatCPU : formatBytes
  const req = kind === 'cpu' ? ns.cpuRequests : ns.memoryRequests
  const lim = kind === 'cpu' ? ns.cpuLimits : ns.memoryLimits
  return `Requested ${req > 0 ? fmt(req) : 'nothing'} · limits ${lim > 0 ? fmt(lim) : 'none set'}`
}
