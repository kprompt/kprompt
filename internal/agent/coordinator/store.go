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
)

// Snapshot is restart-safe recent handoff state (AG-060).
type Snapshot struct {
	SchemaVersion string   `json:"schemaVersion"`
	Records       []Record `json:"records"`
}

// Store persists the Coordinator recent ring (file or ConfigMap).
type Store interface {
	Load() (Snapshot, error)
	Save(Snapshot) error
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
