// Mirrors backend/internal/resources/types.go.

export interface HealthResponse {
  status: string
  version: string
}

export interface Overview {
  generatedAt: string
  cluster: ClusterInfo
  resources: ResourceSummary
  nodes: NodeSummary[]
  pods: PodCounts
  workloads: Workloads
  namespaces: NamespaceSummary[]
  systemPods: PodSummary[]
  network: NetworkSummary
  storage: StorageSummary
  events: EventSummary[]
}

export interface ClusterInfo {
  version: string
  platform: string
  nodesTotal: number
  nodesReady: number
}

/** CPU values are millicores, memory values are bytes. */
export interface Measurement {
  capacity: number
  allocatable: number
  requests: number
  limits: number
  /** null when metrics-server is unavailable. */
  usage: number | null
}

export interface ResourceSummary {
  metricsAvailable: boolean
  cpu: Measurement
  memory: Measurement
  podCapacity: number
}

export interface NodeUsage {
  allocatable: number
  usage: number | null
}

export interface NodeSummary {
  name: string
  ready: boolean
  schedulable: boolean
  roles: string[]
  kubeletVersion: string
  osImage: string
  internalIP: string
  createdAt: string
  pods: number
  podCapacity: number
  cpu: NodeUsage
  memory: NodeUsage
}

export interface PodCounts {
  total: number
  running: number
  pending: number
  succeeded: number
  failed: number
  unknown: number
  restarts: number
}

export interface ReadyCount {
  total: number
  ready: number
}

export interface Workloads {
  deployments: ReadyCount
  statefulSets: ReadyCount
  daemonSets: ReadyCount
  replicaSets: number
  jobs: { total: number; active: number; succeeded: number; failed: number }
  cronJobs: number
}

export interface NamespaceSummary {
  name: string
  status: string
  pods: number
  createdAt: string
}

export interface PodSummary {
  name: string
  namespace: string
  status: string
  healthy: boolean
  readyContainers: number
  containers: number
  restarts: number
  node: string
  ip: string
  createdAt: string
  cpuUsage: number | null
  memoryUsage: number | null
}

export interface NetworkSummary {
  services: number
  servicesByType: Record<string, number>
  ingresses: number
  ingressClasses: number
  networkPolicies: number
  endpointSlices: number
}

export interface StorageSummary {
  persistentVolumes: number
  persistentVolumeClaims: number
  boundClaims: number
  storageClasses: number
  requestedBytes: number
}

export interface EventSummary {
  type: string
  reason: string
  message: string
  namespace: string
  object: string
  count: number
  lastSeen: string
}

/** Cluster-wide usage at one moment. CPU is millicores, memory is bytes. */
export interface UsageSample {
  t: string
  cpuUsage: number
  cpuAllocatable: number
  memoryUsage: number
  memoryAllocatable: number
}

export interface HistoryResponse {
  intervalSeconds: number
  samples: UsageSample[]
}

export interface PodCondition {
  type: string
  status: string
  reason: string
  message: string
}

export interface Termination {
  reason: string
  exitCode: number
  finishedAt: string
}

export interface ContainerSummary {
  name: string
  image: string
  init: boolean
  state: 'Running' | 'Waiting' | 'Terminated' | 'Unknown'
  reason: string
  message: string
  ready: boolean
  restarts: number
  startedAt: string | null
  /** The previous run, present once the container has restarted. */
  lastTermination: Termination | null
  ports: string[]
  /** Current usage; null without metrics-server. */
  cpuUsage: number | null
  memoryUsage: number | null
  cpuRequest: number
  cpuLimit: number
  memoryRequest: number
  memoryLimit: number
}

/** One pod in full. `containers` is the list here, not the count. */
export interface PodDetail extends Omit<PodSummary, 'containers'> {
  phase: string
  qosClass: string
  serviceAccount: string
  hostIP: string
  /** e.g. "ReplicaSet/web-5c7cbdcd5f"; empty when nothing controls the pod. */
  owner: string
  labels: Record<string, string>
  conditions: PodCondition[]
  containers: ContainerSummary[]
  events: EventSummary[]
}

export type WorkloadKind = 'Deployment' | 'StatefulSet' | 'DaemonSet'

export interface WorkloadSummary {
  kind: WorkloadKind
  name: string
  namespace: string
  /** Replicas asked for; for a DaemonSet, the nodes it should run on. */
  desired: number
  ready: number
  updated: number
  available: number
  healthy: boolean
  scalable: boolean
  images: string[]
  createdAt: string
}

export interface WorkloadDetail extends WorkloadSummary {
  strategy: string
  selector: Record<string, string>
  labels: Record<string, string>
  conditions: PodCondition[]
  pods: PodSummary[]
  events: EventSummary[]
}

export interface ServiceSummary {
  name: string
  namespace: string
  type: string
  clusterIP: string
  /** Addresses reachable from outside the cluster. */
  external: string[]
  ports: string[]
  selector: Record<string, string>
  createdAt: string
}

export interface IngressSummary {
  name: string
  namespace: string
  class: string
  hosts: string[]
  addresses: string[]
  tls: boolean
  createdAt: string
}

/** What a namespace is using now. CPU in millicores, memory in bytes. */
export interface NamespaceUsage {
  name: string
  status: string
  pods: number
  runningPods: number
  /** null without metrics-server. */
  cpuUsage: number | null
  memoryUsage: number | null
  cpuRequests: number
  cpuLimits: number
  memoryRequests: number
  memoryLimits: number
  createdAt: string
}

export interface UsagePoint {
  t: string
  cpu: number
  memory: number
}

export interface UsageHistory {
  intervalSeconds: number
  points: UsagePoint[]
}

// --- Helm ---------------------------------------------------------------------

export interface ChartInfo {
  /** Refers to the staged chart in install and upgrade requests. */
  id: string
  name: string
  version: string
  appVersion: string
  description: string
  icon: string
  /** values.yaml as written, comments included. */
  values: string
  readme: string
}

export interface ReleaseSummary {
  name: string
  namespace: string
  revision: number
  status: string
  chart: string
  chartVersion: string
  appVersion: string
  icon: string
  description: string
  updated: string
}

export interface ReleaseResource {
  kind: string
  name: string
  namespace: string
}

export interface ReleaseRevision {
  revision: number
  status: string
  chartVersion: string
  appVersion: string
  description: string
  updated: string
}

export interface ReleaseDetail extends ReleaseSummary {
  firstDeployed: string
  notes: string
  manifest: string
  /** Values set for this release, as YAML. */
  values: string
  /** The chart's defaults, as YAML. */
  defaultValues: string
  resources: ReleaseResource[]
  history: ReleaseRevision[]
  chartInfo: { description: string; home: string; sources: string[] | null }
}

export interface Rendered {
  manifest: string
  notes: string
}

export interface HelmOperationResult {
  release?: ReleaseSummary
  rendered?: Rendered
}

// --- Accounts -----------------------------------------------------------------

export type Role = 'viewer' | 'developer' | 'operator' | 'admin'

export type Permission =
  | 'read'
  | 'workloads.restart'
  | 'pods.delete'
  | 'workloads.scale'
  | 'helm.manage'
  | 'users.manage'
  | 'audit.view'

export interface User {
  id: number
  username: string
  role: Role
  /** Empty means every namespace. */
  namespaces: string[]
  disabled: boolean
  createdAt: string
  lastLogin: string | null
}

export interface Me {
  user: User
  permissions: Permission[]
  allNamespaces: boolean
}

export interface AuditEntry {
  id: number
  time: string
  username: string
  action: string
  namespace: string
  target: string
  detail: string
  success: boolean
  error: string
  remote: string
}
