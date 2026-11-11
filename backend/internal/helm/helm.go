// Package helm installs, upgrades, rolls back and removes Helm releases
// through the official Helm Go SDK, so the UI never needs the helm CLI.
package helm

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"helm.sh/helm/v4/pkg/action"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	"helm.sh/helm/v4/pkg/chart/v2/loader"
	"helm.sh/helm/v4/pkg/cli"
	"helm.sh/helm/v4/pkg/kube"
	"helm.sh/helm/v4/pkg/registry"
	release "helm.sh/helm/v4/pkg/release/v1"
	releaseutil "helm.sh/helm/v4/pkg/release/v1/util"
	"helm.sh/helm/v4/pkg/storage/driver"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"sigs.k8s.io/yaml"
)

var (
	// ErrInvalid marks a request the caller got wrong: bad values YAML, a
	// missing chart, an unreadable archive. The API reports it as 400.
	ErrInvalid = errors.New("invalid request")
	// ErrNotFound means the release or uploaded chart does not exist.
	ErrNotFound = errors.New("not found")
)

const (
	// MaxChartBytes bounds uploaded chart archives.
	MaxChartBytes = 20 << 20
	// operationTimeout bounds one install, upgrade, rollback or uninstall.
	operationTimeout = 5 * time.Minute
	chartCacheTTL    = time.Hour
	chartCacheSize   = 32
	historyMax       = 10
)

// Manager runs Helm actions against the cluster Podora manages.
type Manager struct {
	kubeconfig string
	settings   *cli.EnvSettings
	registry   *registry.Client
	charts     chartCache
	log        *slog.Logger
}

// New prepares a Manager. kubeconfig may be empty to use in-cluster
// credentials. Helm's cache and config live under a temporary directory unless
// HELM_*_HOME is set, so Podora works with a read-only root filesystem.
func New(kubeconfig string) (*Manager, error) {
	base := filepath.Join(os.TempDir(), "podora-helm")
	for env, dir := range map[string]string{"HELM_CACHE_HOME": "cache", "HELM_CONFIG_HOME": "config", "HELM_DATA_HOME": "data"} {
		if os.Getenv(env) == "" {
			if err := os.Setenv(env, filepath.Join(base, dir)); err != nil {
				return nil, err
			}
		}
	}
	settings := cli.New()
	reg, err := registry.NewClient(
		registry.ClientOptEnableCache(true),
		registry.ClientOptWriter(io.Discard),
		registry.ClientOptCredentialsFile(settings.RegistryConfig),
	)
	if err != nil {
		return nil, fmt.Errorf("creating OCI registry client: %w", err)
	}
	return &Manager{
		kubeconfig: kubeconfig,
		settings:   settings,
		registry:   reg,
		log:        slog.Default().With("component", "helm"),
	}, nil
}

// config builds an action configuration scoped to one namespace ("" for all).
func (m *Manager) config(namespace string) (*action.Configuration, error) {
	flags := genericclioptions.NewConfigFlags(true)
	if m.kubeconfig != "" {
		flags.KubeConfig = &m.kubeconfig
	}
	flags.Namespace = &namespace
	cacheDir := filepath.Join(os.Getenv("HELM_CACHE_HOME"), "kube")
	flags.CacheDir = &cacheDir

	cfg := action.NewConfiguration()
	cfg.SetLogger(m.log.Handler())
	if err := cfg.Init(flags, namespace, "secret"); err != nil {
		return nil, fmt.Errorf("initializing helm: %w", err)
	}
	cfg.RegistryClient = m.registry
	return cfg, nil
}

// --- Types returned to the API -------------------------------------------------

// ChartInfo describes a chart that is ready to install.
type ChartInfo struct {
	// ID refers to the chart in later install or upgrade requests.
	ID          string `json:"id"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	AppVersion  string `json:"appVersion"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
	// Values is the chart's values.yaml exactly as written, comments included.
	Values string `json:"values"`
	Readme string `json:"readme"`
}

type ReleaseSummary struct {
	Name         string    `json:"name"`
	Namespace    string    `json:"namespace"`
	Revision     int       `json:"revision"`
	Status       string    `json:"status"`
	Chart        string    `json:"chart"`
	ChartVersion string    `json:"chartVersion"`
	AppVersion   string    `json:"appVersion"`
	Icon         string    `json:"icon"`
	Description  string    `json:"description"`
	Updated      time.Time `json:"updated"`
}

type ReleaseDetail struct {
	ReleaseSummary
	FirstDeployed time.Time `json:"firstDeployed"`
	Notes         string    `json:"notes"`
	Manifest      string    `json:"manifest"`
	// Values holds the values set for this release (not the chart defaults).
	Values string `json:"values"`
	// DefaultValues are the chart's defaults, for reference while editing.
	DefaultValues string      `json:"defaultValues"`
	Resources     []Resource  `json:"resources"`
	History       []Revision  `json:"history"`
	ChartInfo     ChartDigest `json:"chartInfo"`
}

type ChartDigest struct {
	Description string   `json:"description"`
	Home        string   `json:"home"`
	Sources     []string `json:"sources"`
}

// Resource is one object the release created.
type Resource struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

type Revision struct {
	Revision     int       `json:"revision"`
	Status       string    `json:"status"`
	ChartVersion string    `json:"chartVersion"`
	AppVersion   string    `json:"appVersion"`
	Description  string    `json:"description"`
	Updated      time.Time `json:"updated"`
}

// Rendered is what a dry run produces.
type Rendered struct {
	Manifest string `json:"manifest"`
	Notes    string `json:"notes"`
}

// --- Charts --------------------------------------------------------------------

// LoadArchive reads an uploaded .tgz chart and keeps it for a later install.
func (m *Manager) LoadArchive(data []byte) (*ChartInfo, error) {
	ch, err := loader.LoadArchive(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%w: this is not a valid Helm chart archive (%v)", ErrInvalid, err)
	}
	sum := sha256.Sum256(data)
	return m.remember(hex.EncodeToString(sum[:12]), ch)
}

// FetchFromRepository downloads a chart from an HTTP repository (repoURL plus
// chart name) or from an OCI registry (chart is "oci://..." and repoURL is
// empty), then keeps it for a later install.
func (m *Manager) FetchFromRepository(ctx context.Context, repoURL, name, version string) (*ChartInfo, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("%w: a chart name or oci:// reference is required", ErrInvalid)
	}
	if repoURL == "" && !registry.IsOCI(name) {
		return nil, fmt.Errorf("%w: give a repository URL, or a chart reference starting with oci://", ErrInvalid)
	}
	cfg, err := m.config("")
	if err != nil {
		return nil, err
	}
	install := action.NewInstall(cfg)
	install.RepoURL = strings.TrimSpace(repoURL)
	install.Version = strings.TrimSpace(version)

	// LocateChart has no context; run it in the background so a cancelled
	// request returns promptly.
	type located struct {
		path string
		err  error
	}
	done := make(chan located, 1)
	go func() {
		path, err := install.LocateChart(name, m.settings)
		done <- located{path, err}
	}()
	var path string
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-done:
		if res.err != nil {
			return nil, fmt.Errorf("%w: could not download the chart: %v", ErrInvalid, res.err)
		}
		path = res.path
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading downloaded chart: %w", err)
	}
	return m.LoadArchive(data)
}

func (m *Manager) remember(id string, ch *chart.Chart) (*ChartInfo, error) {
	if ch.Metadata == nil {
		return nil, fmt.Errorf("%w: the chart has no Chart.yaml", ErrInvalid)
	}
	if err := ch.Validate(); err != nil {
		return nil, fmt.Errorf("%w: the chart is not valid: %v", ErrInvalid, err)
	}
	m.charts.put(id, ch)
	return describeChart(id, ch), nil
}

func describeChart(id string, ch *chart.Chart) *ChartInfo {
	info := &ChartInfo{
		ID:          id,
		Name:        ch.Metadata.Name,
		Version:     ch.Metadata.Version,
		AppVersion:  ch.Metadata.AppVersion,
		Description: ch.Metadata.Description,
		Icon:        ch.Metadata.Icon,
	}
	for _, f := range ch.Raw {
		switch strings.ToLower(f.Name) {
		case "values.yaml":
			info.Values = string(f.Data)
		case "readme.md":
			info.Readme = string(f.Data)
		}
	}
	if info.Values == "" && len(ch.Values) > 0 {
		out, _ := yaml.Marshal(ch.Values)
		info.Values = string(out)
	}
	return info
}

// chartCache holds recently uploaded or fetched charts until they are used.
type chartCache struct {
	mu      sync.Mutex
	entries map[string]cachedChart
}

type cachedChart struct {
	chart *chart.Chart
	added time.Time
}

func (c *chartCache) put(id string, ch *chart.Chart) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = map[string]cachedChart{}
	}
	c.evict()
	c.entries[id] = cachedChart{chart: ch, added: time.Now()}
}

func (c *chartCache) get(id string) (*chart.Chart, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.evict()
	e, ok := c.entries[id]
	return e.chart, ok
}

// evict drops expired entries, then the oldest ones beyond the size limit.
func (c *chartCache) evict() {
	for id, e := range c.entries {
		if time.Since(e.added) > chartCacheTTL {
			delete(c.entries, id)
		}
	}
	for len(c.entries) >= chartCacheSize {
		oldest := ""
		for id, e := range c.entries {
			if oldest == "" || e.added.Before(c.entries[oldest].added) {
				oldest = id
			}
		}
		delete(c.entries, oldest)
	}
}

// --- Releases ------------------------------------------------------------------

// Releases lists the latest revision of every release in every namespace.
func (m *Manager) Releases() ([]ReleaseSummary, error) {
	cfg, err := m.config("")
	if err != nil {
		return nil, err
	}
	list := action.NewList(cfg)
	list.AllNamespaces = true
	list.All = true
	found, err := list.Run()
	if err != nil {
		return nil, fmt.Errorf("listing releases: %w", err)
	}
	out := make([]ReleaseSummary, 0, len(found))
	for _, r := range found {
		if rel, ok := r.(*release.Release); ok {
			out = append(out, summarize(rel))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Namespace != out[j].Namespace {
			return out[i].Namespace < out[j].Namespace
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// Release returns the latest revision of one release with its history.
func (m *Manager) Release(namespace, name string) (*ReleaseDetail, error) {
	cfg, err := m.config(namespace)
	if err != nil {
		return nil, err
	}
	rel, err := latest(cfg, name)
	if err != nil {
		return nil, err
	}

	d := &ReleaseDetail{
		ReleaseSummary: summarize(rel),
		Manifest:       rel.Manifest,
		Values:         toYAML(rel.Config),
		Resources:      resources(rel.Manifest, rel.Namespace),
		History:        []Revision{},
	}
	if rel.Info != nil {
		d.Notes = rel.Info.Notes
		d.FirstDeployed = rel.Info.FirstDeployed
	}
	if rel.Chart != nil {
		d.DefaultValues = toYAML(rel.Chart.Values)
		if md := rel.Chart.Metadata; md != nil {
			d.ChartInfo = ChartDigest{Description: md.Description, Home: md.Home, Sources: md.Sources}
		}
	}

	hist, err := action.NewHistory(cfg).Run(name)
	if err == nil {
		for _, h := range hist {
			r, ok := h.(*release.Release)
			if !ok {
				continue
			}
			s := summarize(r)
			d.History = append(d.History, Revision{
				Revision: s.Revision, Status: s.Status, ChartVersion: s.ChartVersion,
				AppVersion: s.AppVersion, Description: s.Description, Updated: s.Updated,
			})
		}
		sort.Slice(d.History, func(i, j int) bool { return d.History[i].Revision > d.History[j].Revision })
	}
	return d, nil
}

// InstallRequest describes a new release.
type InstallRequest struct {
	ChartID         string `json:"chartId"`
	Name            string `json:"name"`
	Namespace       string `json:"namespace"`
	CreateNamespace bool   `json:"createNamespace"`
	// Values is YAML text, typically the edited values.yaml.
	Values string `json:"values"`
	DryRun bool   `json:"dryRun"`
}

// Install creates a release, or with DryRun renders it without applying.
func (m *Manager) Install(req InstallRequest) (*ReleaseSummary, *Rendered, error) {
	ch, ok := m.charts.get(req.ChartID)
	if !ok {
		return nil, nil, fmt.Errorf("%w: the chart has expired; upload or fetch it again", ErrInvalid)
	}
	vals, err := parseValues(req.Values)
	if err != nil {
		return nil, nil, err
	}
	if req.Name == "" || req.Namespace == "" {
		return nil, nil, fmt.Errorf("%w: a release name and a namespace are required", ErrInvalid)
	}

	cfg, err := m.config(req.Namespace)
	if err != nil {
		return nil, nil, err
	}
	install := action.NewInstall(cfg)
	install.ReleaseName = req.Name
	install.Namespace = req.Namespace
	install.CreateNamespace = req.CreateNamespace
	install.ServerSideApply = true
	install.WaitStrategy = kube.HookOnlyStrategy
	install.Timeout = operationTimeout
	if req.DryRun {
		install.DryRunStrategy = action.DryRunClient
	}

	// Detached from the request: an install must not be abandoned half-way
	// because the browser tab closed.
	ctx, cancel := context.WithTimeout(context.Background(), operationTimeout)
	defer cancel()
	out, err := install.RunWithContext(ctx, ch, vals)
	if err != nil {
		return nil, nil, wrapHelmError("install", err)
	}
	rel, ok := out.(*release.Release)
	if !ok {
		return nil, nil, fmt.Errorf("install returned an unexpected release type %T", out)
	}
	if req.DryRun {
		return nil, rendered(rel), nil
	}
	m.log.Info("helm install", "release", req.Name, "namespace", req.Namespace, "chart", ch.Metadata.Name, "version", ch.Metadata.Version)
	s := summarize(rel)
	return &s, nil, nil
}

// UpgradeRequest changes a release's values and optionally its chart.
type UpgradeRequest struct {
	// ChartID is optional; empty keeps the chart the release already uses.
	ChartID string `json:"chartId"`
	// Values is the complete set of values for the release, as YAML text.
	Values string `json:"values"`
	DryRun bool   `json:"dryRun"`
}

func (m *Manager) Upgrade(namespace, name string, req UpgradeRequest) (*ReleaseSummary, *Rendered, error) {
	vals, err := parseValues(req.Values)
	if err != nil {
		return nil, nil, err
	}
	cfg, err := m.config(namespace)
	if err != nil {
		return nil, nil, err
	}

	var ch *chart.Chart
	if req.ChartID != "" {
		var ok bool
		if ch, ok = m.charts.get(req.ChartID); !ok {
			return nil, nil, fmt.Errorf("%w: the chart has expired; upload or fetch it again", ErrInvalid)
		}
	} else {
		current, err := latest(cfg, name)
		if err != nil {
			return nil, nil, err
		}
		if current.Chart == nil {
			return nil, nil, fmt.Errorf("%w: the release has no stored chart; upload one to upgrade", ErrInvalid)
		}
		ch = current.Chart
	}

	upgrade := action.NewUpgrade(cfg)
	upgrade.Namespace = namespace
	// The editor holds the full set of values, so start from the chart
	// defaults rather than merging into the previous revision's values.
	upgrade.ResetValues = true
	upgrade.ServerSideApply = "auto"
	upgrade.WaitStrategy = kube.HookOnlyStrategy
	upgrade.Timeout = operationTimeout
	upgrade.MaxHistory = historyMax
	if req.DryRun {
		upgrade.DryRunStrategy = action.DryRunClient
	}

	ctx, cancel := context.WithTimeout(context.Background(), operationTimeout)
	defer cancel()
	out, err := upgrade.RunWithContext(ctx, name, ch, vals)
	if err != nil {
		return nil, nil, wrapHelmError("upgrade", err)
	}
	rel, ok := out.(*release.Release)
	if !ok {
		return nil, nil, fmt.Errorf("upgrade returned an unexpected release type %T", out)
	}
	if req.DryRun {
		return nil, rendered(rel), nil
	}
	m.log.Info("helm upgrade", "release", name, "namespace", namespace, "revision", rel.Version)
	s := summarize(rel)
	return &s, nil, nil
}

// Rollback returns a release to an earlier revision.
func (m *Manager) Rollback(namespace, name string, revision int) error {
	if revision < 1 {
		return fmt.Errorf("%w: revision must be 1 or higher", ErrInvalid)
	}
	cfg, err := m.config(namespace)
	if err != nil {
		return err
	}
	rb := action.NewRollback(cfg)
	rb.Version = revision
	rb.WaitStrategy = kube.HookOnlyStrategy
	rb.Timeout = operationTimeout
	rb.ServerSideApply = "auto"
	rb.MaxHistory = historyMax
	if err := rb.Run(name); err != nil {
		return wrapHelmError("rollback", err)
	}
	m.log.Info("helm rollback", "release", name, "namespace", namespace, "revision", revision)
	return nil
}

// Uninstall removes a release and everything it created.
func (m *Manager) Uninstall(namespace, name string) error {
	cfg, err := m.config(namespace)
	if err != nil {
		return err
	}
	un := action.NewUninstall(cfg)
	un.WaitStrategy = kube.HookOnlyStrategy
	un.Timeout = operationTimeout
	if _, err := un.Run(name); err != nil {
		return wrapHelmError("uninstall", err)
	}
	m.log.Info("helm uninstall", "release", name, "namespace", namespace)
	return nil
}

// --- Helpers -------------------------------------------------------------------

func latest(cfg *action.Configuration, name string) (*release.Release, error) {
	out, err := action.NewGet(cfg).Run(name)
	if err != nil {
		return nil, wrapHelmError("get", err)
	}
	rel, ok := out.(*release.Release)
	if !ok {
		return nil, fmt.Errorf("unexpected release type %T", out)
	}
	return rel, nil
}

func summarize(r *release.Release) ReleaseSummary {
	s := ReleaseSummary{Name: r.Name, Namespace: r.Namespace, Revision: r.Version}
	if r.Info != nil {
		s.Status = string(r.Info.Status)
		s.Description = r.Info.Description
		s.Updated = r.Info.LastDeployed
	}
	if r.Chart != nil && r.Chart.Metadata != nil {
		s.Chart = r.Chart.Metadata.Name
		s.ChartVersion = r.Chart.Metadata.Version
		s.AppVersion = r.Chart.Metadata.AppVersion
		s.Icon = r.Chart.Metadata.Icon
	}
	return s
}

func rendered(r *release.Release) *Rendered {
	out := &Rendered{Manifest: r.Manifest}
	if r.Info != nil {
		out.Notes = r.Info.Notes
	}
	return out
}

// parseValues turns the edited YAML into values, reporting syntax errors with
// their line number.
func parseValues(text string) (map[string]any, error) {
	vals := map[string]any{}
	if strings.TrimSpace(text) == "" {
		return vals, nil
	}
	if err := yaml.Unmarshal([]byte(text), &vals); err != nil {
		return nil, fmt.Errorf("%w: the values are not valid YAML: %v", ErrInvalid, err)
	}
	return vals, nil
}

func toYAML(v map[string]any) string {
	if len(v) == 0 {
		return ""
	}
	out, err := yaml.Marshal(v)
	if err != nil {
		return ""
	}
	return string(out)
}

// resources lists the objects in a rendered manifest, in a stable order.
func resources(manifest, defaultNamespace string) []Resource {
	out := []Resource{}
	for _, doc := range releaseutil.SplitManifests(manifest) {
		var obj struct {
			Kind     string `json:"kind"`
			Metadata struct {
				Name      string `json:"name"`
				Namespace string `json:"namespace"`
			} `json:"metadata"`
		}
		if err := yaml.Unmarshal([]byte(doc), &obj); err != nil || obj.Kind == "" {
			continue
		}
		ns := obj.Metadata.Namespace
		if ns == "" {
			ns = defaultNamespace
		}
		out = append(out, Resource{Kind: obj.Kind, Name: obj.Metadata.Name, Namespace: ns})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func wrapHelmError(op string, err error) error {
	if errors.Is(err, driver.ErrReleaseNotFound) {
		return fmt.Errorf("%w: release not found", ErrNotFound)
	}
	msg := err.Error()
	// Errors that describe what the user asked for, not a broken cluster.
	for _, hint := range []string{"cannot re-use a name", "invalid release name", "parse error", "execution error", "values don't meet the specifications", "has no deployed releases", "is not a valid"} {
		if strings.Contains(msg, hint) {
			return fmt.Errorf("%w: %s failed: %s", ErrInvalid, op, msg)
		}
	}
	return fmt.Errorf("helm %s failed: %w", op, err)
}
