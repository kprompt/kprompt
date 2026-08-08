package autopilot

import (
	"context"
	"strings"
	"testing"

	"github.com/kprompt/kprompt/internal/agent/coordinator"
	"github.com/kprompt/kprompt/internal/agent/ctxbuild"
	"github.com/kprompt/kprompt/internal/agent/patterns"
	"github.com/kprompt/kprompt/internal/incident"
)

type fakeFleet struct {
	sum  coordinator.OutcomeSummary
	err  error
	hits int
}

func (f *fakeFleet) Outcomes(context.Context) (coordinator.OutcomeSummary, error) {
	f.hits++
	return f.sum, f.err
}

func crashCtx() ctxbuild.AgentContext {
	return ctxbuild.AgentContext{
		Namespace: "payments",
		Incident: incident.Incident{
			ID:              "inc-1",
			Summary:         "CrashLoopBackOff on api",
			Evidence:        []incident.EvidenceRef{{Type: incident.EvidenceEvent, Reason: "BackOff", Message: "Back-off"}},
			PrimaryResource: &incident.ResourceRef{Kind: "Deployment", Name: "api"},
		},
		Deployment: &ctxbuild.DeploymentSnapshot{Name: "api", DesiredReplicas: 2, ReadyReplicas: 1},
		Target:     &incident.ResourceRef{Kind: "Deployment", Name: "api"},
	}
}

func learnedLib(t *testing.T, ctx ctxbuild.AgentContext) *patterns.Library {
	t.Helper()
	lib := patterns.New(patterns.NewMemStore())
	for i := 0; i < 2; i++ {
		if _, err := lib.Record("payments", ctx, "high", "crash", "bad", "restart"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := lib.RecordOutcomeAction("payments", ctx, patterns.OutcomeApplySuccess, ActionRestartDeployment); err != nil {
		t.Fatal(err)
	}
	return lib
}

func TestFleetBiasAppliedWhenLocalMatched(t *testing.T) {
	ctx := crashCtx()
	fleet := &fakeFleet{sum: coordinator.OutcomeSummary{
		ByAction: []coordinator.OutcomeActionStat{
			{Action: ActionRestartDeployment, Success: 9, Total: 10},
		},
	}}
	eng := &Engine{Policy: DefaultPolicy(), Audit: &MemAudit{}, Patterns: learnedLib(t, ctx), Fleet: fleet}
	p, err := eng.ProposeFromContext(ctx, 0.9)
	if err != nil || p == nil {
		t.Fatalf("%v %+v", err, p)
	}
	if p.ActionID != ActionRestartDeployment {
		t.Fatalf("action=%s", p.ActionID)
	}
	if !strings.Contains(p.ExpectedImpact, "Fleet evidence (not proof)") {
		t.Fatalf("want fleet note, got impact=%q", p.ExpectedImpact)
	}
	if !strings.Contains(p.LearnNote, "AG-034/RT-022") {
		t.Fatalf("want AG-034 label, got learnNote=%q", p.LearnNote)
	}
	if fleet.hits == 0 {
		t.Fatal("expected fleet fetch")
	}
}

func TestFleetBiasSkippedWithoutLocalMatch(t *testing.T) {
	// No Patterns library → matched=false → fleet must not apply (AG-034).
	ctx := crashCtx()
	fleet := &fakeFleet{sum: coordinator.OutcomeSummary{
		ByAction: []coordinator.OutcomeActionStat{
			{Action: ActionRestartDeployment, Success: 9, Total: 10},
		},
	}}
	eng := &Engine{Policy: DefaultPolicy(), Audit: &MemAudit{}, Fleet: fleet}
	p, err := eng.ProposeFromContext(ctx, 0.9)
	if err != nil || p == nil {
		t.Fatalf("%v %+v", err, p)
	}
	if strings.Contains(p.ExpectedImpact, "Fleet evidence") {
		t.Fatalf("fleet must not bias without local match: %q", p.ExpectedImpact)
	}
	if fleet.hits != 0 {
		t.Fatalf("fleet must not be fetched without local match, hits=%d", fleet.hits)
	}
}

func TestFleetBiasSkippedBelowMinSamples(t *testing.T) {
	ctx := crashCtx()
	fleet := &fakeFleet{sum: coordinator.OutcomeSummary{
		ByAction: []coordinator.OutcomeActionStat{
			{Action: ActionRestartDeployment, Success: 1, Total: 1},
		},
	}}
	eng := &Engine{Policy: DefaultPolicy(), Audit: &MemAudit{}, Patterns: learnedLib(t, ctx), Fleet: fleet}
	p, err := eng.ProposeFromContext(ctx, 0.9)
	if err != nil || p == nil {
		t.Fatalf("%v %+v", err, p)
	}
	if strings.Contains(p.ExpectedImpact, "Fleet evidence") {
		t.Fatalf("fleet must not bias below min samples: %q", p.ExpectedImpact)
	}
}

func TestFleetBiasDoesNotGateApply(t *testing.T) {
	// Even a poor fleet record only lowers ActionConfidence, never the raw
	// confidence gate; proposal is still produced (not denied).
	ctx := crashCtx()
	fleet := &fakeFleet{sum: coordinator.OutcomeSummary{
		ByAction: []coordinator.OutcomeActionStat{
			{Action: ActionRestartDeployment, Failed: 9, Total: 10},
		},
	}}
	eng := &Engine{Policy: DefaultPolicy(), Audit: &MemAudit{}, Patterns: learnedLib(t, ctx), Fleet: fleet}
	p, err := eng.ProposeFromContext(ctx, 0.9)
	if err != nil || p == nil {
		t.Fatalf("%v %+v", err, p)
	}
	if p.Decision == DecisionDenied {
		t.Fatalf("fleet bias must not gate/deny: %+v", p)
	}
	if p.Confidence != 0.9 {
		t.Fatalf("raw confidence must be untouched, got %v", p.Confidence)
	}
}

func TestNormalizeBaseURL(t *testing.T) {
	cases := map[string]string{
		"http://c:9090/v1/handoff": "http://c:9090",
		"http://c:9090/":           "http://c:9090",
		"http://c:9090":            "http://c:9090",
		"http://c:9090/v1/outcomes": "http://c:9090",
	}
	for in, want := range cases {
		if got := coordinator.NormalizeBaseURL(in); got != want {
			t.Fatalf("NormalizeBaseURL(%q)=%q want %q", in, got, want)
		}
	}
}
