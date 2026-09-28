package correlate

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/kprompt/kprompt/internal/incident"
	"github.com/kprompt/kprompt/internal/xdg"
)

const (
	SnapshotKind       = "IncidentState"
	SnapshotAPIVersion = "kprompt.io/v1"
	SnapshotSchema     = "1"

	// ConfigMapName is the in-cluster durable store (AG-032).
	ConfigMapName = "kprompt-incident-state"
	ConfigMapKey  = "state.json"

	maxEvidencePerIncident = 40
)

// Snapshot is restart-safe correlate state (file or ConfigMap).
type Snapshot struct {
	APIVersion    string                       `json:"apiVersion"`
	Kind          string                       `json:"kind"`
	SchemaVersion string                       `json:"schemaVersion"`
	Namespace     string                       `json:"namespace"`
	Seq           int                          `json:"seq"`
	Open          map[string]incident.Incident `json:"open"`
	Recent        map[string]ClosedRefJSON     `json:"recent,omitempty"`
	Dedupe        map[string]time.Time         `json:"dedupe,omitempty"`
	UpdatedAt     time.Time                    `json:"updatedAt"`
}

// ClosedRefJSON is the persisted form of closedRef.
type ClosedRefJSON struct {
	At time.Time `json:"at"`
	ID string    `json:"id"`
}

// Store persists correlate snapshots (AG-032).
type Store interface {
	Load(namespace string) (Snapshot, error)
	Save(snap Snapshot) error
}

// Export copies in-memory state into a Snapshot.
func (b *Builder) Export() Snapshot {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.exportLocked()
}

func (b *Builder) exportLocked() Snapshot {
	now := b.opts.Now()
	snap := Snapshot{
		APIVersion:    SnapshotAPIVersion,
		Kind:          SnapshotKind,
		SchemaVersion: SnapshotSchema,
		Namespace:     b.opts.Namespace,
		Seq:           b.seq,
		Open:          map[string]incident.Incident{},
		Recent:        map[string]ClosedRefJSON{},
		Dedupe:        map[string]time.Time{},
		UpdatedAt:     now,
	}
	for k, inc := range b.open {
		if inc == nil {
			continue
		}
		cp := *inc
		if len(cp.Evidence) > maxEvidencePerIncident {
			cp.Evidence = append([]incident.EvidenceRef(nil), cp.Evidence[len(cp.Evidence)-maxEvidencePerIncident:]...)
		}
		snap.Open[k] = cp
	}
	for k, r := range b.recent {
		snap.Recent[k] = ClosedRefJSON{At: r.at, ID: r.id}
	}
	for k, t := range b.dedupe {
		snap.Dedupe[k] = t
	}
	return snap
}

// Restore loads a Snapshot into the builder (replaces open/recent/dedupe/seq).
func (b *Builder) Restore(snap Snapshot) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if snap.Kind != "" && snap.Kind != SnapshotKind {
		return fmt.Errorf("correlate: unexpected snapshot kind %q", snap.Kind)
	}
	b.open = map[string]*incident.Incident{}
	for k, inc := range snap.Open {
		cp := inc
		b.open[k] = &cp
	}
	b.recent = map[string]closedRef{}
	for k, r := range snap.Recent {
		b.recent[k] = closedRef{at: r.At, id: r.ID}
	}
	b.dedupe = map[string]time.Time{}
	for k, t := range snap.Dedupe {
		b.dedupe[k] = t
	}
	b.seq = snap.Seq
	if strings.TrimSpace(snap.Namespace) != "" && b.opts.Namespace == "" {
		b.opts.Namespace = snap.Namespace
	}
	return nil
}

// Persist writes current state via Store when configured.
func (b *Builder) Persist() error {
	if b == nil || b.store == nil {
		return nil
	}
	b.mu.Lock()
	snap := b.exportLocked()
	b.mu.Unlock()
	return b.store.Save(snap)
}

// SetStore attaches a durable backend (AG-032).
func (b *Builder) SetStore(store Store) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.store = store
}

// FileStore persists one JSON file per namespace.
type FileStore struct {
	Dir string
}

func (s FileStore) path(ns string) string {
	safe := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, ns)
	return filepath.Join(s.Dir, safe+".json")
}

// DefaultIncidentsDir is $XDG_CONFIG_HOME/kprompt/incidents.
func DefaultIncidentsDir() string {
	return xdg.KpromptDir("incidents")
}

func (s FileStore) Load(namespace string) (Snapshot, error) {
	raw, err := os.ReadFile(s.path(namespace))
	if err != nil {
		return Snapshot{}, err
	}
	var snap Snapshot
	if err := json.Unmarshal(raw, &snap); err != nil {
		return Snapshot{}, err
	}
	return snap, nil
}

func (s FileStore) Save(snap Snapshot) error {
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path(snap.Namespace) + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path(snap.Namespace))
}

// ConfigMapStore persists state in ConfigMapName within the watched namespace.
type ConfigMapStore struct {
	Client    kubernetes.Interface
	Namespace string
}

func (s ConfigMapStore) ns(namespace string) string {
	if strings.TrimSpace(s.Namespace) != "" {
		return s.Namespace
	}
	return namespace
}

func (s ConfigMapStore) Load(namespace string) (Snapshot, error) {
	ns := s.ns(namespace)
	cm, err := s.Client.CoreV1().ConfigMaps(ns).Get(context.Background(), ConfigMapName, metav1.GetOptions{})
	if err != nil {
		return Snapshot{}, err
	}
	raw, ok := cm.Data[ConfigMapKey]
	if !ok || strings.TrimSpace(raw) == "" {
		return Snapshot{Namespace: ns, Open: map[string]incident.Incident{}}, nil
	}
	var snap Snapshot
	if err := json.Unmarshal([]byte(raw), &snap); err != nil {
		return Snapshot{}, err
	}
	return snap, nil
}

func (s ConfigMapStore) Save(snap Snapshot) error {
	ns := s.ns(snap.Namespace)
	raw, err := json.Marshal(snap)
	if err != nil {
		return err
	}
	cm, err := s.Client.CoreV1().ConfigMaps(ns).Get(context.Background(), ConfigMapName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = s.Client.CoreV1().ConfigMaps(ns).Create(context.Background(), &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ConfigMapName,
				Namespace: ns,
				Labels: map[string]string{
					"app.kubernetes.io/name":       "kprompt-agent",
					"app.kubernetes.io/component":  "incident-state",
					"app.kubernetes.io/managed-by": "kprompt",
				},
			},
			Data: map[string]string{ConfigMapKey: string(raw)},
		}, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	if cm.Data == nil {
		cm.Data = map[string]string{}
	}
	cm.Data[ConfigMapKey] = string(raw)
	_, err = s.Client.CoreV1().ConfigMaps(ns).Update(context.Background(), cm, metav1.UpdateOptions{})
	return err
}
