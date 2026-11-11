import { useState } from 'react'
import { EventList } from '../components/EventList'
import { NamespaceFilter, SearchField } from '../components/NamespaceFilter'
import { Card, Empty, Notice, PageHeader, Segmented } from '../components/ui'
import { usePolling } from '../hooks/usePolling'
import { href, navigate } from '../hooks/useRoute'
import { api } from '../services/api'
import { plural } from '../services/health'

type TypeFilter = 'all' | 'Warning' | 'Normal'

export function Events({ namespace }: { namespace: string }) {
  const { data, error, loading } = usePolling(api.events)
  const [type, setType] = useState<TypeFilter>('all')
  const [search, setSearch] = useState('')

  const all = data ?? []
  const needle = search.trim().toLowerCase()
  const shown = all.filter(
    (e) =>
      (namespace === '' || e.namespace === namespace) &&
      (type === 'all' || e.type === type) &&
      (needle === '' || `${e.reason} ${e.object} ${e.message}`.toLowerCase().includes(needle)),
  )
  const warnings = shown.filter((e) => e.type === 'Warning').length

  return (
    <div>
      <PageHeader
        title="Events"
        subtitle={loading ? 'Loading…' : `${plural(shown.length, 'event')} · ${plural(warnings, 'warning')} · the cluster keeps about an hour`}
      />
      <div className="mb-4 flex flex-wrap items-end gap-x-4 gap-y-3">
        <NamespaceFilter
          namespaces={[...new Set(all.map((e) => e.namespace).filter(Boolean))]}
          value={namespace}
          onChange={(ns) => navigate(href.events(ns))}
        />
        <SearchField value={search} onChange={setSearch} placeholder="Reason, object or message" />
        <Segmented
          label="Event type"
          value={type}
          onChange={setType}
          options={[
            { value: 'all', label: 'All' },
            { value: 'Warning', label: 'Warnings' },
            { value: 'Normal', label: 'Normal' },
          ]}
        />
      </div>
      {error && <Notice level="warning">Could not refresh: {error}</Notice>}
      <Card padded={false}>
        {shown.length === 0 ? <Empty>{loading ? 'Loading…' : 'No events match.'}</Empty> : <EventList events={shown} />}
      </Card>
    </div>
  )
}
