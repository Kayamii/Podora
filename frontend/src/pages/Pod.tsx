import { useState } from 'react'
import { Trash2 } from 'lucide-react'
import { ConfirmDialog } from '../components/ConfirmDialog'
import { EventList } from '../components/EventList'
import { LogViewer } from '../components/LogViewer'
import { Meter } from '../components/Meter'
import { UsagePanel } from '../components/UsagePanel'
import { NUM, ROW, TD, TH } from '../components/styles'
import { Badge, Button, Card, Facts, Labels, Notice, PageHeader, Status, TableScroll, TextLink } from '../components/ui'
import { usePolling } from '../hooks/usePolling'
import { useSession } from '../hooks/session'
import { href, navigate } from '../hooks/useRoute'
import { api } from '../services/api'
import { formatAge, formatBytes, formatCPU, percent } from '../services/format'
import { plural, podLevel } from '../services/health'
import type { Level } from '../services/health'
import type { ContainerSummary, PodDetail, WorkloadKind } from '../types/api'

export function Pod({ namespace, name }: { namespace: string; name: string }) {
  const { can } = useSession()
  const { data, error, loading } = usePolling((signal) => api.pod(namespace, name, signal))
  const [confirmDelete, setConfirmDelete] = useState(false)

  const breadcrumb = [
    { label: 'Pods', to: href.pods() },
    { label: namespace, to: href.pods(namespace) },
  ]

  if (!data) {
    return (
      <div>
        <PageHeader title={name} breadcrumb={breadcrumb} />
        {loading ? (
          <p className="text-fg-2">Loading…</p>
        ) : (
          <Notice level="fault">
            {error?.includes('not found')
              ? 'This pod no longer exists. Pods are replaced when a workload restarts or scales, so it may have a successor with a new name.'
              : `Could not load this pod: ${error}`}{' '}
            <TextLink to={href.pods(namespace)}>See pods in {namespace}</TextLink>
          </Notice>
        )}
      </div>
    )
  }

  const main = data.containers.filter((c) => !c.init)
  const ready = main.filter((c) => c.ready).length
  const level = podLevel(data)
  const owner = ownerLink(data)

  return (
    <div>
      <PageHeader
        title={data.name}
        breadcrumb={breadcrumb}
        status={<Badge level={level}>{data.status}</Badge>}
        subtitle={`${ready} of ${plural(main.length, 'container')} ready · ${plural(data.restarts, 'restart')} · on ${data.node || 'no node yet'}`}
        actions={
          can('pods.delete', namespace) && (
            <Button variant="dangerOutline" icon={<Trash2 size={15} />} onClick={() => setConfirmDelete(true)}>
              Delete
            </Button>
          )
        }
      />

      {error && <Notice level="warning">Not refreshing right now: {error}</Notice>}

      <Card title="Logs" padded={false}>
        <LogViewer namespace={data.namespace} pod={data.name} containers={data.containers} />
      </Card>

      <div className="mt-6">
        <UsagePanel
          namespace={data.namespace}
          pod={data.name}
          cpuNote={totals(main, 'cpu')}
          memoryNote={totals(main, 'memory')}
        />
      </div>

      <Card title="Containers" padded={false} className="mt-6">
        <TableScroll>
          <thead>
            <tr>
              <th className={TH}>Name</th>
              <th className={TH}>State</th>
              <th className={`${TH} ${NUM}`}>Restarts</th>
              <th className={TH}>Last termination</th>
              <th className={`${TH} min-w-48`}>CPU</th>
              <th className={`${TH} min-w-48`}>Memory</th>
            </tr>
          </thead>
          <tbody>
            {data.containers.map((c) => (
              <tr key={`${c.init}-${c.name}`} className={ROW}>
                <td className={`${TD} align-top`}>
                  <div className="font-medium">
                    {c.name}
                    {c.init && <span className="ml-2 rounded bg-subtle px-1.5 py-0.5 text-xs font-normal text-fg-2">init</span>}
                  </div>
                  <div className="font-mono text-[0.8125rem] break-all text-muted">{c.image}</div>
                  {c.ports.length > 0 && <div className="text-sm text-muted">Ports {c.ports.join(', ')}</div>}
                </td>
                <td className={`${TD} align-top`}>
                  <Status level={containerLevel(c)}>{c.reason || c.state}</Status>
                  {c.state === 'Running' && c.startedAt && <div className="text-sm text-muted">for {formatAge(c.startedAt)}</div>}
                  {c.message && <div className="mt-1 max-w-80 text-sm break-words text-fg-2">{c.message}</div>}
                </td>
                <td className={`${TD} ${NUM} align-top`}>{c.restarts}</td>
                <td className={`${TD} align-top`}>
                  {c.lastTermination ? (
                    <>
                      {c.lastTermination.reason} · exit {c.lastTermination.exitCode}
                      <div className="text-sm text-muted">{formatAge(c.lastTermination.finishedAt)} ago</div>
                    </>
                  ) : (
                    <span className="text-muted">—</span>
                  )}
                </td>
                <td className={`${TD} align-top`}>
                  <ContainerUsage name={`${c.name} CPU`} used={c.cpuUsage} request={c.cpuRequest} limit={c.cpuLimit} format={formatCPU} />
                </td>
                <td className={`${TD} align-top`}>
                  <ContainerUsage name={`${c.name} memory`} used={c.memoryUsage} request={c.memoryRequest} limit={c.memoryLimit} format={formatBytes} />
                </td>
              </tr>
            ))}
          </tbody>
        </TableScroll>
      </Card>

      <div className="mt-6 grid gap-6 xl:grid-cols-2">
        <Card title="Details">
          <Facts
            rows={{
              Namespace: <TextLink to={href.pods(data.namespace)}>{data.namespace}</TextLink>,
              'Controlled by': owner ? <TextLink to={owner.to}>{owner.label}</TextLink> : data.owner || 'Nothing (standalone pod)',
              Node: data.node || 'Not scheduled',
              'Pod IP': data.ip || '—',
              'Host IP': data.hostIP || '—',
              'Service account': data.serviceAccount,
              'QoS class': data.qosClass || '—',
              'CPU in use': data.cpuUsage != null ? formatCPU(data.cpuUsage) : '—',
              'Memory in use': data.memoryUsage != null ? formatBytes(data.memoryUsage) : '—',
              Age: formatAge(data.createdAt),
              Labels: <Labels labels={data.labels} />,
            }}
          />
        </Card>
        <Card title="Events" padded={false}>
          {data.events.length === 0 ? (
            <p className="px-5 py-8 text-center text-fg-2">No recent events. The cluster keeps events for about an hour.</p>
          ) : (
            <EventList events={data.events} hideObject />
          )}
        </Card>
      </div>

      <ConfirmDialog
        open={confirmDelete}
        title={`Delete ${data.name}?`}
        confirmLabel="Delete pod"
        danger
        onClose={() => setConfirmDelete(false)}
        onConfirm={async () => {
          await api.deletePod(data.namespace, data.name)
          navigate(owner ? owner.to : href.pods(data.namespace))
        }}
      >
        {data.owner
          ? `It is managed by ${data.owner}, which will start a replacement pod. Use this to restart a single stuck pod.`
          : 'Nothing manages this pod, so it will not come back.'}
      </ConfirmDialog>
    </div>
  )
}

/**
 * The workload page that owns this pod. A Deployment's pods are owned by a
 * ReplicaSet named "<deployment>-<hash>"; the pod's own name repeats that
 * hash, which lets us recover the Deployment without another request.
 */
function ownerLink(pod: PodDetail): { label: string; to: string } | null {
  const [kind, name] = pod.owner.split('/')
  if (!name) return null
  if (kind === 'ReplicaSet') {
    const hash = pod.labels['pod-template-hash']
    if (hash && name.endsWith(`-${hash}`)) {
      const deployment = name.slice(0, -hash.length - 1)
      return { label: `Deployment/${deployment}`, to: href.workload('Deployment', pod.namespace, deployment) }
    }
    return null
  }
  if (kind === 'StatefulSet' || kind === 'DaemonSet') {
    return { label: pod.owner, to: href.workload(kind as WorkloadKind, pod.namespace, name) }
  }
  return null
}

function containerLevel(c: ContainerSummary): Level {
  if (c.state === 'Running') return c.ready ? 'normal' : 'warning'
  if (c.state === 'Terminated' && c.reason === 'Completed') return 'neutral'
  if (c.state === 'Waiting' && ['ContainerCreating', 'PodInitializing'].includes(c.reason)) return 'warning'
  return 'fault'
}

interface UsageProps {
  name: string
  used: number | null
  request: number
  limit: number
  format: (n: number) => string
}

/**
 * Usage measured against the limit (what gets a container throttled or
 * killed), or against the request when no limit is set.
 */
function ContainerUsage({ name, used, request, limit, format }: UsageProps) {
  const against = limit > 0 ? limit : request
  const detail = `req ${request > 0 ? format(request) : '—'} · limit ${limit > 0 ? format(limit) : '—'}`
  if (used == null) return <span className="text-sm text-fg-2 tabular-nums">{detail}</span>
  const pct = against > 0 ? percent(used, against) : 0
  return (
    <div>
      <div className="mb-1.5 flex items-baseline justify-between gap-3 text-sm tabular-nums">
        <span className="font-medium">{format(used)}</span>
        {against > 0 && (
          <span className="text-muted">
            {Math.round(pct)}% of {limit > 0 ? 'limit' : 'request'}
          </span>
        )}
      </div>
      {against > 0 && <Meter label={`${name} against ${limit > 0 ? 'limit' : 'request'}`} percent={pct} />}
      <div className="mt-1 text-xs text-muted tabular-nums">{detail}</div>
    </div>
  )
}

/** "Requested 200m · limit 1 core" summed over a pod's containers. */
function totals(containers: ContainerSummary[], kind: 'cpu' | 'memory'): string {
  const fmt = kind === 'cpu' ? formatCPU : formatBytes
  const req = containers.reduce((n, c) => n + (kind === 'cpu' ? c.cpuRequest : c.memoryRequest), 0)
  const lim = containers.reduce((n, c) => n + (kind === 'cpu' ? c.cpuLimit : c.memoryLimit), 0)
  return `Requested ${req > 0 ? fmt(req) : 'nothing'} · limit ${lim > 0 ? fmt(lim) : 'none set'}`
}
