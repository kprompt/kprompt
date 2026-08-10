package coordinator

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	// ConfigMapName holds durable Shared Knowledge handoffs (AG-060).
	ConfigMapName = "kprompt-coordinator-knowledge"
	ConfigMapKey  = "handoffs.json"
	// OutcomeConfigMapKey holds the durable cross-ns outcome ring (RT-021).
	OutcomeConfigMapKey = "outcomes.json"
	// OutcomeFileName is the sibling file for the outcome ring (RT-021).
	OutcomeFileName = "outcomes.json"
)

// Snapshot is restart-safe recent handoff state (AG-060).
type Snapshot struct {
	SchemaVersion string   `json:"schemaVersion"`
	Records       []Record `json:"records"`
}

// OutcomeSnapshot is the restart-safe cross-ns outcome ring (RT-021).
type OutcomeSnapshot struct {
	SchemaVersion string          `json:"schemaVersion"`
	Outcomes      []OutcomeRecord `json:"outcomes"`
}

// Store persists the Coordinator recent ring (file or ConfigMap).
type Store interface {
	Load() (Snapshot, error)
	Save(Snapshot) error
}

// OutcomeStore persists the cross-ns outcome ring beside Shared Knowledge (RT-021).
type OutcomeStore interface {
	LoadOutcomes() (OutcomeSnapshot, error)
	SaveOutcomes(OutcomeSnapshot) error
}

// FileStore persists one JSON file (laptop / Kind without RBAC).
type FileStore struct {
	Path string
}

func (s FileStore) Load() (Snapshot, error) {
	path := strings.TrimSpace(s.Path)
	if path == "" {
		return Snapshot{SchemaVersion: SchemaVersion}, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Snapshot{SchemaVersion: SchemaVersion}, nil
		}
		return Snapshot{}, err
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return Snapshot{SchemaVersion: SchemaVersion}, nil
	}
	var snap Snapshot
	if err := json.Unmarshal(raw, &snap); err != nil {
		return Snapshot{}, err
	}
	if snap.SchemaVersion == "" {
		snap.SchemaVersion = SchemaVersion
	}
	return snap, nil
}

func (s FileStore) Save(snap Snapshot) error {
	path := strings.TrimSpace(s.Path)
	if path == "" {
		return fmt.Errorf("coordinator FileStore: empty path")
	}
	if snap.SchemaVersion == "" {
		snap.SchemaVersion = SchemaVersion
	}
	raw, err := json.Marshal(snap)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// outcomePath derives the sibling outcomes.json next to the handoffs file.
func (s FileStore) outcomePath() string {
	path := strings.TrimSpace(s.Path)
	if path == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(path), OutcomeFileName)
}

// LoadOutcomes reads the durable outcome ring (RT-021).
func (s FileStore) LoadOutcomes() (OutcomeSnapshot, error) {
	path := s.outcomePath()
	if path == "" {
		return OutcomeSnapshot{SchemaVersion: SchemaVersion}, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return OutcomeSnapshot{SchemaVersion: SchemaVersion}, nil
		}
		return OutcomeSnapshot{}, err
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return OutcomeSnapshot{SchemaVersion: SchemaVersion}, nil
	}
	var snap OutcomeSnapshot
	if err := json.Unmarshal(raw, &snap); err != nil {
		return OutcomeSnapshot{}, err
	}
	if snap.SchemaVersion == "" {
		snap.SchemaVersion = SchemaVersion
	}
	return snap, nil
}

// SaveOutcomes writes the durable outcome ring atomically (RT-021).
func (s FileStore) SaveOutcomes(snap OutcomeSnapshot) error {
	path := s.outcomePath()
	if path == "" {
		return fmt.Errorf("coordinator FileStore: empty path")
	}
	if snap.SchemaVersion == "" {
		snap.SchemaVersion = SchemaVersion
	}
	raw, err := json.Marshal(snap)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// ConfigMapStore persists handoffs in ConfigMapName (AG-060).
type ConfigMapStore struct {
	Client    kubernetes.Interface
	Namespace string
}

func (s ConfigMapStore) Load() (Snapshot, error) {
	ns := strings.TrimSpace(s.Namespace)
	if ns == "" {
		return Snapshot{}, fmt.Errorf("coordinator ConfigMapStore: namespace required")
	}
	cm, err := s.Client.CoreV1().ConfigMaps(ns).Get(context.Background(), ConfigMapName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return Snapshot{SchemaVersion: SchemaVersion}, nil
		}
		return Snapshot{}, err
	}
	raw, ok := cm.Data[ConfigMapKey]
	if !ok || strings.TrimSpace(raw) == "" {
		return Snapshot{SchemaVersion: SchemaVersion}, nil
	}
	var snap Snapshot
	if err := json.Unmarshal([]byte(raw), &snap); err != nil {
		return Snapshot{}, err
	}
	if snap.SchemaVersion == "" {
		snap.SchemaVersion = SchemaVersion
	}
	return snap, nil
}

func (s ConfigMapStore) Save(snap Snapshot) error {
	ns := strings.TrimSpace(s.Namespace)
	if ns == "" {
		return fmt.Errorf("coordinator ConfigMapStore: namespace required")
	}
	if snap.SchemaVersion == "" {
		snap.SchemaVersion = SchemaVersion
	}
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
					"app.kubernetes.io/name":       "kprompt-coordinator",
					"app.kubernetes.io/component":  "shared-knowledge",
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

// LoadOutcomes reads the outcome ring from the shared ConfigMap key (RT-021).
func (s ConfigMapStore) LoadOutcomes() (OutcomeSnapshot, error) {
	ns := strings.TrimSpace(s.Namespace)
	if ns == "" {
		return OutcomeSnapshot{}, fmt.Errorf("coordinator ConfigMapStore: namespace required")
	}
	cm, err := s.Client.CoreV1().ConfigMaps(ns).Get(context.Background(), ConfigMapName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return OutcomeSnapshot{SchemaVersion: SchemaVersion}, nil
		}
		return OutcomeSnapshot{}, err
	}
	raw, ok := cm.Data[OutcomeConfigMapKey]
	if !ok || strings.TrimSpace(raw) == "" {
		return OutcomeSnapshot{SchemaVersion: SchemaVersion}, nil
	}
	var snap OutcomeSnapshot
	if err := json.Unmarshal([]byte(raw), &snap); err != nil {
		return OutcomeSnapshot{}, err
	}
	if snap.SchemaVersion == "" {
		snap.SchemaVersion = SchemaVersion
	}
	return snap, nil
}

// SaveOutcomes writes the outcome ring under OutcomeConfigMapKey (RT-021).
// Coexists with Shared Knowledge in the same ConfigMap (separate data key).
func (s ConfigMapStore) SaveOutcomes(snap OutcomeSnapshot) error {
	ns := strings.TrimSpace(s.Namespace)
	if ns == "" {
		return fmt.Errorf("coordinator ConfigMapStore: namespace required")
	}
	if snap.SchemaVersion == "" {
		snap.SchemaVersion = SchemaVersion
	}
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
					"app.kubernetes.io/name":       "kprompt-coordinator",
					"app.kubernetes.io/component":  "shared-knowledge",
					"app.kubernetes.io/managed-by": "kprompt",
				},
			},
			Data: map[string]string{OutcomeConfigMapKey: string(raw)},
		}, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	if cm.Data == nil {
		cm.Data = map[string]string{}
	}
	cm.Data[OutcomeConfigMapKey] = string(raw)
	_, err = s.Client.CoreV1().ConfigMaps(ns).Update(context.Background(), cm, metav1.UpdateOptions{})
	return err
}
