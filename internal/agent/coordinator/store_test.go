package coordinator

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/kprompt/kprompt/internal/agent/handoff"
	"k8s.io/client-go/kubernetes/fake"
)

func TestFileStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "handoffs.json")
	svc := New()
	svc.Store = FileStore{Path: path}
	svc.PersistErrLog = func(err error) { t.Fatalf("persist: %v", err) }

	_, err := svc.Handle(context.Background(), handoff.New("payments", "platform", "dep", sampleReport("payments", "timeout")))
	if err != nil {
		t.Fatal(err)
	}

	svc2 := New()
	svc2.Store = FileStore{Path: path}
	if err := svc2.Restore(); err != nil {
		t.Fatal(err)
	}
	if len(svc2.Recent()) != 1 {
		t.Fatalf("restored=%d", len(svc2.Recent()))
	}
	k := svc2.Knowledge()
	if !k.Durable || k.HandoffCount != 1 {
		t.Fatalf("%+v", k)
	}
}

func TestConfigMapStoreRoundTrip(t *testing.T) {
	client := fake.NewSimpleClientset()
	store := ConfigMapStore{Client: client, Namespace: "kprompt-system"}
	svc := New()
	svc.Store = store
	_, err := svc.Handle(context.Background(), handoff.New("a", "b", "r", sampleReport("a", "sum")))
	if err != nil {
		t.Fatal(err)
	}
	svc2 := New()
	svc2.Store = store
	if err := svc2.Restore(); err != nil {
		t.Fatal(err)
	}
	if len(svc2.Recent()) != 1 {
		t.Fatalf("restored=%d", len(svc2.Recent()))
	}
}
