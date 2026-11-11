# Development

## Prerequisites

- Go 1.26+
- Node.js 24+
- Helm 3 or 4 (chart linting)
- Docker (optional, for images and `docker compose`)

## Run locally

```sh
make install       # frontend dependencies (first time)
make dev-backend   # backend on http://localhost:8080
make dev-frontend  # frontend on http://localhost:3000, proxies /api to the backend
```

Without `make` (e.g. on Windows), run the underlying commands directly:

```sh
cd backend && go run ./cmd/podora
cd frontend && npm run dev
```

Or run the containerized stack with `docker compose up --build`, then open
http://localhost:3000.

## Checks

```sh
make test   # go test
make lint   # go vet, oxlint, helm lint
make build  # backend binary + frontend bundle
```

## Configuration

| Variable             | Component | Default                 | Purpose                           |
|----------------------|-----------|-------------------------|-----------------------------------|
| `PODORA_ADDR`        | backend   | `:8080`                 | Listen address                    |
| `PODORA_BACKEND_URL` | frontend  | `http://localhost:8080` | Where `/api` requests are proxied |
| `PODORA_KUBECONFIG`  | backend   | (empty)                 | Explicit kubeconfig path          |
| `PODORA_DATA_DIR`    | backend   | `data`                  | Where the SQLite database (accounts, sessions, audit log) lives |
| `PODORA_SETUP_TOKEN` | backend   | (random)                | Fixed token for creating the first administrator |

On the first start with an empty data directory the backend prints a setup
token; open the UI and use it to create the first administrator. Delete
`backend/data` to start over.

## Cluster access

The backend picks its credentials in this order:

1. `PODORA_KUBECONFIG`, if set.
2. The pod's ServiceAccount, when running inside a cluster.
3. The standard kubeconfig lookup (`KUBECONFIG`, then `~/.kube/config`).

Any cluster works for development (kind, k3d, Docker Desktop). CPU and memory
usage figures need metrics-server; without it the dashboard falls back to
showing resource requests.

## API

| Endpoint                 | Returns                                             |
|--------------------------|-----------------------------------------------------|
| `GET /healthz`           | Process liveness (does not touch the cluster)       |
| `GET /api/v1/health`     | Same, for the frontend                              |
| `GET /api/v1/overview`   | Cluster summary used by the dashboard               |
| `GET /api/v1/namespaces` | Namespaces with status and pod counts               |
| `GET /api/v1/metrics/history` | CPU and memory usage samples from the last hour (kept in memory) |
| `GET /api/v1/pods?namespace=` | Pods in one namespace, or all when omitted     |
| `GET /api/v1/namespaces/{ns}/pods/{name}` | One pod with containers and events |
| `GET /api/v1/namespaces/{ns}/pods/{name}/logs` | Container log as streamed plain text |

| `GET /api/v1/workloads?namespace=` | Deployments, StatefulSets and DaemonSets |
| `GET /api/v1/namespaces/{ns}/workloads/{kind}/{name}` | One workload with its pods and events (`kind`: deployments, statefulsets, daemonsets) |
| `GET /api/v1/services?namespace=` | Services |
| `GET /api/v1/ingresses?namespace=` | Ingresses |
| `GET /api/v1/events?namespace=` | Up to 200 recent events, newest first |
| `POST /api/v1/namespaces/{ns}/workloads/{kind}/{name}/scale` | Body `{"replicas": n}` (0 to 500); not for DaemonSets |
| `POST /api/v1/namespaces/{ns}/workloads/{kind}/{name}/restart` | Same as `kubectl rollout restart` |
| `DELETE /api/v1/namespaces/{ns}/pods/{name}` | Deletes a pod; its workload starts a replacement |

| `GET /api/v1/namespaces/usage` | Every namespace with requests, limits and current usage |
| `GET /api/v1/metrics/usage-history?namespace=&pod=` | Last hour of CPU and memory for a namespace, or one pod (kept in memory) |
| `GET /api/v1/helm/releases` | Latest revision of every Helm release |
| `GET /api/v1/helm/releases/{ns}/{name}` | One release: values, chart defaults, manifest, resources, notes, history |
| `POST /api/v1/helm/charts` | Multipart upload of a `.tgz` (field `chart`, up to 20 MB); returns its values.yaml and an `id` |
| `POST /api/v1/helm/charts/fetch` | `{"repoURL","chart","version"}`, or `chart: "oci://..."` with no repo; same answer as upload |
| `POST /api/v1/helm/releases` | Install: `{"chartId","name","namespace","createNamespace","values","dryRun"}` |
| `POST /api/v1/helm/releases/{ns}/{name}/upgrade` | `{"values","chartId"?,"dryRun"}`; `values` is the complete set for the release |
| `POST /api/v1/helm/releases/{ns}/{name}/rollback` | `{"revision": n}` |
| `DELETE /api/v1/helm/releases/{ns}/{name}` | Uninstall |

Helm runs in-process through the Helm v4 Go SDK; no `helm` binary is needed.
Releases are stored the standard way (Secrets in the release namespace), so
the `helm` CLI sees and can manage the same releases. Uploaded charts are kept
in memory for an hour, waiting to be installed. `dryRun: true` renders the
manifest without applying anything.

Requests that change the cluster must carry an `X-Podora-Request` header
(any value) or they are refused with `403`. Browsers will not send a custom
header cross-site without a CORS preflight, which Podora never approves, so
this blocks other web pages from triggering changes. It is not
authentication. Every applied change is logged by the backend.

Log query parameters: `container`, `tail` (1 to 5000 lines, default 100),
`previous=true` (the container's previous run), `follow=true` (keep the
response open and stream new lines), `timestamps=true`.

Errors are JSON `{"error": "..."}`: `400` for a bad request, `404` when the
resource does not exist, `403` when the cluster refuses Podora's service
account, and `502` for any other failed Kubernetes call.

## Trying a build on a test cluster

Without a registry, build the images and load them straight into the
cluster (shown for k3d; kind has `kind load docker-image`):

```sh
docker build -t podora-backend:dev backend
docker build -t podora-frontend:dev frontend
k3d image import -c <cluster> podora-backend:dev podora-frontend:dev

helm upgrade --install podora charts/podora -n podora --create-namespace   --set backend.image.repository=podora-backend --set backend.image.tag=dev   --set frontend.image.repository=podora-frontend --set frontend.image.tag=dev   --set backend.image.pullPolicy=Never --set frontend.image.pullPolicy=Never

kubectl -n podora logs deploy/podora -c backend     # setup token
kubectl -n podora port-forward svc/podora 8090:80   # http://localhost:8090
```

After loading a rebuilt image under the same tag, delete the pod so it
restarts on the new image. To start over with no accounts, scale the
deployment to 0, delete the `podora-data` claim, and run the upgrade again.
