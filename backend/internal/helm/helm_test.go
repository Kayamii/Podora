package helm

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"strings"
	"testing"
	"time"

	chart "helm.sh/helm/v4/pkg/chart/v2"
)

// chartArchive builds a minimal chart .tgz in memory.
func chartArchive(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		if err := tw.WriteHeader(&tar.Header{Name: "demo/" + name, Mode: 0o644, Size: int64(len(body))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestLoadArchiveKeepsValuesComments(t *testing.T) {
	m := &Manager{}
	values := "# How many pods to run.\nreplicaCount: 2\n"
	data := chartArchive(t, map[string]string{
		"Chart.yaml":               "apiVersion: v2\nname: demo\nversion: 1.2.3\nappVersion: \"4.5\"\ndescription: A demo\n",
		"values.yaml":              values,
		"templates/configmap.yaml": "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: {{ .Release.Name }}\n",
	})

	info, err := m.LoadArchive(data)
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != "demo" || info.Version != "1.2.3" || info.AppVersion != "4.5" {
		t.Errorf("info = %+v", info)
	}
	// The editor must show values.yaml as written, comments included.
	if info.Values != values {
		t.Errorf("values = %q, want %q", info.Values, values)
	}
	if _, ok := m.charts.get(info.ID); !ok {
		t.Error("chart was not kept for install")
	}
	// The same archive gets the same ID.
	again, _ := m.LoadArchive(data)
	if again.ID != info.ID {
		t.Errorf("IDs differ: %s vs %s", again.ID, info.ID)
	}
}

func TestLoadArchiveRejectsGarbage(t *testing.T) {
	m := &Manager{}
	if _, err := m.LoadArchive([]byte("not a tarball")); !errors.Is(err, ErrInvalid) {
		t.Errorf("err = %v, want ErrInvalid", err)
	}
}

func TestParseValues(t *testing.T) {
	vals, err := parseValues("replicaCount: 3\nimage:\n  tag: v2\n")
	if err != nil {
		t.Fatal(err)
	}
	if vals["replicaCount"].(float64) != 3 {
		t.Errorf("vals = %v", vals)
	}
	if vals, err := parseValues("   \n"); err != nil || len(vals) != 0 {
		t.Errorf("blank: vals = %v, err = %v", vals, err)
	}
	if _, err := parseValues("image:\n  tag: [unclosed\n"); !errors.Is(err, ErrInvalid) {
		t.Errorf("bad yaml: err = %v, want ErrInvalid", err)
	}
}

func TestResources(t *testing.T) {
	manifest := `---
# Source: demo/templates/svc.yaml
apiVersion: v1
kind: Service
metadata:
  name: web
---
# Source: demo/templates/deploy.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
  namespace: other
`
	got := resources(manifest, "demo")
	if len(got) != 2 {
		t.Fatalf("resources = %+v", got)
	}
	if got[0] != (Resource{Kind: "Deployment", Name: "web", Namespace: "other"}) {
		t.Errorf("first = %+v", got[0])
	}
	// Without a namespace in the manifest, the release namespace applies.
	if got[1] != (Resource{Kind: "Service", Name: "web", Namespace: "demo"}) {
		t.Errorf("second = %+v", got[1])
	}
}

func TestChartCacheEvicts(t *testing.T) {
	var c chartCache
	for i := 0; i < chartCacheSize+5; i++ {
		c.put(strings.Repeat("x", i+1), &chart.Chart{})
	}
	if len(c.entries) > chartCacheSize {
		t.Errorf("cache holds %d charts, limit %d", len(c.entries), chartCacheSize)
	}
	c.entries["old"] = cachedChart{chart: &chart.Chart{}, added: time.Now().Add(-2 * chartCacheTTL)}
	if _, ok := c.get("old"); ok {
		t.Error("expired chart still served")
	}
}

func TestWrapHelmError(t *testing.T) {
	err := wrapHelmError("install", errors.New("cannot re-use a name that is still in use"))
	if !errors.Is(err, ErrInvalid) {
		t.Errorf("name conflict: err = %v, want ErrInvalid", err)
	}
	if err := wrapHelmError("install", errors.New("connection refused")); errors.Is(err, ErrInvalid) {
		t.Errorf("cluster failure classified as invalid: %v", err)
	}
}
