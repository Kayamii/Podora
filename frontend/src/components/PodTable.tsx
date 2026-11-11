import { useState } from 'react'
import { ArrowDown, ArrowUp } from 'lucide-react'
import { href } from '../hooks/useRoute'
import { formatAge, formatBytes, formatCPU } from '../services/format'
import { podLevel } from '../services/health'
import type { PodSummary } from '../types/api'
import { NUM, ROW_LINK, STRETCHED_LINK, TD, TH } from './styles'
import { Empty, Status, TableScroll } from './ui'

type SortKey = 'name' | 'restarts' | 'cpu' | 'memory' | 'age'

const COMPARE: Record<SortKey, (a: PodSummary, b: PodSummary) => number> = {
  name: (a, b) => a.namespace.localeCompare(b.namespace) || a.name.localeCompare(b.name),
  restarts: (a, b) => a.restarts - b.restarts,
  cpu: (a, b) => (a.cpuUsage ?? -1) - (b.cpuUsage ?? -1),
  memory: (a, b) => (a.memoryUsage ?? -1) - (b.memoryUsage ?? -1),
  // Newer pods first when sorting by age ascending.
  age: (a, b) => b.createdAt.localeCompare(a.createdAt),
}

interface Props {
  pods: PodSummary[]
  /** Show the namespace column (lists that span namespaces). */
  showNamespace?: boolean
  empty: string
  /** Initial sort; heaviest first for numeric columns. */
  initialSort?: SortKey
}

/** Pods as rows; each row opens the pod's page. Columns sort on click. */
export function PodTable({ pods, showNamespace = false, empty, initialSort = 'name' }: Props) {
  const [sort, setSort] = useState<{ key: SortKey; desc: boolean }>({
    key: initialSort,
    desc: initialSort === 'cpu' || initialSort === 'memory' || initialSort === 'restarts',
  })

  if (pods.length === 0) return <Empty>{empty}</Empty>

  const sorted = [...pods].sort((a, b) => (sort.desc ? -1 : 1) * COMPARE[sort.key](a, b))

  function header(key: SortKey, label: string, numeric = false) {
    const active = sort.key === key
    return (
      <th className={`${TH} ${numeric ? NUM : ''}`} aria-sort={active ? (sort.desc ? 'descending' : 'ascending') : 'none'}>
        <button
          type="button"
          onClick={() => setSort({ key, desc: active ? !sort.desc : numeric && key !== 'age' })}
          className={`inline-flex cursor-pointer items-center gap-1 hover:text-fg ${active ? 'text-fg' : ''}`}
        >
          {label}
          {active && (sort.desc ? <ArrowDown size={13} aria-hidden="true" /> : <ArrowUp size={13} aria-hidden="true" />)}
        </button>
      </th>
    )
  }

  return (
    <TableScroll>
      <thead>
        <tr>
          {header('name', 'Name')}
          {showNamespace && <th className={TH}>Namespace</th>}
          <th className={TH}>Status</th>
          <th className={`${TH} ${NUM}`}>Ready</th>
          {header('restarts', 'Restarts', true)}
          {header('cpu', 'CPU', true)}
          {header('memory', 'Memory', true)}
          <th className={TH}>Node</th>
          {header('age', 'Age', true)}
        </tr>
      </thead>
      <tbody>
        {sorted.map((p) => (
          <tr key={`${p.namespace}/${p.name}`} className={ROW_LINK}>
            <td className={`${TD} whitespace-nowrap`}>
              <a href={href.pod(p.namespace, p.name)} className={STRETCHED_LINK}>
                {p.name}
              </a>
            </td>
            {showNamespace && <td className={`${TD} text-fg-2`}>{p.namespace}</td>}
            <td className={TD}>
              <Status level={podLevel(p)}>{p.status}</Status>
            </td>
            <td className={`${TD} ${NUM}`}>
              {p.readyContainers}/{p.containers}
            </td>
            <td className={`${TD} ${NUM} ${p.restarts > 0 ? 'text-warn-fg' : ''}`}>{p.restarts}</td>
            <td className={`${TD} ${NUM} text-fg-2`}>{p.cpuUsage != null ? formatCPU(p.cpuUsage) : '—'}</td>
            <td className={`${TD} ${NUM} whitespace-nowrap text-fg-2`}>
              {p.memoryUsage != null ? formatBytes(p.memoryUsage) : '—'}
            </td>
            <td className={`${TD} whitespace-nowrap text-fg-2`}>{p.node || 'Not scheduled'}</td>
            <td className={`${TD} ${NUM} text-fg-2`}>{formatAge(p.createdAt)}</td>
          </tr>
        ))}
      </tbody>
    </TableScroll>
  )
}
