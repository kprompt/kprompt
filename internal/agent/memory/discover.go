package memory

import (
	"context"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// ConfigMapStore persists facts in ConfigMapName within the watched namespace.
// The ConfigMap shell should be created by Helm; this store updates data only.
type ConfigMapStore struct {
	Client    kubernetes.Interface
	Namespace string
}

// Load reads facts.json from the ConfigMap.
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

// Save writes facts.json into the ConfigMap (create if missing).
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
					"app.kubernetes.io/component":  "namespace-memory",
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

// ListNamespaces enumerates namespaces holding a namespace-memory ConfigMap (RT-023).
// Lists across all namespaces by the managed-by/component labels; requires
// cluster-wide ConfigMap list RBAC (offline export/backup path only).
func (s ConfigMapStore) ListNamespaces() ([]string, error) {
	list, err := s.Client.CoreV1().ConfigMaps(metav1.NamespaceAll).List(context.Background(), metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/component=namespace-memory,app.kubernetes.io/managed-by=kprompt",
	})
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []string
	for _, cm := range list.Items {
		if cm.Name != ConfigMapName {
			continue
		}
		if seen[cm.Namespace] {
			continue
		}
		seen[cm.Namespace] = true
		out = append(out, cm.Namespace)
	}
	return out, nil
}

// Discover scans Services and Deployments for known dependency signals (read-only).
func Discover(ctx context.Context, client kubernetes.Interface, namespace string) ([]Fact, error) {
	if client == nil {
		return nil, fmt.Errorf("memory: client required")
	}
	now := time.Now().UTC()
	found := map[string]Fact{}

	svcs, err := client.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	for _, svc := range svcs.Items {
		for _, dep := range matchDependency(svc.Name) {
			id := KindDependency + "/" + dep
			found[id] = Fact{
				ID:        id,
				Kind:      KindDependency,
				Key:       dep,
				Value:     "service/" + svc.Name,
				Source:    "discover",
				Evidence:  "Service name hints at " + dep,
				UpdatedAt: now,
			}
		}
	}

	deps, err := client.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	for _, d := range deps.Items {
		for _, c := range d.Spec.Template.Spec.Containers {
			for _, dep := range matchDependency(c.Image) {
				id := KindDependency + "/" + dep
				found[id] = Fact{
					ID:        id,
					Kind:      KindDependency,
					Key:       dep,
					Value:     "image:" + shortImage(c.Image),
					Source:    "discover",
					Evidence:  "container image in Deployment/" + d.Name,
					UpdatedAt: now,
				}
			}
			for _, e := range c.Env {
				for _, dep := range matchEnvDependency(e.Name, e.Value) {
					id := KindDependency + "/" + dep
					found[id] = Fact{
						ID:        id,
						Kind:      KindDependency,
						Key:       dep,
						Value:     "env:" + e.Name,
						Source:    "discover",
						Evidence:  "env on Deployment/" + d.Name,
						UpdatedAt: now,
					}
				}
			}
		}
	}

	out := make([]Fact, 0, len(found))
	for _, f := range found {
		out = append(out, f)
	}
	return out, nil
}

var knownDeps = []string{
	"redis", "postgres", "postgresql", "mysql", "mariadb", "mongo", "mongodb",
	"kafka", "rabbitmq", "amqp", "nats", "elasticsearch", "opensearch",
	"memcached", "cassandra", "cockroach", "clickhouse",
}

func matchDependency(s string) []string {
	lower := strings.ToLower(s)
	var out []string
	for _, dep := range knownDeps {
		if strings.Contains(lower, dep) {
			out = append(out, normalizeDepKey(dep))
		}
	}
	return uniqueStrings(out)
}

func matchEnvDependency(name, value string) []string {
	n := strings.ToUpper(name)
	hints := map[string]string{
		"REDIS": "redis", "POSTGRES": "postgres", "PGHOST": "postgres",
		"MYSQL": "mysql", "MONGO": "mongo", "KAFKA": "kafka",
		"RABBIT": "rabbitmq", "AMQP": "rabbitmq", "NATS": "nats",
		"ELASTIC": "elasticsearch", "MEMCACHE": "memcached",
		"DATABASE_URL": "", "DB_HOST": "", "BROKER": "",
	}
	var out []string
	for needle, dep := range hints {
		if strings.Contains(n, needle) {
			if dep != "" {
				out = append(out, dep)
			} else {
				out = append(out, matchDependency(name+" "+value)...)
			}
		}
	}
	out = append(out, matchDependency(value)...)
	return uniqueStrings(out)
}

func normalizeDepKey(dep string) string {
	switch dep {
	case "postgresql":
		return "postgres"
	case "mongodb":
		return "mongo"
	case "mariadb":
		return "mysql"
	case "opensearch":
		return "elasticsearch"
	case "amqp":
		return "rabbitmq"
	default:
		return dep
	}
}

func shortImage(img string) string {
	if i := strings.LastIndex(img, "/"); i >= 0 {
		img = img[i+1:]
	}
	if i := strings.Index(img, "@"); i >= 0 {
		img = img[:i]
	}
	if i := strings.Index(img, ":"); i >= 0 {
		img = img[:i]
	}
	return img
}

func uniqueStrings(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		s = normalizeDepKey(strings.ToLower(strings.TrimSpace(s)))
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}
