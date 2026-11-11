# Architecture

Podora is a single Go backend plus a React single-page app.

```text
Browser ──HTTPS/WSS──> frontend (nginx, :3000) ──/api──> backend (:8080) ──> Kubernetes API
```

- The browser only talks to the frontend origin; nginx (or the Vite dev server)
  proxies `/api` to the backend. Kubernetes credentials never reach the browser.
- In-cluster, both containers run in one pod and the backend uses the pod's
  ServiceAccount.

## Backend layout (`backend/internal`)

| Package      | Responsibility                                         |
|--------------|--------------------------------------------------------|
| `api`        | HTTP routing, handlers, request/response encoding      |
| `config`     | Runtime configuration from environment variables       |
| `kubernetes` | client-go connection, discovery, watches and informers |
| `resources`  | Generic resource listing, inspection and mutation      |
| `logs`       | Real-time container log streaming                      |
| `helm`       | Chart repositories and release lifecycle (Helm Go SDK) |
| `auth`       | Authentication and sessions                            |
| `rbac`       | Podora's application-level roles and permissions       |

Only `api` and `config` contain code so far; the others are placeholders.

## Frontend layout (`frontend/src`)

`pages/` (routed views), `layouts/` (page shells), `components/` (shared UI),
`hooks/`, `services/` (API client), `types/` (API types).
