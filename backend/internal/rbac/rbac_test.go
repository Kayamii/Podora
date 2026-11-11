package rbac

import "testing"

func TestCan(t *testing.T) {
	scoped := Subject{Role: Operator, Namespaces: []string{"demo"}}
	all := Subject{Role: Operator}

	cases := []struct {
		name string
		got  bool
		want bool
	}{
		{"scoped operator scales in its namespace", scoped.Can(ScaleWork, "demo"), true},
		{"scoped operator cannot scale elsewhere", scoped.Can(ScaleWork, "other"), false},
		{"scoped operator cannot act cluster-wide", scoped.Can(Read, ""), false},
		{"scoped operator cannot use Helm", scoped.Has(ManageHelm), false},
		{"unscoped operator uses Helm", all.Has(ManageHelm), true},
		{"operator cannot manage users", all.Has(ManageUsers), false},
		{"viewer cannot restart", Subject{Role: Viewer}.Has(RestartWork), false},
		{"developer restarts", Subject{Role: Developer}.Has(RestartWork), true},
		{"developer cannot scale", Subject{Role: Developer}.Has(ScaleWork), false},
		{"unknown role has nothing", Subject{Role: "root"}.Has(Read), false},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, tc.got, tc.want)
		}
	}

	perms := scoped.Permissions()
	for _, p := range perms {
		if p == ManageHelm {
			t.Error("scoped operator's permission list includes Helm")
		}
	}
}
