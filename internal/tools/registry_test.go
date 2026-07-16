package tools

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/kprompt/kprompt/internal/cluster"
	"github.com/kprompt/kprompt/internal/config"
	"k8s.io/client-go/rest"
)

type fakeKube struct {
	cl *cluster.Clients
	err error
}

func (f fakeKube) Connect(string) (*cluster.Clients, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.cl, nil
}

func TestLoadSettingsEnvOverridesFile(t *testing.T) {
	t.Setenv(EnvPrometheusURL, "http://prom.example")
	f := config.File{Tools: config.ToolsFile{Prometheus: config.PrometheusTool{URL: "http://file"}}}
	s := LoadSettings(f)
	if s.PrometheusURL != "http://prom.example" {
		t.Fatalf("url = %q", s.PrometheusURL)
	}
}

func TestLoadSettingsDisableHelm(t *testing.T) {
	t.Setenv(EnvHelmEnabled, "0")
	s := LoadSettings(config.File{})
	if s.HelmEnabled {
		t.Fatal("expected helm disabled")
	}
}

func TestDetectKubernetesUnavailable(t *testing.T) {
	reg, err := Detect(context.Background(), DetectOptions{
		Kube: fakeKube{err: errors.New("no kubeconfig")},
	})
	if err != nil {
		t.Fatal(err)
	}
	r, ok := reg.Get(IDKubernetes)
	if !ok {
		t.Fatal("missing kubernetes")
	}
	if r.Status != StatusUnavailable {
		t.Fatalf("status = %s", r.Status)
	}
	if r.Hint == "" {
		t.Fatal("expected hint")
	}
}

func TestDetectHelmOnPath(t *testing.T) {
	reg, err := Detect(context.Background(), DetectOptions{
		Kube: fakeKube{cl: &cluster.Clients{Context: "test", Config: &rest.Config{Host: "https://example"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	r, ok := reg.Get(IDHelm)
	if !ok {
		t.Fatal("missing helm")
	}
	// helm may or may not be on PATH in CI — just ensure shape
	if r.ID != IDHelm {
		t.Fatalf("id = %s", r.ID)
	}
	if r.Status == StatusDisabled {
		t.Fatal("helm should not be disabled by default")
	}
}

func TestDetectPrometheusConfiguredWithoutURL(t *testing.T) {
	reg, err := Detect(context.Background(), DetectOptions{
		File: config.File{},
		Kube: fakeKube{err: fmt.Errorf("skip k8s")},
	})
	if err != nil {
		t.Fatal(err)
	}
	r, _ := reg.Get(IDPrometheus)
	if r.Status != StatusUnavailable {
		t.Fatalf("status = %s", r.Status)
	}
	if r.Hint != MissingHint(IDPrometheus) {
		t.Fatalf("hint = %q", r.Hint)
	}
}

func TestRegistryAvailable(t *testing.T) {
	reg := NewRegistry([]Result{{ID: IDHelm, Status: StatusAvailable}})
	if !reg.Available(IDHelm) {
		t.Fatal("expected available")
	}
	reg = NewRegistry([]Result{{ID: IDHelm, Status: StatusUnavailable}})
	if reg.Available(IDHelm) {
		t.Fatal("expected unavailable")
	}
}
