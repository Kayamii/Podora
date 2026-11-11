import { useRef, useState } from 'react'
import type { DragEvent, ReactNode } from 'react'
import { FileArchive, Upload } from 'lucide-react'
import { helm } from '../services/api'
import type { ChartInfo } from '../types/api'
import { Button, Field, Segmented } from './ui'

type Source = 'upload' | 'repo' | 'oci'

interface Props {
  /** Called once a chart is uploaded or downloaded and ready to use. */
  onChart: (chart: ChartInfo) => void
}

/** Gets a chart from an uploaded archive, an HTTP repository or an OCI registry. */
export function ChartSource({ onChart }: Props) {
  const [source, setSource] = useState<Source>('upload')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [dragging, setDragging] = useState(false)
  const [repoURL, setRepoURL] = useState('')
  const [name, setName] = useState('')
  const [ociRef, setOciRef] = useState('')
  const [version, setVersion] = useState('')
  const fileInput = useRef<HTMLInputElement>(null)

  async function run(load: () => Promise<ChartInfo>) {
    setBusy(true)
    setError(null)
    try {
      onChart(await load())
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }

  function upload(file: File | undefined) {
    if (!file) return
    if (!/\.(tgz|tar\.gz)$/i.test(file.name)) {
      setError('Choose a packaged chart: a .tgz file made with "helm package", or downloaded from a chart repository.')
      return
    }
    void run(() => helm.uploadChart(file))
  }

  function onDrop(e: DragEvent) {
    e.preventDefault()
    setDragging(false)
    upload(e.dataTransfer.files[0])
  }

  return (
    <div>
      <Segmented
        label="Chart source"
        value={source}
        onChange={(s) => {
          setSource(s)
          setError(null)
        }}
        options={[
          { value: 'upload', label: 'Upload a chart' },
          { value: 'repo', label: 'Chart repository' },
          { value: 'oci', label: 'OCI registry' },
        ]}
      />

      <div className="mt-5">
        {source === 'upload' && (
          <div
            onDragOver={(e) => {
              e.preventDefault()
              setDragging(true)
            }}
            onDragLeave={() => setDragging(false)}
            onDrop={onDrop}
            className={`flex flex-col items-center rounded-lg border-2 border-dashed px-6 py-10 text-center transition-colors ${
              dragging ? 'border-accent bg-accent-soft' : 'border-line-strong'
            }`}
          >
            <div className="mb-3 flex size-11 items-center justify-center rounded-full bg-subtle text-muted">
              <FileArchive size={22} aria-hidden="true" />
            </div>
            <p className="font-medium">{busy ? 'Reading chart…' : 'Drop a chart archive here'}</p>
            <p className="mt-1 text-sm text-fg-2">A .tgz file, up to 20 MB</p>
            <Button className="mt-4" icon={<Upload size={15} />} disabled={busy} onClick={() => fileInput.current?.click()}>
              Choose file
            </Button>
            <input
              ref={fileInput}
              type="file"
              accept=".tgz,.gz,application/gzip"
              className="sr-only"
              tabIndex={-1}
              onChange={(e) => {
                upload(e.target.files?.[0])
                e.target.value = ''
              }}
            />
          </div>
        )}

        {source === 'repo' && (
          <form
            className="flex flex-wrap items-end gap-3"
            onSubmit={(e) => {
              e.preventDefault()
              void run(() => helm.fetchChart(repoURL, name, version))
            }}
          >
            <Field label="Repository URL">
              <input className="input w-80 max-w-full" required value={repoURL} onChange={(e) => setRepoURL(e.target.value)} placeholder="https://charts.example.com" />
            </Field>
            <Field label="Chart">
              <input className="input w-48" required value={name} onChange={(e) => setName(e.target.value)} placeholder="nginx" />
            </Field>
            <Field label="Version (optional)">
              <input className="input w-36" value={version} onChange={(e) => setVersion(e.target.value)} placeholder="Latest" />
            </Field>
            <Button type="submit" variant="primary" disabled={busy}>
              {busy ? 'Downloading…' : 'Get chart'}
            </Button>
          </form>
        )}

        {source === 'oci' && (
          <form
            className="flex flex-wrap items-end gap-3"
            onSubmit={(e) => {
              e.preventDefault()
              void run(() => helm.fetchChart('', ociRef, version))
            }}
          >
            <Field label="Chart reference">
              <input
                className="input w-96 max-w-full"
                required
                pattern="oci://.+"
                title="Starts with oci://"
                value={ociRef}
                onChange={(e) => setOciRef(e.target.value)}
                placeholder="oci://registry.example.com/charts/app"
              />
            </Field>
            <Field label="Version (optional)">
              <input className="input w-36" value={version} onChange={(e) => setVersion(e.target.value)} placeholder="Latest" />
            </Field>
            <Button type="submit" variant="primary" disabled={busy}>
              {busy ? 'Downloading…' : 'Get chart'}
            </Button>
          </form>
        )}
      </div>

      {error && (
        <p role="alert" className="mt-4 rounded-md bg-danger-soft px-3 py-2 text-sm text-danger-fg">
          {error}
        </p>
      )}
    </div>
  )
}

/** A compact description of a chart that is ready to install. */
export function ChartCard({ chart, action }: { chart: ChartInfo; action?: ReactNode }) {
  return (
    <div className="flex items-start gap-4 rounded-lg border border-line bg-subtle p-4">
      <ChartIcon src={chart.icon} />
      <div className="min-w-0 flex-1">
        <p className="font-semibold">
          {chart.name} <span className="font-normal text-fg-2">{chart.version}</span>
        </p>
        {chart.appVersion && <p className="text-sm text-muted">App version {chart.appVersion}</p>}
        {chart.description && <p className="mt-1 text-sm text-fg-2">{chart.description}</p>}
      </div>
      {action}
    </div>
  )
}

/** The chart's own icon when it has one, otherwise a neutral package mark. */
export function ChartIcon({ src, size = 40 }: { src: string; size?: number }) {
  const [failed, setFailed] = useState(false)
  if (src && !failed) {
    return (
      <img
        src={src}
        alt=""
        width={size}
        height={size}
        onError={() => setFailed(true)}
        className="shrink-0 rounded-md bg-surface object-contain p-1"
        style={{ width: size, height: size }}
      />
    )
  }
  return (
    <div
      aria-hidden="true"
      className="flex shrink-0 items-center justify-center rounded-md bg-accent-soft text-accent"
      style={{ width: size, height: size }}
    >
      <FileArchive size={size * 0.5} />
    </div>
  )
}
