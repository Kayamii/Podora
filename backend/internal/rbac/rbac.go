// Package rbac decides what a Podora user may see and do. It is Podora's own
// authorization, enforced before any request reaches the cluster, and is
// independent of the Kubernetes RBAC granted to Podora's service account.
package rbac

import "slices"

type Role string

const (
	Viewer    Role = "viewer"
	Developer Role = "developer"
	Operator  Role = "operator"
	Admin     Role = "admin"
)

// Roles lists every role, least to most powerful.
var Roles = []Role{Viewer, Developer, Operator, Admin}

func (r Role) Valid() bool { return slices.Contains(Roles, r) }

type Permission string

const (
	// Read covers every read: resources, events, logs, usage, releases.
	Read        Permission = "read"
	RestartWork Permission = "workloads.restart"
	DeletePod   Permission = "pods.delete"
	ScaleWork   Permission = "workloads.scale"
	ManageHelm  Permission = "helm.manage"
	ManageUsers Permission = "users.manage"
	ViewAudit   Permission = "audit.view"
)

var grants = map[Role][]Permission{
	Viewer:    {Read},
	Developer: {Read, RestartWork, DeletePod},
	Operator:  {Read, RestartWork, DeletePod, ScaleWork, ManageHelm},
	Admin:     {Read, RestartWork, DeletePod, ScaleWork, ManageHelm, ManageUsers, ViewAudit},
}

// clusterWide permissions need access to every namespace, whatever the role.
// A Helm chart can create resources outside its namespace (cluster roles,
// CRDs, objects in other namespaces), so a namespace limit cannot contain it.
var clusterWide = []Permission{ManageHelm, ManageUsers, ViewAudit}

// Subject is a signed-in user as far as authorization is concerned.
type Subject struct {
	Username string
	Role     Role
	// Namespaces limits the user; empty means every namespace.
	Namespaces []string
}

// AllNamespaces reports whether the user is unrestricted.
func (s Subject) AllNamespaces() bool { return len(s.Namespaces) == 0 }

// InNamespace reports whether the user may see the namespace at all.
func (s Subject) InNamespace(ns string) bool {
	return s.AllNamespaces() || slices.Contains(s.Namespaces, ns)
}

// Has reports whether the role grants p, ignoring namespaces.
func (s Subject) Has(p Permission) bool {
	if !slices.Contains(grants[s.Role], p) {
		return false
	}
	if slices.Contains(clusterWide, p) && !s.AllNamespaces() {
		return false
	}
	return true
}

// Can reports whether the user may do p in namespace ns. An empty ns means a
// cluster-wide action, which needs access to every namespace.
func (s Subject) Can(p Permission, ns string) bool {
	if !s.Has(p) {
		return false
	}
	if ns == "" {
		return s.AllNamespaces()
	}
	return s.InNamespace(ns)
}

// Permissions lists what the user holds, for the UI to show or hide actions.
func (s Subject) Permissions() []Permission {
	out := []Permission{}
	for _, p := range grants[s.Role] {
		if s.Has(p) {
			out = append(out, p)
		}
	}
	return out
}
