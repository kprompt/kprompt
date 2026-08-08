package coordinator

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/kprompt/kprompt/internal/agent/handoff"
	"k8s.io/client-go/kubernetes/fake"
)

func TestRecordOutcomeValidation(t *testing.T) {
	svc := New()
	if err := svc.RecordOutcome(OutcomeRecord{Namespace: "payments", Action: "scale"}); err == nil {
		t.Fatal("want error for missing result")
	}
	if err := svc.RecordOutcome(OutcomeRecord{Action: "scale", Result: "apply_success"}); err == nil {
		t.Fatal("want error for missing namespace")
	}
	if err := svc.RecordOutcome(OutcomeRecord{Namespace: "payments", Result: "apply_success"}); err == nil {
		t.Fatal("want error for missing action")
	}
	if err := svc.RecordOutcome(OutcomeRecord{Namespace: "payments", Action: "scale", Result: "apply_success"}); err != nil {
		t.Fatalf("valid record: %v", err)
	}
	if got := len(svc.Outcomes()); got != 1 {
		t.Fatalf("outcomes=%d", got)
	}
}

func TestOutcomeFileStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "handoffs.json")
	svc := New()
	svc.Store = FileStore{Path: path}
	svc.PersistErrLog = func(err error) { t.Fatalf("persist: %v", err) }

	if err := svc.RecordOutcome(OutcomeRecord{Namespace: "payments", Action: "rollout-restart", Result: "apply_success"}); err != nil {
		t.Fatal(err)
	}
	if err := svc.RecordOutcome(OutcomeRecord{Namespace: "web", Action: "scale", Result: "apply_failed"}); err != nil {
		t.Fatal(err)
	}

	svc2 := New()
	svc2.Store = FileStore{Path: path}
	if err := svc2.Restore(); err != nil {
		t.Fatal(err)
	}
	if got := len(svc2.Outcomes()); got != 2 {
		t.Fatalf("restored outcomes=%d", got)
	}
	sum := svc2.OutcomeSummarize()
	if !sum.Durable {
		t.Fatal("want durable summary")
	}
	if sum.ByResult["apply_success"] != 1 || sum.ByResult["apply_failed"] != 1 {
		t.Fatalf("byResult=%+v", sum.ByResult)
	}
}

func TestOutcomeConfigMapRoundTripCoexists(t *testing.T) {
	client := fake.NewSimpleClientset()
	store := ConfigMapStore{Client: client, Namespace: "kprompt-system"}
	svc := New()
	svc.Store = store
	// Shared Knowledge handoff + outcome should coexist in the same ConfigMap.
	if _, err := svc.Handle(context.Background(), handoff.New("a", "b", "r", sampleReport("a", "sum"))); err != nil {
		t.Fatal(err)
	}
	if err := svc.RecordOutcome(OutcomeRecord{Namespace: "a", Action: "scale", Result: "apply_success"}); err != nil {
		t.Fatal(err)
	}

	svc2 := New()
	svc2.Store = store
	if err := svc2.Restore(); err != nil {
		t.Fatal(err)
	}
	if got := len(svc2.Recent()); got != 1 {
		t.Fatalf("handoffs=%d", got)
	}
	if got := len(svc2.Outcomes()); got != 1 {
		t.Fatalf("outcomes=%d", got)
	}
}

func TestPruneOutcomesTTL(t *testing.T) {
	svc := New()
	svc.SetOutcomeLimits(0, time.Hour)
	svc.mu.Lock()
	svc.outcomes = []OutcomeRecord{
		{Namespace: "a", Action: "x", Result: "apply_success", At: time.Now().UTC().Add(-2 * time.Hour)},
		{Namespace: "b", Action: "y", Result: "apply_failed", At: time.Now().UTC()},
	}
	svc.pruneOutcomesLocked()
	got := len(svc.outcomes)
	svc.mu.Unlock()
	if got != 1 {
		t.Fatalf("after TTL prune=%d, want 1", got)
	}
}

func TestPruneOutcomesCap(t *testing.T) {
	svc := New()
	svc.SetOutcomeLimits(3, 0)
	for i := 0; i < 10; i++ {
		if err := svc.RecordOutcome(OutcomeRecord{Namespace: "a", Action: "x", Result: "apply_success"}); err != nil {
			t.Fatal(err)
		}
	}
	if got := len(svc.Outcomes()); got != 3 {
		t.Fatalf("after cap=%d, want 3", got)
	}
}
