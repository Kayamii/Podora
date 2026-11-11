import { useState } from 'react'
import { ScrollText } from 'lucide-react'
import { SearchField } from '../components/NamespaceFilter'
import { ROW, TD, TH } from '../components/styles'
import { Card, EmptyState, Field, Notice, PageHeader, Segmented, Skeleton, Status, TableScroll } from '../components/ui'
import { usePolling } from '../hooks/usePolling'
import { accounts } from '../services/api'
import { formatAge } from '../services/format'
import { plural } from '../services/health'
import type { AuditEntry } from '../types/api'

/** Human wording for each recorded action. */
const ACTIONS: Record<string, string> = {
  'auth.setup': 'Set up Podora',
  'auth.login': 'Signed in',
  'auth.logout': 'Signed out',
  'pod.delete': 'Deleted pod',
  'workload.scale': 'Scaled',
  'workload.restart': 'Restarted',
  'helm.install': 'Installed release',
  'helm.upgrade': 'Upgraded release',
  'helm.rollback': 'Rolled back release',
  'helm.uninstall': 'Uninstalled release',
  'user.create': 'Added user',
  'user.update': 'Changed user',
  'user.delete': 'Deleted user',
  'user.password': 'Changed password',
}

const CATEGORIES = [
  { value: '', label: 'All activity' },
  { value: 'cluster', label: 'Cluster changes' },
  { value: 'helm.', label: 'Helm' },
  { value: 'user.', label: 'Accounts' },
  { value: 'auth.', label: 'Sign-ins' },
]

function inCategory(e: AuditEntry, category: string): boolean {
  if (category === '') return true
  if (category === 'cluster') return e.action.startsWith('pod.') || e.action.startsWith('workload.')
  return e.action.startsWith(category)
}

export function Audit() {
  const { data, error, loading } = usePolling(accounts.audit, 10_000)
  const [category, setCategory] = useState('')
  const [user, setUser] = useState('')
  const [outcome, setOutcome] = useState<'all' | 'failed'>('all')
  const [search, setSearch] = useState('')

  const all = data ?? []
  const needle = search.trim().toLowerCase()
  const shown = all.filter(
    (e) =>
      inCategory(e, category) &&
      (user === '' || e.username === user) &&
      (outcome === 'all' || !e.success) &&
      (needle === '' || `${e.target} ${e.namespace} ${e.detail} ${e.error}`.toLowerCase().includes(needle)),
  )
  const failures = all.filter((e) => !e.success).length

  return (
    <div>
      <PageHeader
        title="Audit log"
        subtitle={loading ? 'Loading…' : `Who changed what · last ${plural(all.length, 'entry', 'entries')}, ${failures} failed · kept 180 days`}
      />

      <div className="mb-4 flex flex-wrap items-end gap-x-4 gap-y-3">
        <Field label="Activity">
          <select className="input" value={category} onChange={(e) => setCategory(e.target.value)}>
            {CATEGORIES.map((c) => (
              <option key={c.value} value={c.value}>
                {c.label}
              </option>
            ))}
          </select>
        </Field>
        <Field label="User">
          <select className="input min-w-36" value={user} onChange={(e) => setUser(e.target.value)}>
            <option value="">Everyone</option>
            {[...new Set(all.map((e) => e.username))].sort().map((u) => (
              <option key={u} value={u}>
                {u}
              </option>
            ))}
          </select>
        </Field>
        <SearchField value={search} onChange={setSearch} placeholder="Resource, namespace, detail" />
        <Segmented
          label="Outcome"
          value={outcome}
          onChange={setOutcome}
          options={[
            { value: 'all', label: 'All' },
            { value: 'failed', label: 'Failed only' },
          ]}
        />
      </div>

      {error && <Notice level="warning">Could not refresh: {error}</Notice>}

      <Card padded={false}>
        {loading ? (
          <Skeleton />
        ) : all.length === 0 ? (
          <EmptyState icon={<ScrollText size={22} />} title="Nothing recorded yet">
            Sign-ins, account changes and every change made to the cluster through Podora appear here.
          </EmptyState>
        ) : shown.length === 0 ? (
          <p className="px-5 py-8 text-center text-fg-2">No entries match these filters.</p>
        ) : (
          <TableScroll>
            <thead>
              <tr>
                <th className={TH}>When</th>
                <th className={TH}>User</th>
                <th className={TH}>Action</th>
                <th className={TH}>On</th>
                <th className={TH}>Result</th>
              </tr>
            </thead>
            <tbody>
              {shown.map((e) => (
                <tr key={e.id} className={ROW}>
                  <td className={`${TD} whitespace-nowrap`} title={new Date(e.time).toLocaleString()}>
                    {formatAge(e.time)} ago
                    <div className="text-xs text-muted tabular-nums">{new Date(e.time).toLocaleString()}</div>
                  </td>
                  <td className={TD}>
                    <span className="font-medium">{e.username}</span>
                    {e.remote && <div className="font-mono text-xs text-muted">{e.remote}</div>}
                  </td>
                  <td className={TD}>
                    {ACTIONS[e.action] ?? e.action}
                    {e.detail && <div className="text-sm text-fg-2">{e.detail}</div>}
                  </td>
                  <td className={`${TD} text-sm`}>
                    {e.target && <span className="font-medium">{e.target}</span>}
                    {e.namespace && <div className="text-muted">in {e.namespace}</div>}
                  </td>
                  <td className={TD}>
                    <Status level={e.success ? 'normal' : 'fault'}>{e.success ? 'Done' : 'Failed'}</Status>
                    {e.error && <div className="mt-0.5 max-w-80 text-sm break-words text-danger-fg">{e.error}</div>}
                  </td>
                </tr>
              ))}
            </tbody>
          </TableScroll>
        )}
      </Card>
    </div>
  )
}
