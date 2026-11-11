import { useState } from 'react'
import { NamespaceFilter, SearchField } from '../components/NamespaceFilter'
import { PodTable } from '../components/PodTable'
import { Card, Notice, PageHeader, Switch } from '../components/ui'
import { usePolling } from '../hooks/usePolling'
import { href, navigate } from '../hooks/useRoute'
import { api } from '../services/api'
import { plural } from '../services/health'

export function Pods({ namespace }: { namespace: string }) {
  const { data, error, loading } = usePolling(api.pods)
  const [search, setSearch] = useState('')
  const [problemsOnly, setProblemsOnly] = useState(false)

  const all = data ?? []
  const needle = search.trim().toLowerCase()
  const inNamespace = all.filter((p) => namespace === '' || p.namespace === namespace)
  const shown = inNamespace.filter(
    (p) => (needle === '' || p.name.toLowerCase().includes(needle)) && (!problemsOnly || !p.healthy),
  )
  const problems = inNamespace.filter((p) => !p.healthy).length

  return (
    <div>
      <PageHeader
        title="Pods"
        subtitle={
          loading
            ? 'Loading…'
            : `${plural(inNamespace.length, 'pod')}${namespace ? ` in ${namespace}` : ''} · ${
                problems === 0 ? 'all healthy' : `${problems} not healthy`
              }`
        }
      />

      <div className="mb-4 flex flex-wrap items-end gap-x-4 gap-y-3">
        <NamespaceFilter
          namespaces={[...new Set(all.map((p) => p.namespace))]}
          value={namespace}
          onChange={(ns) => navigate(href.pods(ns))}
        />
        <SearchField value={search} onChange={setSearch} placeholder="Pod name" />
        <Switch checked={problemsOnly} onChange={setProblemsOnly}>
          Problems only
        </Switch>
      </div>

      {error && <Notice level="warning">Could not refresh the list: {error}</Notice>}

      <Card padded={false}>
        {loading ? (
          <p className="px-5 py-8 text-center text-fg-2">Loading…</p>
        ) : (
          <PodTable
            pods={shown}
            showNamespace={namespace === ''}
            empty={all.length === 0 ? 'No pods in this cluster yet.' : 'No pods match these filters.'}
          />
        )}
      </Card>
    </div>
  )
}
