// Package memory persists namespace dependency facts for Observe context (AG-015).
//
// Privacy: stores stay local (file) or in-cluster (ConfigMap). Facts are never
// uploaded to api.kprompt.ai by default.
package memory

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kprompt/kprompt/internal/xdg"
)

const (
	KindSnapshot  = "NamespaceMemory"
	APIVersion    = "kprompt.io/v1"
	SchemaVersion = "1"

	// ConfigMapName is the optional in-cluster store (created by Helm / operator).
	ConfigMapName = "kprompt-namespace-memory"
	ConfigMapKey  = "facts.json"
)

// Fact kinds.
const (
	KindDependency = "dependency" // redis, kafka, postgres, …
	KindNote       = "note"       // free-form operator note
)

// Fact is one remembered namespace attribute.
type Fact struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"` // dependency | note
	Key       string    `json:"key"`  // e.g. redis, postgres
	Value     string    `json:"value,omitempty"`
	Source    string    `json:"source,omitempty"` // discover | manual
	Evidence  string    `json:"evidence,omitempty"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Snapshot is the on-disk / ConfigMap document.
type Snapshot struct {
	APIVersion    string    `json:"apiVersion"`
	Kind          string    `json:"kind"`
	SchemaVersion string    `json:"schemaVersion"`
	Namespace     string    `json:"namespace"`
	Facts         []Fact    `json:"facts"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

// Store persists facts for one or more namespaces.
type Store interface {
	Load(namespace string) (Snapshot, error)
	Save(snap Snapshot) error
}

// Memory is a thread-safe façade over a Store with merge helpers.
type Memory struct {
	mu    sync.Mutex
	store Store
}

// New wraps a Store.
func New(store Store) *Memory {
	return &Memory{store: store}
}

// Upsert merges facts by id (kind/key).
func (m *Memory) Upsert(namespace string, facts ...Fact) (Snapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	snap, err := m.store.Load(namespace)
	if err != nil {
		snap = emptySnapshot(namespace)
	}
	byID := map[string]Fact{}
	for _, f := range snap.Facts {
		byID[f.ID] = f
	}
	now := time.Now().UTC()
	for _, f := range facts {
		f = normalizeFact(f, now)
		byID[f.ID] = f
	}
	snap.Facts = sortedFacts(byID)
	snap.UpdatedAt = now
	snap.Namespace = namespace
	if err := m.store.Save(snap); err != nil {
		return Snapshot{}, err
	}
	return snap, nil
}

// List returns all facts for a namespace.
func (m *Memory) List(namespace string) (Snapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	snap, err := m.store.Load(namespace)
	if err != nil {
		return emptySnapshot(namespace), nil
	}
	return snap, nil
}

// Relevant returns facts that may help analyze the incident text.
func Relevant(facts []Fact, incidentText string) []Fact {
	text := strings.ToLower(incidentText)
	var out []Fact
	for _, f := range facts {
		key := strings.ToLower(f.Key)
		val := strings.ToLower(f.Value)
		if key != "" && strings.Contains(text, key) {
			out = append(out, f)
			continue
		}
		if val != "" && strings.Contains(text, val) {
			out = append(out, f)
			continue
		}
		// Always include deps when incident mentions connection/timeout/auth patterns
		// and the fact is a dependency — analyzer benefits from known stack.
		if f.Kind == KindDependency && mentionsInfra(text) {
			out = append(out, f)
		}
	}
	return dedupeFacts(out)
}

func mentionsInfra(text string) bool {
	needles := []string{
		"timeout", "connection", "connect", "refused", "auth", "password",
		"dns", "upstream", "broker", "queue", "database", "db ", "redis",
		"kafka", "postgres", "mysql", "mongo", "amqp", "elasticsearch",
	}
	for _, n := range needles {
		if strings.Contains(text, n) {
			return true
		}
	}
	return false
}

func normalizeFact(f Fact, now time.Time) Fact {
	f.Kind = strings.TrimSpace(strings.ToLower(f.Kind))
	if f.Kind == "" {
		f.Kind = KindNote
	}
	f.Key = strings.TrimSpace(strings.ToLower(f.Key))
	f.Value = strings.TrimSpace(f.Value)
	if f.ID == "" {
		f.ID = f.Kind + "/" + f.Key
		if f.Key == "" {
			f.ID = f.Kind + "/" + shortHash(f.Value)
		}
	}
	if f.UpdatedAt.IsZero() {
		f.UpdatedAt = now
	}
	if f.Source == "" {
		f.Source = "manual"
	}
	return f
}

func emptySnapshot(ns string) Snapshot {
	return Snapshot{
		APIVersion:    APIVersion,
		Kind:          KindSnapshot,
		SchemaVersion: SchemaVersion,
		Namespace:     ns,
		Facts:         nil,
		UpdatedAt:     time.Now().UTC(),
	}
}

func sortedFacts(byID map[string]Fact) []Fact {
	out := make([]Fact, 0, len(byID))
	for _, f := range byID {
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Key < out[j].Key
	})
	return out
}

func dedupeFacts(in []Fact) []Fact {
	seen := map[string]bool{}
	var out []Fact
	for _, f := range in {
		if seen[f.ID] {
			continue
		}
		seen[f.ID] = true
		out = append(out, f)
	}
	return out
}

func shortHash(s string) string {
	if s == "" {
		return "empty"
	}
	h := 0
	for _, r := range s {
		h = 31*h + int(r)
	}
	if h < 0 {
		h = -h
	}
	return fmt.Sprintf("%x", h&0xffff)
}

// DefaultDir returns $XDG_CONFIG_HOME/kprompt/memory (or KPROMPT_MEMORY_DIR).
func DefaultDir() string {
	if d := strings.TrimSpace(os.Getenv("KPROMPT_MEMORY_DIR")); d != "" {
		return d
	}
	return xdg.KpromptDir("memory")
}

// Encode / Decode helpers for ConfigMap payloads.
func Encode(snap Snapshot) ([]byte, error) {
	snap.APIVersion = APIVersion
	snap.Kind = KindSnapshot
	snap.SchemaVersion = SchemaVersion
	return json.MarshalIndent(snap, "", "  ")
}

func Decode(b []byte) (Snapshot, error) {
	var snap Snapshot
	if err := json.Unmarshal(b, &snap); err != nil {
		return Snapshot{}, err
	}
	if snap.Kind == "" {
		snap.Kind = KindSnapshot
	}
	return snap, nil
}
