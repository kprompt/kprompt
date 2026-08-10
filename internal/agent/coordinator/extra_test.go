package coordinator

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/kprompt/kprompt/internal/agent/handoff"
	"github.com/kprompt/kprompt/internal/incident"
)

func TestMaxHelper(t *testing.T) {
	if max(3, 5) != 5 || max(5, 3) != 5 || max(2, 2) != 2 {
		t.Fatal("max broken")
	}
}

func TestAppendAuditCap(t *testing.T) {
	var nilS *Service
	nilS.appendAudit(AuditEntry{}) // no panic
	if nilS.Audit() != nil {
		t.Fatal("nil audit")
	}
	s := New()
	for i := 0; i < maxAuditKeep+10; i++ {
		s.appendAudit(AuditEntry{Kind: "handoff"})
	}
	if got := len(s.Audit()); got != maxAuditKeep {
		t.Fatalf("audit cap=%d want %d", got, maxAuditKeep)
	}
}

func TestRunTickerNoop(t *testing.T) {
	// interval<=0 and nil service both return immediately.
	RunTicker(context.Background(), New(), TickConfig{Interval: 0}, nil)
	RunTicker(context.Background(), nil, TickConfig{Interval: time.Millisecond}, nil)
}

func TestRunTickerCancels(t *testing.T) {
	s := New()
	// seed one complete edge so a tick does real work.
	env := handoff.Envelope{
		APIVersion: handoff.APIVersion, Kind: handoff.Kind, SchemaVersion: handoff.SchemaVersion,
		FromNamespace: "a", SuspectNamespace: "b", Reason: "x",
		Report: incident.InvestigationReport{
			APIVersion: incident.APIVersion, Kind: incident.KindInvestigationReport,
			SchemaVersion: incident.SchemaVersion2, Namespace: "a", Summary: "seed",
		},
	}
	if _, err := s.Handle(context.Background(), env); err != nil {
		t.Fatalf("seed handle: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	var mu sync.Mutex
	ticks := 0
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		RunTicker(ctx, s, TickConfig{Interval: 2 * time.Millisecond, Budget: 3}, func(string, ...any) {
			mu.Lock()
			ticks++
			mu.Unlock()
		})
	}()
	time.Sleep(30 * time.Millisecond)
	cancel()
	wg.Wait()
	mu.Lock()
	defer mu.Unlock()
	if ticks == 0 {
		t.Fatal("expected at least one tick log")
	}
}

func TestTickSkipsIncompleteEdges(t *testing.T) {
	s := New()
	// A handoff with no suspect namespace produces an edge that Tick must skip.
	env := handoff.Envelope{
		APIVersion: handoff.APIVersion, Kind: handoff.Kind, SchemaVersion: handoff.SchemaVersion,
		FromNamespace: "solo", Reason: "x",
		Report: incident.InvestigationReport{
			APIVersion: incident.APIVersion, Kind: incident.KindInvestigationReport,
			SchemaVersion: incident.SchemaVersion2, Namespace: "solo", Summary: "no suspect",
		},
	}
	if _, err := s.Handle(context.Background(), env); err != nil {
		t.Fatalf("handle: %v", err)
	}
	res := s.Tick(context.Background(), TickConfig{Budget: 1})
	if res.Probed != 0 || res.Skipped == 0 {
		t.Fatalf("expected skip on incomplete edge, got %+v", res)
	}
	// nil service Tick is a safe no-op.
	var nilS *Service
	if got := nilS.Tick(context.Background(), TickConfig{}); got.EdgesConsidered != 0 {
		t.Fatalf("nil tick: %+v", got)
	}
}

func TestServiceRestore(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "handoffs.json")

	writer := New()
	writer.Store = FileStore{Path: path}
	env := handoff.Envelope{
		APIVersion: handoff.APIVersion, Kind: handoff.Kind, SchemaVersion: handoff.SchemaVersion,
		FromNamespace: "a", SuspectNamespace: "b", Reason: "x",
		Report: incident.InvestigationReport{
			APIVersion: incident.APIVersion, Kind: incident.KindInvestigationReport,
			SchemaVersion: incident.SchemaVersion2, Namespace: "a", Summary: "seed",
		},
	}
	if _, err := writer.Handle(context.Background(), env); err != nil {
		t.Fatalf("handle: %v", err)
	}

	reader := New()
	reader.Store = FileStore{Path: path}
	if err := reader.Restore(); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if len(reader.Recent()) != 1 {
		t.Fatalf("expected 1 restored record, got %d", len(reader.Recent()))
	}
	// nil service / nil store Restore is a no-op.
	var nilS *Service
	if err := nilS.Restore(); err != nil {
		t.Fatalf("nil restore: %v", err)
	}
}

func TestStoreErrorPaths(t *testing.T) {
	// FileStore empty path: Load ok (empty), Save error.
	fs := FileStore{}
	if _, err := fs.Load(); err != nil {
		t.Fatalf("empty-path load: %v", err)
	}
	if err := fs.Save(Snapshot{}); err == nil {
		t.Fatal("empty-path save should error")
	}
	if _, err := fs.LoadOutcomes(); err != nil {
		t.Fatalf("empty-path load outcomes: %v", err)
	}
	if err := fs.SaveOutcomes(OutcomeSnapshot{}); err == nil {
		t.Fatal("empty-path save outcomes should error")
	}

	// ConfigMapStore without namespace: every method errors.
	cs := ConfigMapStore{}
	if _, err := cs.Load(); err == nil {
		t.Fatal("cm load without ns should error")
	}
	if err := cs.Save(Snapshot{}); err == nil {
		t.Fatal("cm save without ns should error")
	}
	if _, err := cs.LoadOutcomes(); err == nil {
		t.Fatal("cm load outcomes without ns should error")
	}
	if err := cs.SaveOutcomes(OutcomeSnapshot{}); err == nil {
		t.Fatal("cm save outcomes without ns should error")
	}
}
