package memory

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestDiscoverNilClient(t *testing.T) {
	if _, err := Discover(context.Background(), nil, "ns"); err == nil {
		t.Fatal("expected error for nil client")
	}
}

func TestDiscoverFindsDependencies(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "redis-master", Namespace: "ns"}},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "ns"},
			Spec: appsv1.DeploymentSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{
							Name:  "app",
							Image: "docker.io/library/postgres:15",
							Env: []corev1.EnvVar{
								{Name: "KAFKA_BROKERS", Value: "kafka:9092"},
							},
						}},
					},
				},
			},
		},
	)

	facts, err := Discover(context.Background(), client, "ns")
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	got := map[string]bool{}
	for _, f := range facts {
		if f.Kind != KindDependency {
			t.Errorf("unexpected kind %q", f.Kind)
		}
		got[f.Key] = true
	}
	for _, want := range []string{"redis", "postgres", "kafka"} {
		if !got[want] {
			t.Errorf("expected dependency %q discovered, facts=%+v", want, facts)
		}
	}
}

func TestMatchDependency(t *testing.T) {
	got := matchDependency("my-postgresql-primary")
	if len(got) != 1 || got[0] != "postgres" {
		t.Fatalf("postgresql should normalize to postgres, got %v", got)
	}
	if len(matchDependency("plain-nginx")) != 0 {
		t.Fatal("no known dep should match nginx")
	}
}

func TestMatchEnvDependency(t *testing.T) {
	got := matchEnvDependency("REDIS_URL", "redis://x")
	if !inTestList(got, "redis") {
		t.Fatalf("expected redis from env, got %v", got)
	}
	got = matchEnvDependency("DATABASE_URL", "postgres://db")
	if !inTestList(got, "postgres") {
		t.Fatalf("expected postgres from DATABASE_URL value, got %v", got)
	}
}

func TestNormalizeDepKey(t *testing.T) {
	cases := map[string]string{
		"postgresql":  "postgres",
		"mongodb":     "mongo",
		"mariadb":     "mysql",
		"opensearch":  "elasticsearch",
		"amqp":        "rabbitmq",
		"redis":       "redis",
	}
	for in, want := range cases {
		if got := normalizeDepKey(in); got != want {
			t.Errorf("normalizeDepKey(%q)=%q want %q", in, got, want)
		}
	}
}

func TestShortImage(t *testing.T) {
	cases := map[string]string{
		"docker.io/library/redis:7":        "redis",
		"redis@sha256:abc":                 "redis",
		"ghcr.io/org/app:tag":              "app",
		"postgres":                         "postgres",
	}
	for in, want := range cases {
		if got := shortImage(in); got != want {
			t.Errorf("shortImage(%q)=%q want %q", in, got, want)
		}
	}
}

func TestUniqueStrings(t *testing.T) {
	got := uniqueStrings([]string{"redis", "REDIS", " redis ", "", "postgresql"})
	if len(got) != 2 {
		t.Fatalf("expected 2 unique normalized, got %v", got)
	}
}

func inTestList(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
