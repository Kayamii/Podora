# Security

Design rules (see the main README for the full list):

- All cluster operations go through the backend; Kubernetes credentials are
  never exposed to the frontend.
- Podora runs with a dedicated ServiceAccount and explicit RBAC.
- Access to the UI does not imply access to every resource: Podora's own
  authorization layer is enforced before any write operation is exposed.

## Accounts and access

- Every page and API call needs a signed-in user, except health checks.
- On first start with no accounts, the backend prints a one-time setup token
  to its log (or uses `setupToken` from the Helm chart). The first
  administrator can only be created with it, so reaching the page first is not
  enough to take over an installation.
- Passwords are stored as bcrypt hashes (at least 10 characters). Sessions are
  random 256-bit tokens kept only as SHA-256 hashes, in an HttpOnly,
  SameSite=Lax cookie that is marked Secure behind HTTPS. A session ends after
  12 hours idle or 7 days at most. Five wrong passwords lock an account for a
  minute.
- Roles: **Viewer** (read, including logs), **Developer** (+ restart workloads,
  delete pods), **Operator** (+ scale; + Helm with all namespaces),
  **Administrator** (+ users and audit log; always all namespaces).
- Any non-administrator can be limited to a list of namespaces. Lists are
  filtered to them, and cluster-wide pages (overview, nodes) are refused.
  Helm management needs access to every namespace, because a chart can create
  resources outside its own namespace.
- Every change made through Podora, every sign-in and every account change is
  written to the audit log with the user, the target, the outcome and the error
  if any. Entries are kept 180 days. Administrators can read it in the UI.
- The last active administrator cannot be removed, demoted or disabled, and
  administrators cannot change their own role or delete themselves.
- All of this is enforced in the backend; the UI only hides what would be
  refused.

## Current state

- The Helm chart grants the ServiceAccount **read-only** (`get`/`list`/`watch`)
  cluster access. Secrets are excluded unless `rbac.readSecrets=true`.
- Write actions (scale, restart, delete pod) need `rbac.allowWrites=true` in
  the Helm chart, which adds only `patch` on deployments, statefulsets and
  daemonsets and `delete` on pods. With it off, the cluster refuses them.
- Write requests must carry an `X-Podora-Request` header, which stops other
  websites from triggering changes through a user's browser. Each applied
  change is logged with the caller's address.
- Helm install, upgrade and uninstall need `rbac.helm=true`, which binds the
  service account to `cluster-admin`: a chart can create any kind of
  resource, so nothing narrower works. Treat enabling it as giving every user
  of Podora full control of the cluster.
- Every role can read pod logs in its namespaces, and logs often contain
  sensitive data. Limit users to the namespaces they need.
- Serve Podora over HTTPS (for example through the chart's Ingress with TLS):
  passwords and session cookies must not travel in clear text.

## Reporting a vulnerability

Please report security issues privately to the maintainers rather than opening
a public issue.
