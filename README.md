# Podora

**Your Kubernetes clusters. Under control.**

Podora is a lightweight, self-hosted Kubernetes management platform designed to make cluster operations accessible through a clean, intuitive web interface.

Manage deployments, inspect pods, stream container logs, install Helm charts, configure resources, troubleshoot failures and operate Kubernetes clusters without relying on the command line.

The goal is simple: **everything you normally do with `kubectl` or `helm` should be accessible through a UI.**

---

## Vision

Kubernetes is powerful, but everyday cluster operations often require extensive CLI knowledge, manual commands and constant context switching.

Podora aims to simplify that experience by providing a centralized interface for managing Kubernetes resources and deployments.

It is not intended to be another monitoring dashboard. It is an operational management platform that allows users to interact with and control their Kubernetes environments.

### Core principles

- **Zero CLI:** Perform everyday Kubernetes operations through the web interface.
- **Lightweight:** Minimal resource consumption and a small deployment footprint.
- **Self-hosted:** Runs directly inside the Kubernetes cluster it manages.
- **Reliable:** Use native Kubernetes APIs and established tooling.
- **Secure:** Fine-grained permissions and controlled access to cluster resources.
- **Real-time:** Live logs, resource updates and event streaming.
- **Simple:** Make complex Kubernetes operations understandable and accessible.

---

## Features

Podora is being developed around the following capabilities.

### 1. Cluster Overview

- Cluster health and status.
- Kubernetes version and node information.
- CPU and memory utilization.
- Namespace overview.
- Workload health.
- Recent events and operational activity.

### 2. Resource Explorer

Browse, search, filter and manage Kubernetes resources from one interface.

Supported resources will include:

- Pods
- Deployments
- ReplicaSets
- StatefulSets
- DaemonSets
- Services
- Ingresses
- ConfigMaps
- Secrets
- Jobs and CronJobs
- PersistentVolumes and PersistentVolumeClaims
- StorageClasses
- Nodes
- Namespaces
- ServiceAccounts
- Roles and RoleBindings
- Custom Resources (CRDs)

Filtering capabilities:

- Namespace
- Resource type
- Resource name
- Labels and selectors
- Status
- Helm release
- Custom filters

### 3. Workload Management

Manage Kubernetes workloads without writing commands.

- Create and modify deployments.
- Scale replicas.
- Update container images.
- Configure environment variables.
- Configure resource requests and limits.
- Manage probes and volumes.
- Restart workloads.
- Monitor rollout status.
- Roll back deployment revisions.

### 4. Pod Management

Inspect individual pods and their containers.

- Pod details and status.
- Container information.
- Container restart history.
- Live container logs.
- Previous container logs.
- Container selection.
- Pod events.
- Environment variables.
- Mounted volumes.
- Resource consumption.
- YAML inspection and editing.

### 5. Helm Management

A complete interface for Helm release lifecycle management.

- Add and manage chart repositories.
- Support HTTP and OCI repositories.
- Browse charts and versions.
- Install charts.
- Configure values through forms or YAML.
- Upgrade existing releases.
- View release history.
- Roll back releases.
- Uninstall releases.
- Inspect rendered manifests.

### 6. Networking and Storage

Manage networking and persistent storage resources.

- Services and endpoints.
- Ingresses and TLS configuration.
- NetworkPolicies.
- PersistentVolumes.
- PersistentVolumeClaims.
- StorageClasses.

### 7. Events and Troubleshooting

- Centralized Kubernetes event viewer.
- Namespace and resource filtering.
- Warning and normal events.
- Workload failure inspection.
- Container logs and restart analysis.
- Resource relationship navigation.
- Guided troubleshooting.

### 8. Access Control

Podora will provide its own user management and authorization layer.

Planned roles:

- Administrator
- Operator
- Developer
- Viewer
- Namespace Administrator
- Custom roles

Permissions will be configurable by:

- Cluster
- Namespace
- Resource type
- Operation

Users will only be able to perform operations authorized by Podora's permission system.

---

## Architecture

Podora follows a lightweight, modular architecture.

```text
                    User Browser
                         |
                    HTTPS / WSS
                         |
                  Podora Frontend
                         |
                    REST API
                         |
                  Podora Backend
                         |
             +-----------+-----------+
             |           |           |
        Kubernetes     Helm       Auth / RBAC
         Client        Engine       Engine
             |           |           |
             +-----------+-----------+
                         |
                  Kubernetes API
                         |
                  ServiceAccount
                         |
                 Kubernetes Cluster
```

### Technology Stack

| Component | Technology |
|---|---|
| Backend | Go |
| Kubernetes integration | client-go |
| Helm integration | Official Helm Go SDK |
| Frontend | React |
| Language | TypeScript |
| Styling | Tailwind CSS |
| API communication | REST |
| Real-time communication | WebSockets |
| Database | SQLite initially |
| Production database | PostgreSQL |
| Deployment | Helm |

### Backend

The backend will be written in Go.

It will handle:

- Kubernetes API communication.
- Resource discovery and management.
- Helm operations.
- Authentication and authorization.
- Real-time log streaming.
- Kubernetes watches and event processing.
- Audit logging.
- User preferences and configuration.

The backend should remain modular internally while running as a single application.

### Frontend

The frontend will use React and TypeScript.

The interface should prioritize:

- Fast navigation.
- Minimal visual complexity.
- Consistent resource views.
- Search and filtering.
- Responsive layouts.
- Real-time updates.
- Clear operational feedback.

### Kubernetes Integration

Podora will use Kubernetes-native APIs through `client-go`.

The platform will run inside Kubernetes using a dedicated ServiceAccount.

All cluster operations will be executed through the backend. Kubernetes credentials must never be exposed to the frontend.

---

## Deployment Model

Podora is designed to be installed directly into a Kubernetes cluster.

The initial deployment will consist of:

- Podora backend.
- Podora frontend.
- Service.
- Ingress (optional).
- ServiceAccount.
- RBAC configuration.
- Persistent storage (when required).

Installation and configuration will be distributed through a Helm chart.

The initial version will focus on managing the cluster where Podora is installed.

Multi-cluster management may be introduced in a future release.

---

## Security

Security is a fundamental part of the architecture.

- Dedicated Kubernetes ServiceAccount.
- Explicit Kubernetes RBAC permissions.
- Application-level authorization.
- Secure authentication.
- Namespace-level access restrictions.
- Secret access restrictions.
- Audit logs for sensitive operations.
- No direct exposure of Kubernetes credentials.
- Protection against unauthorized destructive operations.

Podora must never assume that a user authorized to access the interface is automatically authorized to access every cluster resource.

---

## Getting Started

```sh
cd backend && go run ./cmd/podora     # API on http://localhost:8080
cd frontend && npm install && npm run dev   # UI on http://localhost:3000
```

Or run both in containers with `docker compose up --build`.

## Installing on a cluster

```sh
helm install podora oci://ghcr.io/kayamii/charts/podora   --namespace podora --create-namespace
kubectl -n podora logs deploy/podora -c backend   # shows the one-time setup token
kubectl -n podora port-forward svc/podora 8090:80 # then open http://localhost:8090
```

Podora starts read-only. Enable actions with `--set rbac.allowWrites=true`
(scale, restart, delete pods) and `--set rbac.helm=true` (Helm management),
and expose it with the chart's Ingress over HTTPS. See
[docs/security](docs/security/README.md) before enabling either.

Published images and charts come from the release workflow, which runs when a
`v*.*.*` tag is pushed.

See [docs/development](docs/development/README.md) for details, and [docs/architecture](docs/architecture/README.md) for the repository layout.

---

## Development Roadmap

### Phase 1 — Foundation

- [x] Initialize repository structure.
- [x] Set up Go backend.
- [x] Set up React frontend.
- [x] Establish frontend/backend communication.
- [x] Implement Kubernetes client integration.
- [x] Implement ServiceAccount authentication.
- [ ] Implement resource discovery.
- [x] Build cluster overview.
- [x] Implement namespace listing.

### Phase 2 — Resource Management

- [ ] Universal resource explorer.
- [x] Namespace filtering.
- [x] Resource search.
- [x] Pod details.
- [x] Container selection.
- [x] Live log streaming.
- [x] Event viewer.
- [x] Deployment management.
- [ ] Basic YAML editor.

### Phase 3 — Helm Integration

- [ ] Helm repository management.
- [ ] Chart discovery.
- [x] Chart installation.
- [x] Values configuration.
- [x] Release management.
- [x] Upgrade and rollback.
- [x] Manifest inspection.

### Phase 4 — Access Control

- [x] Authentication.
- [x] User management.
- [x] Role management.
- [x] Namespace-level permissions.
- [ ] Resource-level permissions.
- [x] Audit logging.

### Phase 5 — Production Readiness

- [ ] Resource optimization.
- [ ] Error handling and recovery.
- [ ] Security review.
- [ ] Integration testing.
- [ ] Helm deployment chart.
- [ ] Documentation.
- [ ] Release automation.

---

## Development Principles

1. Prefer Kubernetes-native APIs over custom implementations.
2. Avoid unnecessary dependencies and infrastructure.
3. Keep the backend modular without introducing premature microservices.
4. Use watches and informers instead of excessive polling.
5. Design for clusters with large numbers of resources.
6. Implement authorization before exposing write operations.
7. Treat destructive operations carefully.
8. Keep the UI consistent across resource types.
9. Make every operation observable and provide meaningful error messages.
10. Prioritize reliability and maintainability over premature optimization.

---

## Project Status

**Early development — Architecture and foundation stage.**

Podora is an independent open-source project under development.

The initial objective is to deliver a reliable, lightweight Kubernetes management experience that can be installed directly into a cluster and operated entirely through a web browser.

---

## License

To be determined.

---

**Podora — Your Kubernetes clusters. Under control.**