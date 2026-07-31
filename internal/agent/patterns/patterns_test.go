package patterns

import (
	"strings"
	"testing"
	"time"

	"github.com/kprompt/kprompt/internal/agent/ctxbuild"
	"github.com/kprompt/kprompt/internal/incident"
)

func sampleCtx(reason string) ctxbuild.AgentContext {
	return ctxbuild.AgentContext{
		Namespace: "payments",
		Incident: incident.Incident{
			ID:              "inc-1",
			Summary:         "CrashLoop on api",
			Severity:        incident.SeverityHigh,
			Evidence:        []incident.EvidenceRef{{Type: incident.EvidenceEvent, Reason: reason, Message: "Back-off restarting"}},
			PrimaryResource: &incident.ResourceRef{Kind: "Pod", Name: "api-1"},
		},
		Target: &incident.ResourceRef{Kind: "Pod", Name: "api-1"},
	}
}

func TestRecordMatchBoost(t *testing.T) {
	lib := New(NewMemStore())
	ctx := sampleCtx("BackOff")
	for i := 0; i < 2; i++ {
		if _, err := lib.Record("payments", ctx, "high", "CrashLoop", "migration timeout", "check migrate job"); err != nil {
			t.Fatal(err)
		}
	}
	match, ok := lib.Match("payments", ctx)
	if !ok || match.Count < 2 {
		t.Fatalf("match=%v ok=%v", match, ok)
	}
	boosted, note := ApplyBoost(SeverityConfidence{Confidence: 0.7, RootCause: "CrashLoop", Recommendation: "Inspect logs"}, match)
	if boosted.Confidence <= 0.7 {
		t.Fatalf("expected boost, got %v", boosted.Confidence)
	}
	if !strings.Contains(note, "Seen before") {
		t.Fatalf("note=%q", note)
	}
	if !strings.Contains(boosted.RootCause, "Seen before") {
		t.Fatalf("root=%q", boosted.RootCause)
	}
}

func TestNoMutateRecommendation(t *testing.T) {
	p := Pattern{Count: 5, LastRec: "kubectl delete pod --force", LastRootCause: "bad"}
	got, _ := ApplyBoost(SeverityConfidence{Confidence: 0.5, RootCause: "x", Recommendation: "Inspect logs"}, p)
	if strings.Contains(got.Recommendation, "kubectl delete") {
		t.Fatal("mutate rec leaked")
	}
}

func TestRecordOutcomeResolvedAndFP(t *testing.T) {
	lib := New(NewMemStore())
	ctx := sampleCtx("BackOff")
	if _, err := lib.Record("payments", ctx, "high", "CrashLoop", "migration timeout", "check migrate job"); err != nil {
		t.Fatal(err)
	}
	p, err := lib.RecordOutcome("payments", ctx, OutcomeResolved)
	if err != nil {
		t.Fatal(err)
	}
	if p.Confirmed != 1 {
		t.Fatalf("resolved: %+v", p)
	}
	p, err = lib.RecordOutcome("payments", ctx, OutcomeFalsePositive)
	if err != nil {
		t.Fatal(err)
	}
	if p.FalsePositives != 1 || p.Weight >= 1 {
		t.Fatalf("fp: %+v", p)
	}
	boost := EffectiveBoost(Pattern{Count: 5, Weight: p.Weight, FalsePositives: 2, Confirmed: 0})
	if boost >= MaxBoost {
		t.Fatalf("FP should dampen boost, got %v", boost)
	}
}

func TestEffectiveBoostZeroBelowMinPrior(t *testing.T) {
	if EffectiveBoost(Pattern{Count: 1, Weight: 1}) != 0 {
		t.Fatal("expected zero boost under MinPriorCount")
	}
}

func TestList(t *testing.T) {
	lib := New(NewMemStore())
	if _, err := lib.Record("payments", sampleCtx("CrashLoopBackOff"), "high", "crash", "oom", "check limits"); err != nil {
		t.Fatal(err)
	}
	snap, err := lib.List("payments")
	if err != nil || len(snap.Patterns) != 1 {
		t.Fatalf("snap=%+v err=%v", snap, err)
	}
	empty, err := lib.List("other")
	if err != nil || len(empty.Patterns) != 0 {
		t.Fatalf("empty ns should be empty: %+v err=%v", empty, err)
	}
}

func TestFileStore(t *testing.T) {
	dir := t.TempDir()
	lib := New(FileStore{Dir: dir})
	ctx := sampleCtx("OOMKilled")
	if _, err := lib.Record("ns", ctx, "critical", "OOM", "memory limit", "raise limit"); err != nil {
		t.Fatal(err)
	}
	lib2 := New(FileStore{Dir: dir})
	if _, err := lib2.Record("ns", ctx, "critical", "OOM", "memory limit", "raise limit"); err != nil {
		t.Fatal(err)
	}
	match, ok := lib2.Match("ns", ctx)
	if !ok || match.Count < 2 {
		t.Fatalf("%+v ok=%v", match, ok)
	}
	if match.LastSeenAt.IsZero() {
		t.Fatal("timestamp")
	}
	_ = time.Now()
}
