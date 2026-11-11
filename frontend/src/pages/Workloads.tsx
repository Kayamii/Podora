import { useState } from 'react'
import { NamespaceFilter, SearchField } from '../components/NamespaceFilter'
import { NUM, ROW_LINK, STRETCHED_LINK, TD, TH } from '../components/styles'
import { Card, Empty, Notice, PageHeader, Segmented, Status, TableScroll } from '../components/ui'
import { usePolling } from '../hooks/usePolling'
import { href, navigate } from '../hooks/useRoute'
import { api } from '../services/api'
import { formatAge } from '../services/format'
import { plural, workloadStatus } from '../services/health'
import type { WorkloadKind } from '../types/api'

type KindFilter = 'all' | WorkloadKind

export function Workloads({ namespace }: { namespace: string }) {
  const { data, error, loading } = usePolling(api.workloads)
  const [kind, setKind] = useState<KindFilter>('all')
  const [search, setSearch] = useState('')

  const all = data ?? []
  const needle = search.trim().toLowerCase()
  const shown = all.filter(
    (w) =>
      (namespace === '' || w.namespace === namespace) &&
      (kind === 'all' || w.kind === kind) &&
      (needle === '' || w.name.toLowerCase().includes(needle)),
  )
  const unhealthy = shown.filter((w) => w.desired > 0 && !w.healthy).length

  return (
    <div>
      <PageHeader
        title="Workloads"
        subtitle={
          loading
            ? 'Loading…'
            : `${plural(shown.length, 'workload')}${unhealthy > 0 ? `, ${unhealthy} not fully ready` : ', all ready'}`
        }
      />

      <div className="mb-4 flex flex-wrap items-end gap-x-4 gap-y-3">
        <NamespaceFilter
          namespaces={[...new Set(all.map((w) => w.namespace))]}
          value={namespace}
          onChange={(ns) => navigate(href.workloads(ns))}
        />
        <SearchField value={search} onChange={setSearch} placeholder="Name" />
        <Segmented
          label="Kind"
          value={kind}
          onChange={setKind}
          options={[
            { value: 'all', label: 'All' },
            { value: 'Deployment', label: 'Deployments' },
            { value: 'StatefulSet', label: 'StatefulSets' },
            { value: 'DaemonSet', label: 'DaemonSets' },
          ]}
        />
      </div>

      {error && <Notice level="warning">Could not refresh the list: {error}</Notice>}

      <Card padded={false}>
        {!loading && shown.length === 0 ? (
          <Empty>{all.length === 0 ? 'No workloads in this cluster yet.' : 'No workloads match these filters.'}</Empty>
        ) : (
          <TableScroll>
            <thead>
              <tr>
                <th className={TH}>Name</th>
                <th className={TH}>Kind</th>
                <th className={TH}>Namespace</th>
                <th className={TH}>Status</th>
                <th className={`${TH} ${NUM}`}>Ready</th>
                <th className={TH}>Image</th>
                <th className={`${TH} ${NUM}`}>Age</th>
              </tr>
            </thead>
            <tbody>
              {shown.map((w) => {
                const status = workloadStatus(w)
                return (
                  <tr key={`${w.kind}/${w.namespace}/${w.name}`} className={ROW_LINK}>
                    <td className={TD}>
                      <a href={href.workload(w.kind, w.namespace, w.name)} className={STRETCHED_LINK}>
                        {w.name}
                      </a>
                    </td>
                    <td className={`${TD} text-fg-2`}>{w.kind}</td>
                    <td className={`${TD} text-fg-2`}>{w.namespace}</td>
                    <td className={TD}>
                      <Status level={status.level}>{status.label}</Status>
                    </td>
                    <td className={`${TD} ${NUM}`}>
                      {w.ready}/{w.desired}
                    </td>
                    <td className={`${TD} max-w-80 truncate font-mono text-[0.8125rem] text-fg-2`} title={w.images.join(', ')}>
                      {w.images[0]}
                      {w.images.length > 1 && <span className="font-sans text-muted"> +{w.images.length - 1}</span>}
                    </td>
                    <td className={`${TD} ${NUM} text-fg-2`}>{formatAge(w.createdAt)}</td>
                  </tr>
                )
              })}
            </tbody>
          </TableScroll>
        )}
      </Card>
    </div>
  )
}
