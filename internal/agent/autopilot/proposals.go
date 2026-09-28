package autopilot

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/kprompt/kprompt/internal/xdg"
)

const (
	// ProposalsConfigMapName is the in-cluster durable AutopilotProposal ring (RT-007).
	ProposalsConfigMapName = "kprompt-autopilot-proposals"
	ProposalsConfigMapKey  = "proposals.json"
	ProposalsKind          = "AutopilotProposalList"
	ProposalsMax           = 50
)

// ProposalSnapshot is the persisted document for a namespace.
type ProposalSnapshot struct {
	APIVersion    string     `json:"apiVersion"`
	Kind          string     `json:"kind"`
	SchemaVersion string     `json:"schemaVersion"`
	Namespace     string     `json:"namespace"`
	Proposals     []Proposal `json:"proposals"`
	UpdatedAt     time.Time  `json:"updatedAt"`
}

// ProposalStore loads/saves proposal snapshots (RT-007).
type ProposalStore interface {
	Load(namespace string) (ProposalSnapshot, error)
	Save(snap ProposalSnapshot) error
}

// ProposalLibrary is a thread-safe durable proposal ring.
type ProposalLibrary struct {
	mu    sync.Mutex
	store ProposalStore
}

// NewProposalLibrary wraps a ProposalStore.
func NewProposalLibrary(store ProposalStore) *ProposalLibrary {
	return &ProposalLibrary{store: store}
}

// Put upserts a proposal by ID (newest first).
func (l *ProposalLibrary) Put(prop Proposal) error {
	if l == nil || l.store == nil {
		return fmt.Errorf("autopilot proposals: library unset")
	}
	ns := strings.TrimSpace(prop.Namespace)
	if ns == "" || strings.TrimSpace(prop.ID) == "" {
		return fmt.Errorf("autopilot proposals: namespace and id required")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	snap, err := l.store.Load(ns)
	if err != nil {
		snap = emptyProposalSnapshot(ns)
	}
	now := time.Now().UTC()
	found := false
	for i, p := range snap.Proposals {
		if p.ID == prop.ID {
			snap.Proposals[i] = prop
			found = true
			break
		}
	}
	if !found {
		snap.Proposals = append([]Proposal{prop}, snap.Proposals...)
	}
	sort.SliceStable(snap.Proposals, func(i, j int) bool {
		return snap.Proposals[i].CreatedAt.After(snap.Proposals[j].CreatedAt)
	})
	if len(snap.Proposals) > ProposalsMax {
		snap.Proposals = snap.Proposals[:ProposalsMax]
	}
	snap.Namespace = ns
	snap.UpdatedAt = now
	snap.APIVersion = APIVersion
	snap.Kind = ProposalsKind
	snap.SchemaVersion = SchemaVersion
	return l.store.Save(snap)
}

// Get returns a proposal by id.
func (l *ProposalLibrary) Get(namespace, id string) (Proposal, error) {
	if l == nil || l.store == nil {
		return Proposal{}, fmt.Errorf("autopilot proposals: library unset")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	snap, err := l.store.Load(namespace)
	if err != nil {
		return Proposal{}, err
	}
	for _, p := range snap.Proposals {
		if p.ID == id {
			return p, nil
		}
	}
	return Proposal{}, fmt.Errorf("autopilot proposals: %q not found in %s", id, namespace)
}

// FindOpen returns the newest non-applied proposal for an incident+action (RT-017 stable id).
func (l *ProposalLibrary) FindOpen(namespace, incidentID, actionID string) (Proposal, bool) {
	if l == nil || l.store == nil {
		return Proposal{}, false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	snap, err := l.store.Load(namespace)
	if err != nil {
		return Proposal{}, false
	}
	for _, p := range snap.Proposals {
		if p.IncidentID == incidentID && p.ActionID == actionID &&
			!p.Applied && p.Decision != DecisionDenied && p.Decision != DecisionFailed {
			return p, true
		}
	}
	return Proposal{}, false
}

// List returns the snapshot for a namespace.
func (l *ProposalLibrary) List(namespace string) (ProposalSnapshot, error) {
	if l == nil || l.store == nil {
		return ProposalSnapshot{}, fmt.Errorf("autopilot proposals: library unset")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	snap, err := l.store.Load(namespace)
	if err != nil {
		return emptyProposalSnapshot(namespace), nil
	}
	return snap, nil
}

func emptyProposalSnapshot(ns string) ProposalSnapshot {
	return ProposalSnapshot{
		APIVersion:    APIVersion,
		Kind:          ProposalsKind,
		SchemaVersion: SchemaVersion,
		Namespace:     ns,
		UpdatedAt:     time.Now().UTC(),
	}
}

// DefaultProposalsDir returns $XDG_CONFIG_HOME/kprompt/proposals (or KPROMPT_PROPOSALS_DIR).
func DefaultProposalsDir() string {
	if d := strings.TrimSpace(os.Getenv("KPROMPT_PROPOSALS_DIR")); d != "" {
		return d
	}
	return xdg.KpromptDir("proposals")
}

// FileProposalStore persists one JSON file per namespace.
type FileProposalStore struct {
	Dir string
}

func (s FileProposalStore) path(ns string) string {
	safe := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, ns)
	return filepath.Join(s.Dir, safe+".json")
}

func (s FileProposalStore) Load(namespace string) (ProposalSnapshot, error) {
	b, err := os.ReadFile(s.path(namespace))
	if err != nil {
		return ProposalSnapshot{}, err
	}
	var snap ProposalSnapshot
	if err := json.Unmarshal(b, &snap); err != nil {
		return ProposalSnapshot{}, err
	}
	return snap, nil
}

func (s FileProposalStore) Save(snap ProposalSnapshot) error {
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path(snap.Namespace) + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path(snap.Namespace))
}

// MemProposalStore is an in-process store for tests.
type MemProposalStore struct {
	mu   sync.Mutex
	data map[string]ProposalSnapshot
}

func NewMemProposalStore() *MemProposalStore {
	return &MemProposalStore{data: map[string]ProposalSnapshot{}}
}

func (s *MemProposalStore) Load(namespace string) (ProposalSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	snap, ok := s.data[namespace]
	if !ok {
		return ProposalSnapshot{}, fmt.Errorf("proposals: empty")
	}
	return snap, nil
}

func (s *MemProposalStore) Save(snap ProposalSnapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data == nil {
		s.data = map[string]ProposalSnapshot{}
	}
	b, _ := json.Marshal(snap)
	var copy ProposalSnapshot
	_ = json.Unmarshal(b, &copy)
	s.data[snap.Namespace] = copy
	return nil
}

// ConfigMapProposalStore persists proposals in ProposalsConfigMapName (RT-007).
type ConfigMapProposalStore struct {
	Client    kubernetes.Interface
	Namespace string
}

func (s ConfigMapProposalStore) ns(namespace string) string {
	if strings.TrimSpace(s.Namespace) != "" {
		return s.Namespace
	}
	return namespace
}

func (s ConfigMapProposalStore) Load(namespace string) (ProposalSnapshot, error) {
	ns := s.ns(namespace)
	cm, err := s.Client.CoreV1().ConfigMaps(ns).Get(context.Background(), ProposalsConfigMapName, metav1.GetOptions{})
	if err != nil {
		return ProposalSnapshot{}, err
	}
	raw, ok := cm.Data[ProposalsConfigMapKey]
	if !ok || strings.TrimSpace(raw) == "" {
		return emptyProposalSnapshot(ns), nil
	}
	var snap ProposalSnapshot
	if err := json.Unmarshal([]byte(raw), &snap); err != nil {
		return ProposalSnapshot{}, err
	}
	return snap, nil
}

func (s ConfigMapProposalStore) Save(snap ProposalSnapshot) error {
	ns := s.ns(snap.Namespace)
	b, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}
	cm, err := s.Client.CoreV1().ConfigMaps(ns).Get(context.Background(), ProposalsConfigMapName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = s.Client.CoreV1().ConfigMaps(ns).Create(context.Background(), &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ProposalsConfigMapName,
				Namespace: ns,
				Labels: map[string]string{
					"app.kubernetes.io/name":       "kprompt-agent",
					"app.kubernetes.io/component":  "autopilot-proposals",
					"app.kubernetes.io/managed-by": "kprompt",
				},
			},
			Data: map[string]string{ProposalsConfigMapKey: string(b)},
		}, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	if cm.Data == nil {
		cm.Data = map[string]string{}
	}
	cm.Data[ProposalsConfigMapKey] = string(b)
	_, err = s.Client.CoreV1().ConfigMaps(ns).Update(context.Background(), cm, metav1.UpdateOptions{})
	return err
}
