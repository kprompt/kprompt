package ask

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/kprompt/kprompt/internal/incident"
)

func TestAnswerNilHandler(t *testing.T) {
	var h *Handler
	if got := h.Answer(context.Background(), "status"); !strings.Contains(got, "not configured") {
		t.Fatalf("nil handler: %s", got)
	}
}

func TestAnswerHelpAndUnknown(t *testing.T) {
	h := &Handler{}
	if got := h.Answer(context.Background(), "help"); !strings.Contains(got, "Ask me") {
		t.Fatalf("help: %s", got)
	}
	if got := h.Answer(context.Background(), "deploy something"); !strings.Contains(got, "read-only except approve") {
		t.Fatalf("unknown: %s", got)
	}
}

func TestAnswerStatusNoIncidents(t *testing.T) {
	h := &Handler{OpenIncidents: func() []incident.Incident { return nil }}
	if got := h.Answer(context.Background(), "status"); !strings.Contains(got, "No open incidents") {
		t.Fatalf("status empty: %s", got)
	}
	if got := h.Answer(context.Background(), "what broke"); !strings.Contains(got, "Nothing open") {
		t.Fatalf("whatbroke empty: %s", got)
	}
	if got := h.Answer(context.Background(), "why"); !strings.Contains(got, "No open incident") {
		t.Fatalf("why empty: %s", got)
	}
}

func TestAnswerWhatBrokeWithResource(t *testing.T) {
	h := &Handler{OpenIncidents: func() []incident.Incident {
		return []incident.Incident{{
			ID:              "inc-9",
			Summary:         "pods crashing",
			PrimaryResource: &incident.ResourceRef{Kind: "Deployment", Name: "api"},
		}}
	}}
	got := h.Answer(context.Background(), "what broke")
	if !strings.Contains(got, "Deployment/api") {
		t.Fatalf("expected target in output: %s", got)
	}
}

func TestAnswerFalsePositive(t *testing.T) {
	incs := []incident.Incident{
		{ID: "low", Severity: incident.SeverityLow},
		{ID: "crit", Severity: incident.SeverityCritical},
	}
	// No incidents.
	empty := &Handler{OpenIncidents: func() []incident.Incident { return nil }}
	if got := empty.Answer(context.Background(), "false positive"); !strings.Contains(got, "No open incident") {
		t.Fatalf("fp empty: %s", got)
	}
	// No callback configured.
	noCb := &Handler{OpenIncidents: func() []incident.Incident { return incs }}
	if got := noCb.Answer(context.Background(), "false positive"); !strings.Contains(got, "not enabled") {
		t.Fatalf("fp no cb: %s", got)
	}
	// Callback success — picks highest severity.
	var marked string
	ok := &Handler{
		OpenIncidents: func() []incident.Incident { return incs },
		MarkFalsePositive: func(ctx context.Context, inc incident.Incident) error {
			marked = inc.ID
			return nil
		},
	}
	if got := ok.Answer(context.Background(), "fp"); !strings.Contains(got, "Recorded false positive") {
		t.Fatalf("fp ok: %s", got)
	}
	if marked != "crit" {
		t.Fatalf("expected highest severity marked, got %q", marked)
	}
	// Callback error.
	bad := &Handler{
		OpenIncidents:     func() []incident.Incident { return incs },
		MarkFalsePositive: func(ctx context.Context, inc incident.Incident) error { return fmt.Errorf("boom") },
	}
	if got := bad.Answer(context.Background(), "false positive"); !strings.Contains(got, "Could not record") {
		t.Fatalf("fp err: %s", got)
	}
}

func TestAnswerApprove(t *testing.T) {
	// No callback.
	noCb := &Handler{}
	if got := noCb.Answer(context.Background(), "approve ap-1"); !strings.Contains(got, "not enabled") {
		t.Fatalf("approve no cb: %s", got)
	}
	// With callback receives parsed id.
	var gotID string
	h := &Handler{ApproveProposal: func(ctx context.Context, id string) string {
		gotID = id
		return "approved " + id
	}}
	out := h.Answer(context.Background(), "approve ap-42")
	if gotID != "ap-42" || !strings.Contains(out, "approved ap-42") {
		t.Fatalf("approve cb: id=%q out=%q", gotID, out)
	}
}

func TestSeverityRank(t *testing.T) {
	if severityRank(incident.SeverityCritical) <= severityRank(incident.SeverityLow) {
		t.Fatal("critical must outrank low")
	}
	if severityRank("bogus") != 0 {
		t.Fatal("unknown severity rank must be 0")
	}
}
