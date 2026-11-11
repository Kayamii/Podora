import { useState } from 'react'
import { NamespaceFilter, SearchField } from '../components/NamespaceFilter'
import { NUM, ROW, TD, TH } from '../components/styles'
import { Card, Empty, Labels, Notice, PageHeader, Segmented, TableScroll, TextLink } from '../components/ui'
import { usePolling } from '../hooks/usePolling'
import { href, navigate } from '../hooks/useRoute'
import { api } from '../services/api'
import { formatAge } from '../services/format'
import { plural } from '../services/health'

type Tab = 'services' | 'ingresses'

export function Network({ namespace }: { namespace: string }) {
  const services = usePolling(api.services, 10_000)
  const ingresses = usePolling(api.ingresses, 10_000)
  const [tab, setTab] = useState<Tab>('services')
  const [search, setSearch] = useState('')

  const needle = search.trim().toLowerCase()
  const match = (r: { namespace: string; name: string }) =>
    (namespace === '' || r.namespace === namespace) && (needle === '' || r.name.toLowerCase().includes(needle))
  const svc = (services.data ?? []).filter(match)
  const ing = (ingresses.data ?? []).filter(match)
  const namespaces = [...new Set([...(services.data ?? []), ...(ingresses.data ?? [])].map((r) => r.namespace))]
  const error = services.error ?? ingresses.error

  return (
    <div>
      <PageHeader
        title="Services & Ingresses"
        subtitle={`${plural(svc.length, 'service')} · ${plural(ing.length, 'ingress', 'ingresses')}${namespace ? ` in ${namespace}` : ''}`}
      />

      <div className="mb-4 flex flex-wrap items-end gap-x-4 gap-y-3">
        <Segmented
          label="Resource type"
          value={tab}
          onChange={setTab}
          options={[
            { value: 'services', label: `Services (${svc.length})` },
            { value: 'ingresses', label: `Ingresses (${ing.length})` },
          ]}
        />
        <NamespaceFilter namespaces={namespaces} value={namespace} onChange={(ns) => navigate(href.network(ns))} />
        <SearchField value={search} onChange={setSearch} placeholder="Name" />
      </div>

      {error && <Notice level="warning">Could not refresh: {error}</Notice>}

      <Card padded={false}>
        {tab === 'services' ? (
          svc.length === 0 ? (
            <Empty>{services.loading ? 'Loading…' : 'No services match.'}</Empty>
          ) : (
            <TableScroll>
              <thead>
                <tr>
                  <th className={TH}>Name</th>
                  <th className={TH}>Namespace</th>
                  <th className={TH}>Type</th>
                  <th className={TH}>Cluster IP</th>
                  <th className={TH}>Ports</th>
                  <th className={TH}>External</th>
                  <th className={TH}>Selects pods</th>
                  <th className={`${TH} ${NUM}`}>Age</th>
                </tr>
              </thead>
              <tbody>
                {svc.map((s) => (
                  <tr key={`${s.namespace}/${s.name}`} className={ROW}>
                    <td className={`${TD} font-medium`}>{s.name}</td>
                    <td className={`${TD} text-fg-2`}>{s.namespace}</td>
                    <td className={TD}>
                      <span className="rounded bg-subtle px-1.5 py-0.5 text-sm">{s.type}</span>
                    </td>
                    <td className={`${TD} font-mono text-[0.8125rem] text-fg-2`}>{s.clusterIP || '—'}</td>
                    <td className={`${TD} font-mono text-[0.8125rem] whitespace-nowrap`}>
                      {s.ports.length === 0 ? '—' : s.ports.map((p) => <div key={p}>{p}</div>)}
                    </td>
                    <td className={`${TD} font-mono text-[0.8125rem] text-fg-2`}>{s.external.join(', ') || '—'}</td>
                    <td className={TD}>
                      {Object.keys(s.selector).length === 0 ? (
                        <span className="text-muted">—</span>
                      ) : (
                        <TextLink to={href.pods(s.namespace)} className="text-sm">
                          <Labels labels={s.selector} />
                        </TextLink>
                      )}
                    </td>
                    <td className={`${TD} ${NUM} text-fg-2`}>{formatAge(s.createdAt)}</td>
                  </tr>
                ))}
              </tbody>
            </TableScroll>
          )
        ) : ing.length === 0 ? (
          <Empty>{ingresses.loading ? 'Loading…' : 'No ingresses match. Ingresses expose services outside the cluster by host name.'}</Empty>
        ) : (
          <TableScroll>
            <thead>
              <tr>
                <th className={TH}>Name</th>
                <th className={TH}>Namespace</th>
                <th className={TH}>Class</th>
                <th className={TH}>Hosts</th>
                <th className={TH}>Address</th>
                <th className={TH}>TLS</th>
                <th className={`${TH} ${NUM}`}>Age</th>
              </tr>
            </thead>
            <tbody>
              {ing.map((i) => (
                <tr key={`${i.namespace}/${i.name}`} className={ROW}>
                  <td className={`${TD} font-medium`}>{i.name}</td>
                  <td className={`${TD} text-fg-2`}>{i.namespace}</td>
                  <td className={`${TD} text-fg-2`}>{i.class || 'Default'}</td>
                  <td className={`${TD} font-mono text-[0.8125rem]`}>{i.hosts.join(', ') || '*'}</td>
                  <td className={`${TD} font-mono text-[0.8125rem] text-fg-2`}>{i.addresses.join(', ') || 'Pending'}</td>
                  <td className={TD}>{i.tls ? 'Yes' : 'No'}</td>
                  <td className={`${TD} ${NUM} text-fg-2`}>{formatAge(i.createdAt)}</td>
                </tr>
              ))}
            </tbody>
          </TableScroll>
        )}
      </Card>
    </div>
  )
}
