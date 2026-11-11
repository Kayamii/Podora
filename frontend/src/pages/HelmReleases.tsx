import { useState } from 'react'
import { Package, Plus } from 'lucide-react'
import { ChartIcon } from '../components/ChartSource'
import { NamespaceFilter, SearchField } from '../components/NamespaceFilter'
import { NUM, ROW_LINK, STRETCHED_LINK, TD, TH } from '../components/styles'
import { Button, Card, EmptyState, Notice, PageHeader, Skeleton, Status, TableScroll } from '../components/ui'
import { usePolling } from '../hooks/usePolling'
import { useSession } from '../hooks/session'
import { href, navigate } from '../hooks/useRoute'
import { helm } from '../services/api'
import { formatAge } from '../services/format'
import { plural, releaseLevel } from '../services/health'

export function HelmReleases() {
  const { can } = useSession()
  const { data, error, loading } = usePolling(helm.releases)
  const [namespace, setNamespace] = useState('')
  const [search, setSearch] = useState('')

  const all = data ?? []
  const needle = search.trim().toLowerCase()
  const shown = all.filter(
    (r) =>
      (namespace === '' || r.namespace === namespace) &&
      (needle === '' || r.name.toLowerCase().includes(needle) || r.chart.toLowerCase().includes(needle)),
  )
  // Helm keeps releases in Secrets; a read-only Podora install cannot see them.
  const disabled = error?.startsWith('Helm management is not enabled') ?? false
  const failing = shown.filter((r) => releaseLevel(r.status) === 'fault').length
  const install = can('helm.manage') && !disabled ? (
    <Button variant="primary" icon={<Plus size={16} />} onClick={() => navigate(href.helmInstall)}>
      Install chart
    </Button>
  ) : undefined

  return (
    <div>
      <PageHeader
        title="Helm releases"
        subtitle={
          disabled
            ? 'Not enabled for this installation'
            : loading
              ? 'Loading…'
              : `${plural(shown.length, 'release')}${failing > 0 ? ` · ${failing} failed` : ''}`
        }
        actions={install}
      />

      {all.length > 0 && (
        <div className="mb-4 flex flex-wrap items-end gap-x-4 gap-y-3">
          <NamespaceFilter namespaces={[...new Set(all.map((r) => r.namespace))]} value={namespace} onChange={setNamespace} />
          <SearchField value={search} onChange={setSearch} placeholder="Release or chart" />
        </div>
      )}

      {error && !disabled && <Notice level="warning">Could not refresh: {error}</Notice>}

      <Card padded={false}>
        {disabled ? (
          <EmptyState icon={<Package size={22} />} title="Helm management is not enabled">
            Helm keeps releases in Secrets, and this installation of Podora runs with read-only access that excludes Secrets. To
            list and manage Helm releases, upgrade Podora's chart with <code className="font-mono text-sm">rbac.helm=true</code>.
          </EmptyState>
        ) : loading ? (
          <Skeleton />
        ) : all.length === 0 ? (
          <EmptyState icon={<Package size={22} />} title="No Helm releases yet" action={install}>
            {install
              ? 'Install an application from a chart: upload a .tgz, or get one from a chart repository or OCI registry. You can review and edit its values before anything is deployed.'
              : 'No applications have been installed with Helm in the namespaces you can see.'}
          </EmptyState>
        ) : shown.length === 0 ? (
          <p className="px-5 py-8 text-center text-fg-2">No releases match these filters.</p>
        ) : (
          <TableScroll>
            <thead>
              <tr>
                <th className={TH}>Release</th>
                <th className={TH}>Namespace</th>
                <th className={TH}>Chart</th>
                <th className={TH}>App version</th>
                <th className={TH}>Status</th>
                <th className={`${TH} ${NUM}`}>Revision</th>
                <th className={`${TH} ${NUM}`}>Updated</th>
              </tr>
            </thead>
            <tbody>
              {shown.map((r) => (
                <tr key={`${r.namespace}/${r.name}`} className={ROW_LINK}>
                  <td className={TD}>
                    <span className="flex items-center gap-3">
                      <ChartIcon src={r.icon} size={28} />
                      <a href={href.helmRelease(r.namespace, r.name)} className={STRETCHED_LINK}>
                        {r.name}
                      </a>
                    </span>
                  </td>
                  <td className={`${TD} text-fg-2`}>{r.namespace}</td>
                  <td className={`${TD} whitespace-nowrap`}>
                    {r.chart} <span className="text-muted">{r.chartVersion}</span>
                  </td>
                  <td className={`${TD} text-fg-2`}>{r.appVersion || '—'}</td>
                  <td className={TD}>
                    <Status level={releaseLevel(r.status)}>{r.status}</Status>
                  </td>
                  <td className={`${TD} ${NUM}`}>{r.revision}</td>
                  <td className={`${TD} ${NUM} text-fg-2`}>{formatAge(r.updated)} ago</td>
                </tr>
              ))}
            </tbody>
          </TableScroll>
        )}
      </Card>
    </div>
  )
}
