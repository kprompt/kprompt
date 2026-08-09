package coordinator

import (
	"context"
	"testing"
	"time"

	"github.com/kprompt/kprompt/internal/agent/handoff"
	"github.com/kprompt/kprompt/internal/incident"
)

func validEnv(from, suspect string) handoff.Envelope {
	return handoff.Envelope{
		APIVersion: handoff.APIVersion, Kind: handoff.Kind, SchemaVersion: handoff.SchemaVersion,
		FromNamespace: from, SuspectNamespace: suspect, Reason: "proactive",
		Report: incident.InvestigationReport{
			APIVersion: incident.APIVersion, Kind: incident.KindInvestigationReport,
			SchemaVersion: incident.SchemaVersion2, Namespace: from, Summary: "seed",
		},
	}
}

func TestTickBudgetSortAndSkip(t *testing.T) {
	s := New()
	// a->b appears twice (count 2), c->d once (count 1) => sort puts a->b first.
	for _, e := range []handoff.Envelope{validEnv("a", "b"), validEnv("a", "b"), validEnv("c", "d")} {
		if _, err := s.Handle(context.Background(), e); err != nil {
			t.Fatalf("seed handle: %v", err)
		}
	}

	// Budget 0 -> DefaultTickBudget; both edges probed.
	res := s.Tick(context.Background(), TickConfig{Budget: 0})
	if res.EdgesConsidered != 2 {
		t.Fatalf("edgesConsidered=%d want 2", res.EdgesConsidered)
	}
	if res.Probed != 2 || res.Skipped != 0 {
		t.Fatalf("budget0: probed=%d skipped=%d", res.Probed, res.Skipped)
	}

	// Budget 1 -> the lower-priority edge is skipped.
	res = s.Tick(context.Background(), TickConfig{Budget: 1})
	if res.Probed != 1 || res.Skipped != 1 {
		t.Fatalf("budget1: probed=%d skipped=%d", res.Probed, res.Skipped)
	}
}

func TestLatestOriginReport(t *testing.T) {
	s := New()
	if _, err := s.Handle(context.Background(), validEnv("a", "b")); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// Found path returns the stored report.
	rep := s.latestOriginReport("a", "b")
	if rep.Namespace != "a" || rep.Summary == "" {
		t.Fatalf("found report: %+v", rep)
	}
	// Fallback path synthesizes a refresh report for an unknown edge.
	fb := s.latestOriginReport("x", "y")
	if fb.Namespace != "x" || fb.SchemaVersion != incident.SchemaVersion2 {
		t.Fatalf("fallback report: %+v", fb)
	}
}

func TestRunTickerNilLogf(t *testing.T) {
	s := New()
	if _, err := s.Handle(context.Background(), validEnv("a", "b")); err != nil {
		t.Fatalf("seed: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		RunTicker(ctx, s, TickConfig{Interval: time.Millisecond, Budget: 1}, nil)
		close(done)
	}()
	time.Sleep(15 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("RunTicker did not stop after cancel")
	}
}
