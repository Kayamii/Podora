import type { ReactNode } from 'react'
import { AuthGate } from './components/AuthGate'
import { Notice, PageHeader, TextLink } from './components/ui'
import { useSession } from './hooks/session'
import { usePolling } from './hooks/usePolling'
import type { Polled } from './hooks/usePolling'
import { href, sectionOf, useRoute } from './hooks/useRoute'
import { AppLayout } from './layouts/AppLayout'
import { Audit } from './pages/Audit'
import { Events } from './pages/Events'
import { HelmInstall } from './pages/HelmInstall'
import { HelmRelease } from './pages/HelmRelease'
import { HelmReleases } from './pages/HelmReleases'
import { Namespace } from './pages/Namespace'
import { Namespaces } from './pages/Namespaces'
import { Network } from './pages/Network'
import { Nodes } from './pages/Nodes'
import { Overview } from './pages/Overview'
import { Pod } from './pages/Pod'
import { Pods } from './pages/Pods'
import { Users } from './pages/Users'
import { Workload } from './pages/Workload'
import { Workloads } from './pages/Workloads'
import { api } from './services/api'
import { formatClock } from './services/format'
import type { Overview as OverviewData } from './types/api'

export default function App() {
  return (
    <AuthGate>
      <Shell />
    </AuthGate>
  )
}

/** Resolves to null without a request: used for data the user may not read. */
const nothing = async () => null

function Shell() {
  const route = useRoute()
  const { me, can } = useSession()
  const clusterWide = me.allNamespaces
  // The overview feeds the top bar and the cluster pages. Users limited to
  // some namespaces cannot read cluster-wide data, so it is skipped for them.
  const overview = usePolling(clusterWide ? api.overview : nothing, 5_000)
  const history = usePolling(clusterWide ? api.history : nothing, 10_000)
  const { data, error } = overview
  const page = !clusterWide && (route.page === 'overview' || route.page === 'nodes') ? 'namespaces' : route.page

  return (
    <AppLayout
      section={page === 'namespaces' ? 'namespaces' : sectionOf(route)}
      status={
        data ? (
          <>
            <span className="hidden sm:inline">Kubernetes {data.cluster.version}</span>
            <span className="flex items-center gap-2">
              <span
                aria-hidden="true"
                className={`size-2 rounded-full ${error ? 'bg-warn' : 'bg-ok'}`}
              />
              {error ? `Disconnected · last update ${formatClock(data.generatedAt)}` : 'Connected'}
            </span>
          </>
        ) : (
          !clusterWide && <span>Limited to {me.user.namespaces.join(', ')}</span>
        )
      }
    >
      {page === 'overview' && (
        <NeedsOverview overview={overview}>{(o) => <Overview overview={o} history={history.data} />}</NeedsOverview>
      )}
      {page === 'nodes' && <NeedsOverview overview={overview}>{(o) => <Nodes overview={o} />}</NeedsOverview>}
      {page === 'namespaces' && <Namespaces overview={data} />}
      {route.page === 'users' && (can('users.manage') ? <Users /> : <NotAllowed />)}
      {route.page === 'audit' && (can('audit.view') ? <Audit /> : <NotAllowed />)}
      {route.page === 'namespace' && <Namespace key={route.name} name={route.name} />}
      {route.page === 'helm' && <HelmReleases />}
      {route.page === 'helmInstall' && <HelmInstall />}
      {route.page === 'helmRelease' && (
        <HelmRelease key={`${route.namespace}/${route.name}`} namespace={route.namespace} name={route.name} />
      )}
      {route.page === 'workloads' && <Workloads namespace={route.namespace} />}
      {route.page === 'pods' && <Pods namespace={route.namespace} />}
      {route.page === 'network' && <Network namespace={route.namespace} />}
      {route.page === 'events' && <Events namespace={route.namespace} />}
      {/* A new key gives each resource its own polling loop and log stream. */}
      {route.page === 'workload' && (
        <Workload key={`${route.kind}/${route.namespace}/${route.name}`} kind={route.kind} namespace={route.namespace} name={route.name} />
      )}
      {route.page === 'pod' && <Pod key={`${route.namespace}/${route.name}`} namespace={route.namespace} name={route.name} />}
      {route.page === 'notFound' && (
        <div>
          <PageHeader title="Page not found" />
          <p className="text-fg-2">
            This address does not match anything in Podora. <TextLink to={clusterWide ? href.overview : href.namespaces}>Go to the start page</TextLink>
          </p>
        </div>
      )}
    </AppLayout>
  )
}

/** Renders a page that is built from the cluster overview, with its loading and error states. */
function NeedsOverview({ overview, children }: { overview: Polled<OverviewData | null>; children: (o: OverviewData) => ReactNode }) {
  const { data, error, loading } = overview
  return (
    <>
      {error && (
        <Notice level={data ? 'warning' : 'fault'}>
          {data
            ? `Cannot reach the cluster. Showing data from ${formatClock(data.generatedAt)}; retrying every few seconds.`
            : 'Podora cannot reach the Kubernetes API. Retrying every few seconds.'}
          <span className="mt-1 block text-sm opacity-80">{error}</span>
        </Notice>
      )}
      {loading && <p className="text-fg-2">Loading cluster data…</p>}
      {data && children(data)}
    </>
  )
}

function NotAllowed() {
  return (
    <div>
      <PageHeader title="Not available" />
      <p className="text-fg-2">Your account does not have access to this page. Ask an administrator if you need it.</p>
    </div>
  )
}
