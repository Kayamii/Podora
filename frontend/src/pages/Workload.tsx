import { useState } from 'react'
import { RotateCw, Scaling } from 'lucide-react'
import { ConfirmDialog } from '../components/ConfirmDialog'
import { EventList } from '../components/EventList'
import { PodTable } from '../components/PodTable'
import { Badge, Button, Card, Facts, Field, Labels, Notice, PageHeader } from '../components/ui'
import { usePolling } from '../hooks/usePolling'
import { useSession } from '../hooks/session'
import { href } from '../hooks/useRoute'
import { api } from '../services/api'
import { formatAge } from '../services/format'
import { plural, workloadStatus } from '../services/health'
import type { WorkloadDetail, WorkloadKind } from '../types/api'

interface Props {
  kind: WorkloadKind
  namespace: string
  name: string
}

type Pending = 'scale' | 'restart' | null

export function Workload({ kind, namespace, name }: Props) {
  const { can } = useSession()
  const { data, error, loading, reload } = usePolling((signal) => api.workload(kind, namespace, name, signal))
  const [pending, setPending] = useState<Pending>(null)
  const [done, setDone] = useState<string | null>(null)

  const breadcrumb = [
    { label: 'Workloads', to: href.workloads() },
    { label: namespace, to: href.workloads(namespace) },
  ]

  if (!data) {
    return (
      <div>
        <PageHeader title={name} breadcrumb={breadcrumb} subtitle={kind} />
        {loading ? (
          <p className="text-fg-2">Loading…</p>
        ) : (
          <Notice level="fault">
            {error?.includes('not found') ? `This ${kind} no longer exists.` : `Could not load this ${kind}: ${error}`}
          </Notice>
        )}
      </div>
    )
  }

  const status = workloadStatus(data)
  const finish = (message: string) => {
    setDone(message)
    reload()
  }

  return (
    <div>
      <PageHeader
        title={data.name}
        breadcrumb={breadcrumb}
        status={<Badge level={status.level}>{status.label}</Badge>}
        subtitle={`${data.kind} · ${data.ready} of ${plural(data.desired, data.kind === 'DaemonSet' ? 'node' : 'replica')} ready`}
        actions={
          <>
            {data.scalable && can('workloads.scale', namespace) && (
              <Button icon={<Scaling size={15} />} onClick={() => setPending('scale')}>
                Scale
              </Button>
            )}
            {can('workloads.restart', namespace) && (
              <Button icon={<RotateCw size={15} />} onClick={() => setPending('restart')}>
                Restart
              </Button>
            )}
          </>
        }
      />

      {done && <Notice level="normal">{done}</Notice>}
      {error && <Notice level="warning">Not refreshing right now: {error}</Notice>}

      <div className="grid gap-6 xl:grid-cols-3">
        <div className="space-y-6 xl:col-span-2">
          <Card title="Pods" aside={plural(data.pods.length, 'pod')} padded={false}>
            <PodTable pods={data.pods} empty="This workload has no pods right now." />
          </Card>
          <Card title="Events" padded={false}>
            {data.events.length === 0 ? (
              <p className="px-5 py-8 text-center text-fg-2">No recent events. The cluster keeps events for about an hour.</p>
            ) : (
              <EventList events={data.events} hideObject />
            )}
          </Card>
        </div>

        <div className="space-y-6">
          <Card title="Details">
            <Facts
              rows={{
                Namespace: data.namespace,
                Kind: data.kind,
                'Update strategy': data.strategy || 'Default',
                Updated: `${data.updated} of ${data.desired}`,
                Available: `${data.available} of ${data.desired}`,
                Age: formatAge(data.createdAt),
              }}
            />
          </Card>
          <Card title="Containers">
            <ul className="space-y-1.5">
              {data.images.map((img) => (
                <li key={img} className="font-mono text-[0.8125rem] break-all">
                  {img}
                </li>
              ))}
            </ul>
          </Card>
          <Card title="Labels">
            <Facts rows={{ Selector: <Labels labels={data.selector} />, Labels: <Labels labels={data.labels} /> }} />
          </Card>
          {data.conditions.length > 0 && (
            <Card title="Conditions">
              <Facts
                rows={Object.fromEntries(
                  data.conditions.map((c) => [c.type, `${c.status === 'True' ? 'Yes' : 'No'}${c.reason ? ` · ${c.reason}` : ''}`]),
                )}
              />
            </Card>
          )}
        </div>
      </div>

      {pending === 'scale' && (
        <ScaleDialog workload={data} onClose={() => setPending(null)} onDone={finish} />
      )}
      <ConfirmDialog
        open={pending === 'restart'}
        title={`Restart ${data.name}?`}
        confirmLabel="Restart"
        onClose={() => setPending(null)}
        onConfirm={async () => {
          await api.restartWorkload(data.kind, data.namespace, data.name)
          finish(`Restart started. Pods are being replaced one by one following the ${data.strategy || 'default'} strategy.`)
        }}
      >
        Every pod of this {data.kind} is replaced with a new one, following its update strategy. This is the same as{' '}
        <code className="font-mono text-sm">kubectl rollout restart</code>.
      </ConfirmDialog>
    </div>
  )
}

function ScaleDialog({ workload, onClose, onDone }: { workload: WorkloadDetail; onClose: () => void; onDone: (m: string) => void }) {
  const [replicas, setReplicas] = useState(String(workload.desired))
  const n = Number(replicas)
  const valid = replicas.trim() !== '' && Number.isInteger(n) && n >= 0 && n <= 500

  return (
    <ConfirmDialog
      open
      title={`Scale ${workload.name}`}
      confirmLabel="Scale"
      danger={valid && n === 0}
      onClose={onClose}
      onConfirm={async () => {
        if (!valid) throw new Error('Enter a whole number between 0 and 500.')
        await api.scaleWorkload(workload.kind, workload.namespace, workload.name, n)
        onDone(`Scaled ${workload.name} from ${workload.desired} to ${plural(n, 'replica')}.`)
      }}
    >
      <p className="mb-4">Currently {plural(workload.desired, 'replica')}.</p>
      <Field label="Replicas">
        <input
          type="number"
          min={0}
          max={500}
          className="input w-32"
          value={replicas}
          onChange={(e) => setReplicas(e.target.value)}
          autoFocus
        />
      </Field>
      {valid && n === 0 && (
        <p className="mt-3 text-sm text-danger-fg">Scaling to 0 stops every pod. The workload will serve nothing until scaled up again.</p>
      )}
    </ConfirmDialog>
  )
}
