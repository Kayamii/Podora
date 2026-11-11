import type {
  AuditEntry,
  ChartInfo,
  Me,
  Role,
  User,
  EventSummary,
  HelmOperationResult,
  NamespaceUsage,
  ReleaseDetail,
  ReleaseSummary,
  UsageHistory,
  HealthResponse,
  HistoryResponse,
  IngressSummary,
  NamespaceSummary,
  Overview,
  PodDetail,
  PodSummary,
  ServiceSummary,
  WorkloadDetail,
  WorkloadKind,
  WorkloadSummary,
} from '../types/api'

const BASE = '/api/v1'
const enc = encodeURIComponent

/** Fired when the server says the session is gone; the app shows sign-in. */
export const SIGNED_OUT_EVENT = 'podora:signed-out'

function checkSession(res: Response) {
  if (res.status === 401) window.dispatchEvent(new Event(SIGNED_OUT_EVENT))
}

async function get<T>(path: string, signal?: AbortSignal): Promise<T> {
  const res = await fetch(`${BASE}${path}`, { signal })
  checkSession(res)
  if (!res.ok) {
    throw new Error(await errorMessage(res))
  }
  return (await res.json()) as T
}

/** The backend refuses changes without this header; see api/resources.go. */
const CHANGE_HEADER = { 'X-Podora-Request': '1' }

/** Sends a request that changes the cluster and returns its JSON answer, if any. */
async function change<T = void>(method: 'POST' | 'DELETE' | 'PATCH', path: string, body?: unknown, quiet401 = false): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    method,
    headers: { ...CHANGE_HEADER, 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  // A wrong password is a 401 too; sign-in calls must not count as expiry.
  if (!quiet401) checkSession(res)
  if (!res.ok) {
    throw new Error(await errorMessage(res))
  }
  return (res.status === 204 ? undefined : await res.json()) as T
}

/** Prefers the backend's `{ "error": "..." }` body over the bare status. */
export async function errorMessage(res: Response): Promise<string> {
  try {
    const body: unknown = await res.json()
    if (body && typeof body === 'object' && 'error' in body && typeof body.error === 'string') {
      return body.error
    }
  } catch {
    // Not JSON (e.g. a proxy error page); fall through to the status line.
  }
  return `Request failed: ${res.status} ${res.statusText}`
}

/** URL segment for a workload kind, e.g. "deployments". */
export function kindPath(kind: WorkloadKind): string {
  return `${kind.toLowerCase()}s`
}

const workloadPath = (kind: WorkloadKind, namespace: string, name: string) =>
  `/namespaces/${enc(namespace)}/workloads/${kindPath(kind)}/${enc(name)}`

export const api = {
  health: (signal?: AbortSignal) => get<HealthResponse>('/health', signal),
  overview: (signal?: AbortSignal) => get<Overview>('/overview', signal),
  history: (signal?: AbortSignal) => get<HistoryResponse>('/metrics/history', signal),
  namespaces: (signal?: AbortSignal) => get<NamespaceSummary[]>('/namespaces', signal),

  // Lists cover every namespace; pages filter in the browser.
  pods: (signal?: AbortSignal) => get<PodSummary[]>('/pods', signal),
  workloads: (signal?: AbortSignal) => get<WorkloadSummary[]>('/workloads', signal),
  services: (signal?: AbortSignal) => get<ServiceSummary[]>('/services', signal),
  ingresses: (signal?: AbortSignal) => get<IngressSummary[]>('/ingresses', signal),
  events: (signal?: AbortSignal) => get<EventSummary[]>('/events', signal),

  pod: (namespace: string, name: string, signal?: AbortSignal) =>
    get<PodDetail>(`/namespaces/${enc(namespace)}/pods/${enc(name)}`, signal),
  workload: (kind: WorkloadKind, namespace: string, name: string, signal?: AbortSignal) =>
    get<WorkloadDetail>(workloadPath(kind, namespace, name), signal),

  namespacesUsage: (signal?: AbortSignal) => get<NamespaceUsage[]>('/namespaces/usage', signal),
  /** Recorded usage of a namespace, or of one pod in it. */
  usageHistory: (namespace: string, pod = '', signal?: AbortSignal) =>
    get<UsageHistory>(`/metrics/usage-history?namespace=${enc(namespace)}${pod ? `&pod=${enc(pod)}` : ''}`, signal),

  deletePod: (namespace: string, name: string) =>
    change('DELETE', `/namespaces/${enc(namespace)}/pods/${enc(name)}`),
  scaleWorkload: (kind: WorkloadKind, namespace: string, name: string, replicas: number) =>
    change('POST', `${workloadPath(kind, namespace, name)}/scale`, { replicas }),
  restartWorkload: (kind: WorkloadKind, namespace: string, name: string) =>
    change('POST', `${workloadPath(kind, namespace, name)}/restart`),
}

const releasePath = (namespace: string, name: string) => `/helm/releases/${enc(namespace)}/${enc(name)}`

export const helm = {
  releases: (signal?: AbortSignal) => get<ReleaseSummary[]>('/helm/releases', signal),
  release: (namespace: string, name: string, signal?: AbortSignal) => get<ReleaseDetail>(releasePath(namespace, name), signal),

  /** Uploads a .tgz chart; the answer describes it and holds its values.yaml. */
  async uploadChart(file: File): Promise<ChartInfo> {
    const form = new FormData()
    form.append('chart', file)
    const res = await fetch(`${BASE}/helm/charts`, { method: 'POST', headers: CHANGE_HEADER, body: form })
    checkSession(res)
    if (!res.ok) throw new Error(await errorMessage(res))
    return (await res.json()) as ChartInfo
  },
  /** Downloads a chart from an HTTP repository, or an OCI registry when `chart` is "oci://...". */
  fetchChart: (repoURL: string, chart: string, version: string) =>
    change<ChartInfo>('POST', '/helm/charts/fetch', { repoURL, chart, version }),

  install: (req: { chartId: string; name: string; namespace: string; createNamespace: boolean; values: string; dryRun: boolean }) =>
    change<HelmOperationResult>('POST', '/helm/releases', req),
  upgrade: (namespace: string, name: string, req: { chartId?: string; values: string; dryRun: boolean }) =>
    change<HelmOperationResult>('POST', `${releasePath(namespace, name)}/upgrade`, req),
  rollback: (namespace: string, name: string, revision: number) =>
    change('POST', `${releasePath(namespace, name)}/rollback`, { revision }),
  uninstall: (namespace: string, name: string) => change('DELETE', releasePath(namespace, name)),
}

export interface LogQuery {
  container: string
  /** How many of the most recent lines to start with. */
  tail: number
  /** Read the container's previous run instead of the current one. */
  previous: boolean
  /** Keep the response open and receive new lines as they are written. */
  follow: boolean
  timestamps: boolean
}

/** URL of a pod's plain-text log stream. */
export function logsUrl(namespace: string, name: string, q: LogQuery): string {
  const params = new URLSearchParams({ container: q.container, tail: String(q.tail) })
  if (q.previous) params.set('previous', 'true')
  if (q.follow && !q.previous) params.set('follow', 'true')
  if (q.timestamps) params.set('timestamps', 'true')
  return `${BASE}/namespaces/${enc(namespace)}/pods/${enc(name)}/logs?${params}`
}

export const accounts = {
  status: () => get<{ setupRequired: boolean }>('/auth/status'),
  /** The signed-in user, or null when nobody is signed in. */
  async me(): Promise<Me | null> {
    const res = await fetch(`${BASE}/auth/me`)
    if (res.status === 401) return null
    if (!res.ok) throw new Error(await errorMessage(res))
    return (await res.json()) as Me
  },
  login: (username: string, password: string) => change<Me>('POST', '/auth/login', { username, password }, true),
  setup: (token: string, username: string, password: string) =>
    change<Me>('POST', '/auth/setup', { token, username, password }, true),
  logout: () => change('POST', '/auth/logout', undefined, true),
  changePassword: (current: string, next: string) => change('POST', '/auth/password', { current, new: next }),

  users: (signal?: AbortSignal) => get<User[]>('/users', signal),
  createUser: (u: { username: string; password: string; role: Role; namespaces: string[] }) => change<User>('POST', '/users', u),
  updateUser: (id: number, u: { role?: Role; namespaces?: string[]; disabled?: boolean; password?: string }) =>
    change<User>('PATCH', `/users/${id}`, u),
  deleteUser: (id: number) => change('DELETE', `/users/${id}`),
  audit: (signal?: AbortSignal) => get<AuditEntry[]>('/audit', signal),
}
