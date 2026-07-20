package tools

import (
	"context"
	"sync"

	"github.com/kprompt/kprompt/internal/config"
)

// Registry holds the latest detect results keyed by tool ID.
type Registry struct {
	mu    sync.RWMutex
	tools map[ID]Result
}

// NewRegistry builds a registry from detect results.
func NewRegistry(results []Result) *Registry {
	m := make(map[ID]Result, len(results))
	for _, r := range results {
		m[r.ID] = r
	}
	return &Registry{tools: m}
}

// All returns results in stable display order.
func (reg *Registry) All() []Result {
	reg.mu.RLock()
	defer reg.mu.RUnlock()
	order := []ID{
		IDKubernetes,
		IDHelm,
		IDArgoWorkflows,
		IDTekton,
		IDKEDA,
		IDIstio,
		IDCrossplane,
		IDGitOps,
		IDPrometheus,
		IDGrafana,
		IDOpenTelemetry,
	}
	out := make([]Result, 0, len(order))
	for _, id := range order {
		if r, ok := reg.tools[id]; ok {
			out = append(out, r)
		}
	}
	return out
}

// Get returns one tool result.
func (reg *Registry) Get(id ID) (Result, bool) {
	reg.mu.RLock()
	defer reg.mu.RUnlock()
	r, ok := reg.tools[id]
	return r, ok
}

// Available reports whether the tool is usable (available or configured for URL-only backends).
func (reg *Registry) Available(id ID) bool {
	r, ok := reg.Get(id)
	if !ok {
		return false
	}
	return r.Available()
}

// DetectOptions configures capability detection.
type DetectOptions struct {
	Context string // kube context override
	File    config.File
	Kube    kubeConnector // nil uses real cluster.Connect
}

// Detect probes local binaries, config URLs, and the active cluster.
func Detect(ctx context.Context, opts DetectOptions) (*Registry, error) {
	settings := LoadSettings(opts.File)
	k := opts.Kube
	if k == nil {
		k = defaultKube{}
	}
	results := []Result{
		detectKubernetes(ctx, opts.Context, k),
		detectHelm(settings),
		detectArgoWorkflows(ctx, settings, opts.Context, k),
		detectTekton(ctx, settings, opts.Context, k),
		detectKeda(ctx, settings, opts.Context, k),
		detectIstio(ctx, settings, opts.Context, k),
		detectCrossplane(ctx, settings, opts.Context, k),
		detectGitOps(ctx, settings, opts.Context, k),
		detectPrometheus(ctx, settings),
		detectGrafana(ctx, settings),
		detectOTel(settings),
	}
	return NewRegistry(results), nil
}
