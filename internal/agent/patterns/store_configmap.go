package patterns

import (
	"context"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// ConfigMapStore persists incident patterns in ConfigMapName (AG-054 / in-cluster Incident Memory).
type ConfigMapStore struct {
	Client    kubernetes.Interface
	Namespace string
}

func (s ConfigMapStore) Load(namespace string) (Snapshot, error) {
	ns := s.ns(namespace)
	cm, err := s.Client.CoreV1().ConfigMaps(ns).Get(context.Background(), ConfigMapName, metav1.GetOptions{})
	if err != nil {
		return Snapshot{}, err
	}
	raw, ok := cm.Data[ConfigMapKey]
	if !ok || strings.TrimSpace(raw) == "" {
		return emptySnapshot(ns), nil
	}
	return Decode([]byte(raw))
}

func (s ConfigMapStore) Save(snap Snapshot) error {
	ns := s.ns(snap.Namespace)
	b, err := Encode(snap)
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
					"app.kubernetes.io/component":  "incident-memory",
					"app.kubernetes.io/managed-by": "kprompt",
				},
			},
			Data: map[string]string{ConfigMapKey: string(b)},
		}, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	if cm.Data == nil {
		cm.Data = map[string]string{}
	}
	cm.Data[ConfigMapKey] = string(b)
	_, err = s.Client.CoreV1().ConfigMaps(ns).Update(context.Background(), cm, metav1.UpdateOptions{})
	return err
}

func (s ConfigMapStore) ns(namespace string) string {
	if strings.TrimSpace(s.Namespace) != "" {
		return s.Namespace
	}
	return namespace
}
