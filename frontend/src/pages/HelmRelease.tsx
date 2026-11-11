import { useState } from 'react'
import { Eye, History, RotateCcw, Save, Trash2, Upload } from 'lucide-react'
import { ChartCard, ChartIcon, ChartSource } from '../components/ChartSource'
import { CodeBlock } from '../components/CodeBlock'
import { ConfirmDialog } from '../components/ConfirmDialog'
import { NUM, ROW, TD, TH } from '../components/styles'
import { ValuesEditor } from '../components/ValuesEditor'
import { Badge, Button, Card, Facts, Notice, PageHeader, Status, TableScroll, Tabs, TextLink } from '../components/ui'
import { usePolling } from '../hooks/usePolling'
import { useSession } from '../hooks/session'
import { href, navigate } from '../hooks/useRoute'
import { helm } from '../services/api'
import { formatAge } from '../services/format'
import { plural, releaseLevel } from '../services/health'
import type { ChartInfo, ReleaseDetail, ReleaseResource, Rendered } from '../types/api'

type Tab = 'overview' | 'values' | 'history' | 'manifest'

export function HelmRelease({ namespace, name }: { namespace: string; name: string }) {
  const { can } = useSession()
  const manage = can('helm.manage')
  const { data, error, loading, reload } = usePolling((signal) => helm.release(namespace, name, signal))
  const [tab, setTab] = useState<Tab>('overview')
  const [done, setDone] = useState<string | null>(null)
  const [confirmUninstall, setConfirmUninstall] = useState(false)

  const breadcrumb = [{ label: 'Helm releases', to: href.helm }]

  if (!data) {
    return (
      <div>
        <PageHeader title={name} breadcrumb={breadcrumb} />
        {loading ? (
          <p className="text-fg-2">Loading…</p>
        ) : (
          <Notice level="fault">
            {error?.includes('not found') ? 'This release does not exist. It may have been uninstalled.' : `Could not load this release: ${error}`}
          </Notice>
        )}
      </div>
    )
  }

  const finish = (message: string) => {
    setDone(message)
    reload()
  }

  return (
    <div>
      <PageHeader
        title={
          <span className="flex items-center gap-3">
            <ChartIcon src={data.icon} size={32} />
            {data.name}
          </span>
        }
        breadcrumb={breadcrumb}
        status={<Badge level={releaseLevel(data.status)}>{data.status}</Badge>}
        subtitle={`${data.chart} ${data.chartVersion}${data.appVersion ? ` · app ${data.appVersion}` : ''} · ${data.namespace} · revision ${data.revision} · updated ${formatAge(data.updated)} ago`}
        actions={
          manage && (
            <Button variant="dangerOutline" icon={<Trash2 size={15} />} onClick={() => setConfirmUninstall(true)}>
              Uninstall
            </Button>
          )
        }
      />

      {done && <Notice level="normal">{done}</Notice>}
      {error && <Notice level="warning">Not refreshing right now: {error}</Notice>}

      <Tabs
        label="Release views"
        value={tab}
        onChange={setTab}
        tabs={[
          { value: 'overview', label: 'Overview' },
          { value: 'values', label: manage ? 'Values & upgrade' : 'Values' },
          { value: 'history', label: 'History', count: data.history.length },
          { value: 'manifest', label: 'Manifest', count: data.resources.length },
        ]}
      />

      {tab === 'overview' && <OverviewTab release={data} />}
      {/* Keyed by revision: after an upgrade the editor starts from the new values. */}
      {tab === 'values' &&
        (manage ? (
          <ValuesTab key={data.revision} release={data} onDone={finish} />
        ) : (
          <Card title="Release values" aside="Read only">
            <ValuesEditor label="Release values" value={data.values || '# No values set; the chart defaults apply.'} onChange={() => {}} readOnly />
          </Card>
        ))}
      {tab === 'history' && <HistoryTab release={data} onDone={finish} canRollback={manage} />}
      {tab === 'manifest' && <CodeBlock code={data.manifest} label="Rendered manifest" maxHeight="70vh" />}

      <UninstallDialog
        open={confirmUninstall}
        release={data}
        onClose={() => setConfirmUninstall(false)}
        onDone={() => navigate(href.helm)}
      />
    </div>
  )
}

// --- Overview ----------------------------------------------------------------

const WORKLOAD_KINDS = ['Deployment', 'StatefulSet', 'DaemonSet'] as const

function resourceLink(r: ReleaseResource): string | null {
  const kind = WORKLOAD_KINDS.find((k) => k === r.kind)
  if (kind) return href.workload(kind, r.namespace, r.name)
  if (r.kind === 'Service' || r.kind === 'Ingress') return href.network(r.namespace)
  if (r.kind === 'Namespace') return href.namespace(r.name)
  return null
}

function OverviewTab({ release }: { release: ReleaseDetail }) {
  return (
    <div className="grid gap-6 xl:grid-cols-3">
      <div className="space-y-6 xl:col-span-2">
        <Card title="Resources" aside={plural(release.resources.length, 'object')} padded={false}>
          {release.resources.length === 0 ? (
            <p className="px-5 py-8 text-center text-fg-2">This release created no resources.</p>
          ) : (
            <TableScroll>
              <thead>
                <tr>
                  <th className={TH}>Kind</th>
                  <th className={TH}>Name</th>
                  <th className={TH}>Namespace</th>
                </tr>
              </thead>
              <tbody>
                {release.resources.map((r) => {
                  const link = resourceLink(r)
                  return (
                    <tr key={`${r.kind}/${r.namespace}/${r.name}`} className={ROW}>
                      <td className={`${TD} text-fg-2`}>{r.kind}</td>
                      <td className={`${TD} font-medium`}>{link ? <TextLink to={link}>{r.name}</TextLink> : r.name}</td>
                      <td className={`${TD} text-fg-2`}>{r.namespace}</td>
                    </tr>
                  )
                })}
              </tbody>
            </TableScroll>
          )}
        </Card>
        {release.notes && (
          <Card title="Notes" aside="Written by the chart">
            <pre className="font-mono text-[0.8125rem] leading-relaxed whitespace-pre-wrap text-fg-2">{release.notes.trim()}</pre>
          </Card>
        )}
      </div>
      <Card title="Details">
        <Facts
          rows={{
            Chart: `${release.chart} ${release.chartVersion}`,
            'App version': release.appVersion || '—',
            Namespace: <TextLink to={href.namespace(release.namespace)}>{release.namespace}</TextLink>,
            Revision: release.revision,
            'Last action': release.description || '—',
            'First installed': `${formatAge(release.firstDeployed)} ago`,
            'Last updated': `${formatAge(release.updated)} ago`,
            ...(release.chartInfo.home ? { Homepage: <TextLink to={release.chartInfo.home}>{release.chartInfo.home}</TextLink> } : {}),
          }}
        />
        {release.chartInfo.description && <p className="mt-4 border-t border-line pt-4 text-sm text-fg-2">{release.chartInfo.description}</p>}
      </Card>
    </div>
  )
}

// --- Values and upgrade ------------------------------------------------------

function ValuesTab({ release, onDone }: { release: ReleaseDetail; onDone: (m: string) => void }) {
  const [values, setValues] = useState(release.values)
  const [newChart, setNewChart] = useState<ChartInfo | null>(null)
  const [pickChart, setPickChart] = useState(false)
  const [showDefaults, setShowDefaults] = useState(false)
  const [preview, setPreview] = useState<Rendered | null>(null)
  const [previewing, setPreviewing] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [confirm, setConfirm] = useState(false)

  const changed = values !== release.values || newChart != null
  const request = { chartId: newChart?.id, values, dryRun: false }

  async function runPreview() {
    setPreviewing(true)
    setError(null)
    try {
      const res = await helm.upgrade(release.namespace, release.name, { ...request, dryRun: true })
      setPreview(res.rendered ?? null)
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setPreviewing(false)
    }
  }

  return (
    <div className="space-y-6">
      <Card
        title="Chart version"
        aside={!pickChart && !newChart && <button type="button" className="cursor-pointer text-accent hover:underline" onClick={() => setPickChart(true)}>Use a different version</button>}
      >
        {newChart ? (
          <ChartCard
            chart={newChart}
            action={
              <div className="flex flex-col items-end gap-2">
                <Badge level="neutral">
                  {release.chartVersion} → {newChart.version}
                </Badge>
                <Button onClick={() => setNewChart(null)}>Keep current</Button>
              </div>
            }
          />
        ) : pickChart ? (
          <div>
            <ChartSource
              onChart={(c) => {
                setNewChart(c)
                setPickChart(false)
              }}
            />
            <Button className="mt-4" onClick={() => setPickChart(false)}>
              Cancel
            </Button>
          </div>
        ) : (
          <p className="text-fg-2">
            Using <span className="font-medium text-fg">{release.chart} {release.chartVersion}</span>. Upgrading keeps this chart unless you choose another version.
          </p>
        )}
      </Card>

      <div className={`grid gap-6 ${showDefaults ? 'xl:grid-cols-2' : ''}`}>
        <Card
          title="Release values"
          aside={
            <span className="flex gap-4">
              {values !== release.values && (
                <button type="button" className="cursor-pointer text-accent hover:underline" onClick={() => setValues(release.values)}>
                  Undo changes
                </button>
              )}
              <button type="button" className="cursor-pointer text-accent hover:underline" onClick={() => setShowDefaults((v) => !v)}>
                {showDefaults ? 'Hide chart defaults' : 'Show chart defaults'}
              </button>
            </span>
          }
        >
          <p className="mb-3 text-fg-2">
            The values this release overrides. Anything not listed uses the chart default. Saving replaces this set as a whole.
          </p>
          <ValuesEditor label="Release values (YAML)" value={values} onChange={setValues} />
        </Card>
        {showDefaults && (
          <Card
            title="Chart defaults"
            aside={
              <button type="button" className="cursor-pointer text-accent hover:underline" onClick={() => setValues(newChart?.values ?? release.defaultValues)}>
                Copy into editor
              </button>
            }
          >
            <p className="mb-3 text-fg-2">Read only. Copy a setting into your values to change it.</p>
            <ValuesEditor label="Chart default values" value={newChart?.values ?? release.defaultValues} onChange={() => {}} readOnly />
          </Card>
        )}
      </div>

      {error && <Notice level="fault">{error}</Notice>}

      <div className="sticky bottom-0 z-10 -mx-4 flex flex-wrap items-center justify-end gap-3 border-t border-line bg-canvas/90 px-4 py-4 backdrop-blur sm:-mx-6 sm:px-6 lg:-mx-8 lg:px-8">
        <span className="mr-auto text-sm text-fg-2">{changed ? 'Unsaved changes' : 'No changes'}</span>
        <Button icon={<Eye size={15} />} disabled={previewing} onClick={runPreview}>
          {previewing ? 'Rendering…' : 'Preview manifest'}
        </Button>
        <Button variant="primary" icon={newChart ? <Upload size={15} /> : <Save size={15} />} disabled={!changed} onClick={() => setConfirm(true)}>
          Upgrade
        </Button>
      </div>

      {preview && (
        <Card title="Preview" aside="What Upgrade would apply · nothing has changed yet">
          <CodeBlock code={preview.manifest} label="Rendered manifest" />
        </Card>
      )}

      <ConfirmDialog
        open={confirm}
        title={`Upgrade ${release.name}?`}
        confirmLabel="Upgrade"
        onClose={() => setConfirm(false)}
        onConfirm={async () => {
          const res = await helm.upgrade(release.namespace, release.name, request)
          onDone(`Upgraded to revision ${res.release?.revision ?? release.revision + 1}. If something is wrong, roll back from the History tab.`)
        }}
      >
        {newChart
          ? `This moves the release from chart ${release.chartVersion} to ${newChart.version} and applies your values.`
          : 'This applies your values as a new revision.'}{' '}
        Kubernetes then updates the affected resources. You can roll back to revision {release.revision} at any time.
      </ConfirmDialog>
    </div>
  )
}

// --- History -----------------------------------------------------------------

function HistoryTab({ release, onDone, canRollback }: { release: ReleaseDetail; onDone: (m: string) => void; canRollback: boolean }) {
  const [target, setTarget] = useState<number | null>(null)
  return (
    <Card title="Revisions" aside={`Newest first · up to 10 kept`} padded={false}>
      <TableScroll>
        <thead>
          <tr>
            <th className={`${TH} ${NUM}`}>Revision</th>
            <th className={TH}>Status</th>
            <th className={TH}>Chart</th>
            <th className={TH}>App version</th>
            <th className={TH}>Description</th>
            <th className={`${TH} ${NUM}`}>Updated</th>
            <th className={TH}>
              <span className="sr-only">Action</span>
            </th>
          </tr>
        </thead>
        <tbody>
          {release.history.map((h) => (
            <tr key={h.revision} className={ROW}>
              <td className={`${TD} ${NUM} font-medium`}>{h.revision}</td>
              <td className={TD}>
                <Status level={releaseLevel(h.status)}>{h.status}</Status>
              </td>
              <td className={`${TD} whitespace-nowrap`}>{h.chartVersion}</td>
              <td className={`${TD} text-fg-2`}>{h.appVersion || '—'}</td>
              <td className={`${TD} text-fg-2`}>{h.description}</td>
              <td className={`${TD} ${NUM} whitespace-nowrap text-fg-2`}>{formatAge(h.updated)} ago</td>
              <td className={`${TD} text-right`}>
                {h.revision === release.revision ? (
                  <span className="text-sm text-muted">Current</span>
                ) : !canRollback ? null : (
                  <Button icon={<RotateCcw size={14} />} onClick={() => setTarget(h.revision)}>
                    Roll back
                  </Button>
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </TableScroll>
      <ConfirmDialog
        open={target != null}
        title={`Roll back to revision ${target}?`}
        confirmLabel="Roll back"
        onClose={() => setTarget(null)}
        onConfirm={async () => {
          if (target == null) return
          await helm.rollback(release.namespace, release.name, target)
          onDone(`Rolled back to revision ${target}. This was recorded as a new revision, so you can undo it the same way.`)
        }}
      >
        <span className="flex items-start gap-2">
          <History size={16} className="mt-1 shrink-0" aria-hidden="true" />
          The release returns to the chart and values of revision {target}. Helm records this as revision {release.revision + 1}.
        </span>
      </ConfirmDialog>
    </Card>
  )
}

// --- Uninstall ---------------------------------------------------------------

function UninstallDialog({ open, release, onClose, onDone }: { open: boolean; release: ReleaseDetail; onClose: () => void; onDone: () => void }) {
  const [typed, setTyped] = useState('')
  return (
    <ConfirmDialog
      open={open}
      title={`Uninstall ${release.name}?`}
      confirmLabel="Uninstall"
      danger
      onClose={() => {
        setTyped('')
        onClose()
      }}
      onConfirm={async () => {
        if (typed !== release.name) throw new Error(`Type ${release.name} to confirm.`)
        await helm.uninstall(release.namespace, release.name)
        onDone()
      }}
    >
      <p>
        This deletes all {plural(release.resources.length, 'resource')} the release created in {release.namespace}, and its
        history. Data in persistent volumes may be lost. This cannot be undone.
      </p>
      <label className="mt-4 block">
        <span className="text-sm text-fg-2">
          Type <span className="font-mono font-medium text-fg">{release.name}</span> to confirm
        </span>
        <input className="input mt-1 w-full" value={typed} onChange={(e) => setTyped(e.target.value)} autoComplete="off" />
      </label>
    </ConfirmDialog>
  )
}
