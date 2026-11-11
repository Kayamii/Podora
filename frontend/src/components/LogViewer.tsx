import { useLayoutEffect, useMemo, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import { ArrowDown, ChevronDown, ChevronUp, Download, RotateCw } from 'lucide-react'
import { useLogStream } from '../hooks/useLogStream'
import type { StreamPhase } from '../hooks/useLogStream'
import { logsUrl } from '../services/api'
import type { Level } from '../services/health'
import { parseLines } from '../services/logLevel'
import type { LogLevel, ParsedLine } from '../services/logLevel'
import type { ContainerSummary } from '../types/api'
import { Button, Field, Segmented, Status, Switch } from './ui'

const LINE_OPTIONS = [50, 100, 150, 200, 500, 1000] as const
type Source = 'live' | 'previous'
type LevelFilter = 'all' | 'warn' | 'error'

interface Props {
  namespace: string
  pod: string
  containers: ContainerSummary[]
}

/** A container's log: followed live, or the last lines of its previous run. */
export function LogViewer({ namespace, pod, containers }: Props) {
  const [container, setContainer] = useState(() => (containers.find((c) => !c.init) ?? containers[0])?.name ?? '')
  const [source, setSource] = useState<Source>('live')
  const [tail, setTail] = useState<number>(100)
  const [timestamps, setTimestamps] = useState(false)
  const [wrap, setWrap] = useState(true)
  const [filter, setFilter] = useState('')
  const [levels, setLevels] = useState<LevelFilter>('all')
  // Bumped to reopen a stream that ended or failed.
  const [attempt, setAttempt] = useState(0)

  const current = containers.find((c) => c.name === container)
  const hasPrevious = current?.lastTermination != null
  const url = logsUrl(namespace, pod, {
    container,
    tail,
    previous: source === 'previous',
    follow: source === 'live',
    timestamps,
  })

  if (container === '') return <p className="p-5 text-fg-2">This pod has no containers to read logs from.</p>

  return (
    <div>
      <div className="flex flex-wrap items-end gap-x-4 gap-y-3 border-b border-line px-5 py-4">
        {containers.length > 1 && (
          <Field label="Container">
            <select className="input" value={container} onChange={(e) => setContainer(e.target.value)}>
              {containers.map((c) => (
                <option key={c.name} value={c.name}>
                  {c.name}
                  {c.init ? ' (init)' : ''}
                </option>
              ))}
            </select>
          </Field>
        )}
        <Field label="Source">
          <Segmented
            label="Log source"
            value={source}
            onChange={setSource}
            options={[
              { value: 'live', label: 'Live' },
              {
                value: 'previous',
                label: 'Previous run',
                disabled: !hasPrevious,
                title: hasPrevious ? undefined : 'This container has not restarted, so there is no previous run.',
              },
            ]}
          />
        </Field>
        <Field label="Lines">
          <Segmented
            label="Number of lines"
            value={tail}
            onChange={setTail}
            options={LINE_OPTIONS.map((n) => ({ value: n, label: String(n) }))}
          />
        </Field>
        <Field label="Level">
          <Segmented
            label="Minimum level"
            value={levels}
            onChange={setLevels}
            options={[
              { value: 'all', label: 'All' },
              { value: 'warn', label: 'Warnings +' },
              { value: 'error', label: 'Errors' },
            ]}
          />
        </Field>
        <Field label="Search">
          <input
            type="search"
            className="input w-48"
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
            placeholder="Text in the line"
          />
        </Field>
        <div className="flex gap-4">
          <Switch checked={timestamps} onChange={setTimestamps}>
            Timestamps
          </Switch>
          <Switch checked={wrap} onChange={setWrap}>
            Wrap
          </Switch>
        </div>
      </div>

      <LogPane
        // A new key opens a new stream and drops the old lines.
        key={`${url}#${attempt}`}
        url={url}
        source={source}
        tail={tail}
        wrap={wrap}
        filter={filter}
        levels={levels}
        fileName={`${pod}-${container}${source === 'previous' ? '-previous' : ''}.log`}
        onReconnect={() => setAttempt((n) => n + 1)}
      />
    </div>
  )
}

interface PaneProps {
  url: string
  source: Source
  tail: number
  wrap: boolean
  filter: string
  levels: LevelFilter
  fileName: string
  onReconnect: () => void
}

const RANK: Record<LogLevel, number> = { debug: 0, info: 1, warn: 2, error: 3 }
const MIN_RANK: Record<LevelFilter, number> = { all: 0, warn: 2, error: 3 }

const LINE_STYLE: Record<LogLevel, string> = {
  error: 'bg-[rgb(239_68_68/0.14)] text-[#ffb4ab] shadow-[inset_2px_0_0_#ef4444]',
  warn: 'bg-[rgb(234_179_8/0.11)] text-[#f5d27a] shadow-[inset_2px_0_0_#eab308]',
  info: '',
  debug: 'text-[#8b93a1]',
}

function LogPane({ url, source, tail, wrap, filter, levels, fileName, onReconnect }: PaneProps) {
  const { lines, phase, error, truncated } = useLogStream(url)
  const scroller = useRef<HTMLDivElement>(null)
  // Follow the newest line unless the reader has scrolled up to read.
  const pinned = useRef(true)
  const [showJump, setShowJump] = useState(false)
  const [cursor, setCursor] = useState(-1)

  const parsed = useMemo(() => parseLines(lines), [lines])
  const needle = filter.trim().toLowerCase()
  const shown = useMemo(
    () =>
      parsed
        .map((line, index) => ({ ...line, index }))
        .filter((l) => RANK[l.level] >= MIN_RANK[levels] && (needle === '' || l.text.toLowerCase().includes(needle))),
    [parsed, levels, needle],
  )
  const counts = useMemo(() => {
    let errors = 0
    let warnings = 0
    for (const l of parsed) {
      if (l.level === 'error') errors++
      else if (l.level === 'warn') warnings++
    }
    return { errors, warnings }
  }, [parsed])
  // Positions (within `shown`) of the error lines, for previous/next.
  const errorPositions = useMemo(
    () => shown.flatMap((l, i) => (l.level === 'error' ? [i] : [])),
    [shown],
  )

  useLayoutEffect(() => {
    const el = scroller.current
    if (el && pinned.current) el.scrollTop = el.scrollHeight
  }, [shown.length])

  function onScroll() {
    const el = scroller.current
    if (!el) return
    const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 24
    pinned.current = atBottom
    setShowJump(!atBottom)
  }

  function jumpToLatest() {
    const el = scroller.current
    if (!el) return
    pinned.current = true
    el.scrollTop = el.scrollHeight
    setShowJump(false)
    setCursor(-1)
  }

  /** Moves to the previous (-1) or next (+1) error line. */
  function stepError(direction: 1 | -1) {
    if (errorPositions.length === 0) return
    const currentIdx = errorPositions.indexOf(cursor)
    const nextIdx =
      currentIdx === -1
        ? direction === 1
          ? 0
          : errorPositions.length - 1
        : (currentIdx + direction + errorPositions.length) % errorPositions.length
    const pos = errorPositions[nextIdx]
    setCursor(pos)
    pinned.current = false
    scroller.current?.querySelector(`[data-pos="${pos}"]`)?.scrollIntoView({ block: 'center' })
  }

  function download() {
    const blob = new Blob([lines.join('\n') + '\n'], { type: 'text/plain' })
    const a = document.createElement('a')
    a.href = URL.createObjectURL(blob)
    a.download = fileName
    a.click()
    URL.revokeObjectURL(a.href)
  }

  const status = describe(phase, source, tail, lines.length, error)
  const filtered = levels !== 'all' || needle !== ''

  return (
    <div>
      <div className="flex flex-wrap items-center justify-between gap-x-6 gap-y-2 px-5 py-2.5">
        <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-sm text-fg-2" role="status">
          <Status level={status.level}>{status.text}</Status>
          {lines.length > 0 && (
            <span className="flex items-center gap-3">
              <span className={counts.errors > 0 ? 'font-medium text-danger-fg' : 'text-muted'}>
                {counts.errors} {counts.errors === 1 ? 'error' : 'errors'}
              </span>
              <span className={counts.warnings > 0 ? 'font-medium text-warn-fg' : 'text-muted'}>
                {counts.warnings} {counts.warnings === 1 ? 'warning' : 'warnings'}
              </span>
              {filtered && <span className="text-muted">· {shown.length} shown</span>}
            </span>
          )}
        </div>
        <div className="flex gap-2">
          {errorPositions.length > 0 && (
            <div className="flex">
              <Button
                className="rounded-r-none"
                icon={<ChevronUp size={15} />}
                onClick={() => stepError(-1)}
                aria-label="Previous error"
                title="Previous error"
              />
              <Button
                className="-ml-px rounded-l-none"
                icon={<ChevronDown size={15} />}
                onClick={() => stepError(1)}
                aria-label="Next error"
                title="Next error"
              >
                Errors
              </Button>
            </div>
          )}
          {((phase === 'ended' && source === 'live') || phase === 'error') && (
            <Button icon={<RotateCw size={15} />} onClick={onReconnect}>
              {phase === 'error' ? 'Try again' : 'Reconnect'}
            </Button>
          )}
          <Button icon={<Download size={15} />} onClick={download} disabled={lines.length === 0}>
            Download
          </Button>
        </div>
      </div>

      <div className="relative">
        <div
          ref={scroller}
          onScroll={onScroll}
          tabIndex={0}
          role="log"
          aria-label="Container log"
          className="h-[min(65vh,44rem)] min-h-72 overflow-auto rounded-b-lg bg-code-bg py-3 font-mono text-[0.8125rem] leading-[1.6] text-code-fg"
        >
          {truncated && <p className="mb-2 px-5 font-sans text-sm opacity-70">Older lines were dropped to keep the page fast.</p>}
          {shown.length === 0 ? (
            <p className="px-5 font-sans text-sm opacity-70">
              {lines.length > 0 ? 'No lines match the level and search filters.' : status.empty}
            </p>
          ) : (
            shown.map((line, pos) => (
              <LogLine key={line.index} line={line} pos={pos} wrap={wrap} needle={needle} current={pos === cursor} />
            ))
          )}
        </div>
        {showJump && (
          <div className="absolute right-5 bottom-4">
            <Button variant="primary" icon={<ArrowDown size={15} />} onClick={jumpToLatest}>
              Jump to latest
            </Button>
          </div>
        )}
      </div>
    </div>
  )
}

function LogLine({ line, pos, wrap, needle, current }: { line: ParsedLine; pos: number; wrap: boolean; needle: string; current: boolean }) {
  return (
    <div
      data-pos={pos}
      className={`px-5 ${wrap ? 'break-words whitespace-pre-wrap' : 'whitespace-pre'} ${LINE_STYLE[line.level]} ${
        current ? 'outline outline-1 -outline-offset-1 outline-[#ef4444]' : ''
      }`}
    >
      {line.timestamp && <span className="mr-3 text-[#6b7280] select-none">{line.timestamp}</span>}
      {line.text === '' ? ' ' : highlight(line.text, needle)}
    </div>
  )
}

/** Marks every occurrence of the search text. */
function highlight(text: string, needle: string): ReactNode {
  if (!needle) return text
  const lower = text.toLowerCase()
  const parts: ReactNode[] = []
  let from = 0
  for (let at = lower.indexOf(needle); at !== -1; at = lower.indexOf(needle, from)) {
    if (at > from) parts.push(text.slice(from, at))
    parts.push(
      <mark key={at} className="rounded-sm bg-[#facc15] px-px text-[#111]">
        {text.slice(at, at + needle.length)}
      </mark>,
    )
    from = at + needle.length
  }
  parts.push(text.slice(from))
  return parts
}

interface StreamStatus {
  level: Level
  text: string
  /** Shown inside the pane while it has no lines. */
  empty: string
}

function describe(phase: StreamPhase, source: Source, tail: number, count: number, error: string | null): StreamStatus {
  const lineCount = `${count} ${count === 1 ? 'line' : 'lines'}`
  if (phase === 'error') return { level: 'fault', text: explain(error), empty: 'No log to show.' }
  if (phase === 'connecting') return { level: 'neutral', text: 'Opening log…', empty: 'Opening log…' }
  if (source === 'previous') {
    return { level: 'neutral', text: `Previous run · last ${tail} requested · ${lineCount}`, empty: 'The previous run wrote no log lines.' }
  }
  if (phase === 'open') {
    return { level: 'normal', text: `Streaming live · ${lineCount}`, empty: 'Connected. The container has not written anything yet.' }
  }
  return {
    level: 'warning',
    text: `Stream closed · ${lineCount}. The container may have stopped or restarted.`,
    empty: 'The stream closed before any line was written.',
  }
}

/** Turns the cluster's wording for common log failures into plain words. */
function explain(error: string | null): string {
  const raw = error ?? 'Unknown error'
  if (raw.includes('previous terminated container')) return 'This container has not restarted, so it has no previous run.'
  if (raw.includes('is waiting to start') || raw.includes('ContainerCreating') || raw.includes('PodInitializing')) {
    return 'The container has not started yet, so it has no log.'
  }
  if (raw.includes('not found')) return 'The pod or container no longer exists.'
  return `Could not read the log: ${raw}`
}
