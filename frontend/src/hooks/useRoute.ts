import { useSyncExternalStore } from 'react'
import type { WorkloadKind } from '../types/api'

/**
 * Hash-based routing, e.g. `#/pods?namespace=demo` or
 * `#/workloads/demo/deployments/web`. The hash keeps deep links working
 * behind any proxy path without server-side route handling.
 */
export type Route =
  | { page: 'overview' | 'nodes' | 'namespaces' | 'helm' | 'helmInstall' | 'users' | 'audit' }
  | { page: 'namespace'; name: string }
  | { page: 'helmRelease'; namespace: string; name: string }
  | { page: 'workloads' | 'pods' | 'network' | 'events'; namespace: string }
  | { page: 'workload'; kind: WorkloadKind; namespace: string; name: string }
  | { page: 'pod'; namespace: string; name: string }
  | { page: 'notFound' }

const enc = encodeURIComponent
const withNamespace = (path: string, namespace = '') => (namespace ? `${path}?namespace=${enc(namespace)}` : path)

const KIND_SEGMENTS: Record<string, WorkloadKind> = {
  deployments: 'Deployment',
  statefulsets: 'StatefulSet',
  daemonsets: 'DaemonSet',
}

export const href = {
  overview: '#/',
  nodes: '#/nodes',
  namespaces: '#/namespaces',
  workloads: (namespace?: string) => withNamespace('#/workloads', namespace),
  pods: (namespace?: string) => withNamespace('#/pods', namespace),
  network: (namespace?: string) => withNamespace('#/network', namespace),
  events: (namespace?: string) => withNamespace('#/events', namespace),
  workload: (kind: WorkloadKind, namespace: string, name: string) =>
    `#/workloads/${enc(namespace)}/${kind.toLowerCase()}s/${enc(name)}`,
  pod: (namespace: string, name: string) => `#/pods/${enc(namespace)}/${enc(name)}`,
  namespace: (name: string) => `#/namespaces/${enc(name)}`,
  helm: '#/helm',
  users: '#/admin/users',
  audit: '#/admin/audit',
  helmInstall: '#/helm/install',
  helmRelease: (namespace: string, name: string) => `#/helm/releases/${enc(namespace)}/${enc(name)}`,
}

export function parseRoute(hash: string): Route {
  const [path, query = ''] = hash.replace(/^#/, '').split('?')
  const parts = path.split('/').filter(Boolean).map(decodeURIComponent)
  const namespace = new URLSearchParams(query).get('namespace') ?? ''
  const [section] = parts

  if (parts.length === 0) return { page: 'overview' }
  if (parts.length === 1) {
    if (section === 'nodes' || section === 'namespaces') return { page: section }
    if (section === 'workloads' || section === 'pods' || section === 'network' || section === 'events') {
      return { page: section, namespace }
    }
  }
  if (section === 'admin' && parts.length === 2 && (parts[1] === 'users' || parts[1] === 'audit')) return { page: parts[1] }
  if (section === 'namespaces' && parts.length === 2) return { page: 'namespace', name: parts[1] }
  if (section === 'helm') {
    if (parts.length === 1) return { page: 'helm' }
    if (parts.length === 2 && parts[1] === 'install') return { page: 'helmInstall' }
    if (parts.length === 4 && parts[1] === 'releases') return { page: 'helmRelease', namespace: parts[2], name: parts[3] }
  }
  if (section === 'pods' && parts.length === 3) {
    return { page: 'pod', namespace: parts[1], name: parts[2] }
  }
  if (section === 'workloads' && parts.length === 4 && parts[2] in KIND_SEGMENTS) {
    return { page: 'workload', namespace: parts[1], kind: KIND_SEGMENTS[parts[2]], name: parts[3] }
  }
  return { page: 'notFound' }
}

/** A top-level area of the app, as shown in the sidebar. */
export type Section = 'overview' | 'nodes' | 'namespaces' | 'workloads' | 'pods' | 'network' | 'events' | 'helm' | 'users' | 'audit'

/** The sidebar section a route belongs to. */
export function sectionOf(route: Route): Section | null {
  switch (route.page) {
    case 'workload':
      return 'workloads'
    case 'pod':
      return 'pods'
    case 'namespace':
      return 'namespaces'
    case 'helmInstall':
    case 'helmRelease':
      return 'helm'
    case 'notFound':
      return null
    default:
      return route.page
  }
}

function subscribe(onChange: () => void) {
  window.addEventListener('hashchange', onChange)
  return () => window.removeEventListener('hashchange', onChange)
}

/** The current route, re-rendering when the hash changes. */
export function useRoute(): Route {
  const hash = useSyncExternalStore(subscribe, () => window.location.hash)
  return parseRoute(hash)
}

export function navigate(to: string) {
  window.location.hash = to
}
