# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Users

A mixed audience: people who know Kubernetes well and people who do not. Podora
has to be approachable for someone who avoids `kubectl` without being dumbed
down for someone who lives in it. Typical situation: checking on, or acting on,
a cluster they are responsible for, often in the middle of other work.

## Product Purpose

Podora is a lightweight, self-hosted Kubernetes management platform. It runs
inside the cluster it manages and makes everyday cluster operations available
through a web interface: everything normally done with `kubectl` or `helm`
should be doable in the UI. It is an operational management tool, not another
monitoring dashboard. Success is a user completing a cluster operation without
opening a terminal.

## Positioning

Self-hosted and in-cluster, with a small footprint, and built for operating the
cluster (deploy, scale, inspect, roll back, install charts) rather than only
observing it.

## Operating Context

- Installed into a cluster with a Helm chart; the backend talks to the
  Kubernetes API through a dedicated ServiceAccount.
- The first release manages only the cluster it is installed in.
- The cluster overview is the landing screen. Its first job is to answer
  "is everything healthy?" with one clear verdict, then show only what needs
  attention; detail lives one step away.

## Capabilities and Constraints

- Built so far: a cluster overview (nodes, CPU and memory usage, pods,
  workloads, network, storage, system pods, namespaces, recent events),
  refreshed by polling every few seconds.
- CPU and memory usage depend on metrics-server; without it only requests are
  known. The backend returns current values only, no stored history yet.
- Planned, not built: resource explorer, workload and pod management, live
  logs, Helm management, access control with roles. No authentication yet.
- Stack: Go backend, React + TypeScript + Tailwind frontend.
- Kubernetes terminology is kept as-is (Pod, Deployment, Namespace); the UI
  does not rename cluster concepts.

## Brand Commitments

- Name: Podora. Tagline: "Your Kubernetes clusters. Under control."
- It must not feel toy-like or playful: it is a serious tool for production.
- It must not feel like a heavy corporate cloud console.
- No logo or other brand assets exist yet.

## Evidence on Hand

- Live data from a real k3s test cluster through `GET /api/v1/overview`.
- No customers, testimonials, benchmarks or pricing exist; none may be invented.

## Product Principles

1. Verdict first, detail on demand: say whether things are healthy before
   showing how they are measured.
2. Zero CLI: an operation that needs a terminal is an unfinished feature.
3. Honest state: never show a guess as a fact; say when data is missing or
   stale.
4. Careful with destructive actions: authorization comes before write
   operations.
5. Lightweight in every sense: small footprint, few dependencies, little
   visual noise.
