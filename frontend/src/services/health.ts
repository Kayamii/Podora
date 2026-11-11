import { msSince, percent } from './format'
import { href } from '../hooks/useRoute'
import type { Overview, PodSummary, WorkloadSummary } from '../types/api'

/** How a resource is doing. `neutral` is for states that are neither. */
export type Level = 'normal' | 'warning' | 'fault' | 'neutral'

export interface Issue {
  level: Level
  text: string
  /** Where to look into it. */
  to: string
}

export interface Assessment {
  level: Level
  /** Every problem across the cluster, most severe first. */
  issues: Issue[]
}

/** Usage thresholds, as a share of allocatable. */
const WARN_AT = 75
const FAULT_AT = 90
/** Warning events older than this no longer count as a current problem. */
export const RECENT_EVENT_MS = 15 * 60 * 1000

const RANK: Record<Level, number> = { neutral: 0, normal: 0, warning: 1, fault: 2 }

export function usageLevel(pct: number): Level {
  if (pct >= FAULT_AT) return 'fault'
  if (pct >= WARN_AT) return 'warning'
  return 'normal'
}

export function plural(n: number, one: string, many: string = `${one}s`): string {
  return `${n} ${n === 1 ? one : many}`
}

/** A pod that is still starting is a warning; anything else unhealthy is a fault. */
export function podLevel(p: Pick<PodSummary, 'healthy' | 'status'>): Level {
  if (p.healthy) return p.status === 'Completed' ? 'neutral' : 'normal'
  const starting = ['Pending', 'ContainerCreating', 'PodInitializing', 'Running', 'Terminating']
  return starting.includes(p.status) ? 'warning' : 'fault'
}

export function workloadStatus(w: Pick<WorkloadSummary, 'desired' | 'ready' | 'healthy'>): { level: Level; label: string } {
  if (w.desired === 0) return { level: 'neutral', label: 'Scaled to zero' }
  if (w.healthy) return { level: 'normal', label: 'Ready' }
  return { level: w.ready === 0 ? 'fault' : 'warning', label: w.ready === 0 ? 'Unavailable' : 'Degraded' }
}

/** Turns the overview into one verdict and a list of things to look at. */
export function assess(o: Overview): Assessment {
  const links = {
    nodes: href.nodes,
    pods: href.pods(),
    workloads: href.workloads(),
    namespaces: href.namespaces,
    events: href.events(),
    overview: href.overview,
  }
  const issues: Issue[] = []
  const add = (level: Level, text: string, to: string) => issues.push({ level, text, to })

  const notReady = o.nodes.filter((n) => !n.ready).length
  const cordoned = o.nodes.filter((n) => n.ready && !n.schedulable).length
  if (notReady > 0) add('fault', `${plural(notReady, 'node')} not ready`, links.nodes)
  if (cordoned > 0) add('warning', `${plural(cordoned, 'node')} cordoned`, links.nodes)

  const { pods } = o
  if (pods.failed > 0) add('fault', `${plural(pods.failed, 'pod')} failed`, links.pods)
  if (pods.pending > 0) add('warning', `${plural(pods.pending, 'pod')} pending`, links.pods)
  if (pods.unknown > 0) add('warning', `${plural(pods.unknown, 'pod')} in an unknown state`, links.pods)

  const w = o.workloads
  const notReadyWorkloads =
    w.deployments.total - w.deployments.ready + (w.statefulSets.total - w.statefulSets.ready) + (w.daemonSets.total - w.daemonSets.ready)
  if (notReadyWorkloads > 0) add('warning', `${plural(notReadyWorkloads, 'workload')} not fully ready`, links.workloads)
  if (w.jobs.failed > 0) add('warning', `${plural(w.jobs.failed, 'job')} failed`, links.workloads)

  const unbound = o.storage.persistentVolumeClaims - o.storage.boundClaims
  if (unbound > 0) add('warning', `${plural(unbound, 'volume claim')} not bound`, links.overview)

  const terminating = o.namespaces.filter((ns) => ns.status !== 'Active').length
  if (terminating > 0) add('warning', `${plural(terminating, 'namespace')} terminating`, links.namespaces)

  const { cpu, memory } = o.resources
  for (const [name, m] of [['CPU', cpu], ['Memory', memory]] as const) {
    if (m.usage == null) continue
    const pct = percent(m.usage, m.allocatable)
    const level = usageLevel(pct)
    if (level !== 'normal') add(level, `${name} usage at ${Math.round(pct)}%`, links.nodes)
  }

  const now = new Date(o.generatedAt).getTime()
  const recent = o.events.filter((e) => e.type === 'Warning' && msSince(e.lastSeen, now) <= RECENT_EVENT_MS).length
  if (recent > 0) add('warning', `${plural(recent, 'warning event')} in the last 15 minutes`, links.events)

  issues.sort((a, b) => RANK[b.level] - RANK[a.level])
  return { level: issues.length === 0 ? 'normal' : issues[0].level, issues }
}

/** How a Helm release status reads: deployed is healthy, failed is a fault. */
export function releaseLevel(status: string): Level {
  if (status === 'deployed') return 'normal'
  if (status === 'failed') return 'fault'
  if (status.startsWith('pending') || status === 'uninstalling') return 'warning'
  return 'neutral'
}
