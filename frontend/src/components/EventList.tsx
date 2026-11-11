import { href } from '../hooks/useRoute'
import { formatAge, msSince } from '../services/format'
import { RECENT_EVENT_MS } from '../services/health'
import type { EventSummary } from '../types/api'
import { Status } from './ui'

interface Props {
  events: EventSummary[]
  /** When the data was read; ages are relative to it. Defaults to the present. */
  now?: number
  /** Hide the object each event is about, e.g. on that object's own page. */
  hideObject?: boolean
}

const WORKLOAD_KINDS = ['Deployment', 'StatefulSet', 'DaemonSet'] as const

/** Where an event's object lives in Podora, if it has a page. */
function objectLink(e: EventSummary): string | null {
  const [kind, name] = e.object.split('/')
  if (!e.namespace || !name) return null
  if (kind === 'Pod') return href.pod(e.namespace, name)
  const workload = WORKLOAD_KINDS.find((k) => k === kind)
  return workload ? href.workload(workload, e.namespace, name) : null
}

/** Cluster events, newest first. Recent warnings are marked; old ones fade to neutral. */
export function EventList({ events, now, hideObject = false }: Props) {
  return (
    <ul>
      {events.map((e, i) => {
        const warning = e.type === 'Warning'
        const fresh = msSince(e.lastSeen, now) <= RECENT_EVENT_MS
        const link = objectLink(e)
        return (
          <li key={i} className="grid gap-x-5 gap-y-1 border-t border-line px-5 py-3 first:border-t-0 sm:grid-cols-[4.5rem_1fr]">
            <span className="text-sm whitespace-nowrap text-muted tabular-nums sm:pt-px">{formatAge(e.lastSeen, now)} ago</span>
            <div className="min-w-0">
              <p className="flex flex-wrap items-center gap-x-3 gap-y-1">
                <span className="font-medium">
                  <Status level={warning ? (fresh ? 'warning' : 'neutral') : 'neutral'}>{e.reason}</Status>
                </span>
                {e.count > 1 && <span className="text-sm text-muted">×{e.count}</span>}
                {!hideObject && (
                  <span className="min-w-0 text-sm break-all text-fg-2">
                    {e.namespace && <span className="text-muted">{e.namespace} / </span>}
                    {link ? (
                      <a href={link} className="text-accent hover:underline">
                        {e.object}
                      </a>
                    ) : (
                      e.object
                    )}
                  </span>
                )}
              </p>
              <p className="mt-0.5 max-w-[80ch] text-sm break-words text-fg-2">{e.message}</p>
            </div>
          </li>
        )
      })}
    </ul>
  )
}
