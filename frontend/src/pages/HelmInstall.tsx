import { useState } from 'react'
import { Eye, Rocket } from 'lucide-react'
import { ChartCard, ChartSource } from '../components/ChartSource'
import { CodeBlock } from '../components/CodeBlock'
import { ValuesEditor } from '../components/ValuesEditor'
import { Button, Card, Field, Notice, PageHeader } from '../components/ui'
import { usePolling } from '../hooks/usePolling'
import { useSession } from '../hooks/session'
import { href, navigate } from '../hooks/useRoute'
import { api, helm } from '../services/api'
import type { ChartInfo, Rendered } from '../types/api'

/** Release names must be valid Kubernetes object names, and Helm caps them at 53. */
const NAME = /^[a-z0-9]([-a-z0-9]*[a-z0-9])?$/

export function HelmInstall() {
  const { can } = useSession()
  const [chart, setChart] = useState<ChartInfo | null>(null)

  if (!can('helm.manage')) {
    return (
      <div>
        <PageHeader title="Install a chart" breadcrumb={[{ label: 'Helm releases', to: href.helm }]} />
        <Notice level="warning">
          Installing charts needs the Operator or Administrator role with access to all namespaces, because a chart can create
          resources anywhere in the cluster.
        </Notice>
      </div>
    )
  }

  return (
    <div>
      <PageHeader
        title="Install a chart"
        breadcrumb={[{ label: 'Helm releases', to: href.helm }]}
        subtitle="Choose a chart, review its values, then install. Nothing is deployed until you press Install."
      />
      <Card title="1. Chart">
        {chart ? (
          <ChartCard chart={chart} action={<Button onClick={() => setChart(null)}>Change</Button>} />
        ) : (
          <ChartSource onChart={setChart} />
        )}
      </Card>
      {/* Keyed by chart so switching charts starts from that chart's values. */}
      {chart && <Configure key={chart.id} chart={chart} />}
    </div>
  )
}

function Configure({ chart }: { chart: ChartInfo }) {
  const namespaces = usePolling(api.namespaces, 30_000)
  const [name, setName] = useState(chart.name.toLowerCase().replace(/[^a-z0-9-]/g, '-').slice(0, 53))
  const [namespace, setNamespace] = useState('default')
  const [values, setValues] = useState(chart.values)
  const [busy, setBusy] = useState<'preview' | 'install' | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [preview, setPreview] = useState<Rendered | null>(null)

  const existing = (namespaces.data ?? []).map((n) => n.name)
  const isNew = namespaces.data != null && !existing.includes(namespace)
  const nameError = name && (!NAME.test(name) || name.length > 53) ? 'Use lowercase letters, digits and dashes (at most 53), starting and ending with a letter or digit.' : null
  const nsError = namespace && !NAME.test(namespace) ? 'Use lowercase letters, digits and dashes.' : null
  const ready = name !== '' && namespace !== '' && !nameError && !nsError

  async function submit(dryRun: boolean) {
    setBusy(dryRun ? 'preview' : 'install')
    setError(null)
    try {
      const res = await helm.install({ chartId: chart.id, name, namespace, createNamespace: isNew, values, dryRun })
      if (dryRun) {
        setPreview(res.rendered ?? null)
      } else {
        navigate(href.helmRelease(namespace, name))
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(null)
    }
  }

  return (
    <>
      <Card title="2. Release" className="mt-6">
        <div className="flex flex-wrap gap-x-6 gap-y-4">
          <div>
            <Field label="Release name">
              <input className="input w-72" value={name} onChange={(e) => setName(e.target.value.trim())} required />
            </Field>
            {nameError && <p className="mt-1 max-w-72 text-sm text-danger-fg">{nameError}</p>}
          </div>
          <div>
            <Field label="Namespace">
              <input
                className="input w-72"
                list="namespace-options"
                value={namespace}
                onChange={(e) => setNamespace(e.target.value.trim())}
                required
              />
            </Field>
            <datalist id="namespace-options">
              {existing.map((n) => (
                <option key={n} value={n} />
              ))}
            </datalist>
            {nsError ? (
              <p className="mt-1 text-sm text-danger-fg">{nsError}</p>
            ) : (
              isNew && namespace && <p className="mt-1 text-sm text-fg-2">This namespace does not exist yet; it will be created.</p>
            )}
          </div>
        </div>
      </Card>

      <Card
        title="3. Values"
        aside={
          values !== chart.values && (
            <button type="button" className="cursor-pointer text-accent hover:underline" onClick={() => setValues(chart.values)}>
              Reset to chart defaults
            </button>
          )
        }
        className="mt-6"
      >
        <p className="mb-3 text-fg-2">
          These are the chart's settings from its values.yaml. Change what you need; anything you leave keeps the chart's default.
        </p>
        <ValuesEditor label="Values (YAML)" value={values} onChange={setValues} />
      </Card>

      {error && (
        <div className="mt-6">
          <Notice level="fault">{error}</Notice>
        </div>
      )}

      <div className="sticky bottom-0 z-10 mt-6 -mx-4 flex flex-wrap items-center justify-end gap-3 border-t border-line bg-canvas/90 px-4 py-4 backdrop-blur sm:-mx-6 sm:px-6 lg:-mx-8 lg:px-8">
        <span className="mr-auto text-sm text-fg-2">
          {chart.name} {chart.version} → <span className="font-medium text-fg">{namespace || '…'}/{name || '…'}</span>
        </span>
        <Button icon={<Eye size={15} />} disabled={!ready || busy != null} onClick={() => submit(true)}>
          {busy === 'preview' ? 'Rendering…' : 'Preview manifest'}
        </Button>
        <Button variant="primary" icon={<Rocket size={15} />} disabled={!ready || busy != null} onClick={() => submit(false)}>
          {busy === 'install' ? 'Installing…' : 'Install'}
        </Button>
      </div>

      {preview && (
        <Card title="Preview" aside="What Install would create · nothing has been applied" className="mt-6">
          <CodeBlock code={preview.manifest} label="Rendered manifest" />
          {preview.notes && (
            <>
              <h3 className="mt-5 mb-2 font-semibold">Notes</h3>
              <pre className="font-mono text-sm whitespace-pre-wrap text-fg-2">{preview.notes}</pre>
            </>
          )}
        </Card>
      )}
    </>
  )
}
