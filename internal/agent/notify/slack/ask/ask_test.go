package ask

import (
	"context"
	"strings"
	"testing"

	"github.com/kprompt/kprompt/internal/agent/health"
	"github.com/kprompt/kprompt/internal/incident"
)

func TestParseIntent(t *testing.T) {
	cases := map[string]Intent{
		"status":           IntentStatus,
		"<@U123> status":   IntentStatus,
		"what broke?":      IntentWhatBroke,
		"why is api down":  IntentWhy,
		"help":             IntentHelp,
		"deploy redis":     IntentUnknown,
		"approve ap-1":     IntentApprove,
		"approve":          IntentApprove,
		"false positive":   IntentFalsePos,
	}
	for in, want := range cases {
		if got := ParseIntent(in); got != want {
			t.Fatalf("%q: got %s want %s", in, got, want)
		}
	}
}

func TestAnswerStatusAndWhy(t *testing.T) {
	incs := []incident.Incident{{
		ID:         "inc-1",
		Namespace:  "payments",
		Severity:   incident.SeverityHigh,
		Summary:    "CrashLoop on api",
		RootCause:  "Bad config",
		Confidence: 0.9,
		Status:     incident.StatusOpen,
	}}
	h := &Handler{
		OpenIncidents: func() []incident.Incident { return incs },
		Health: func(ctx context.Context) *health.Snapshot {
			return &health.Snapshot{Score: 72, Trend: "down", OpenIncidents: 1, Message: "degraded"}
		},
	}
	status := h.Answer(context.Background(), "status")
	if !strings.Contains(status, "72/100") || !strings.Contains(status, "CrashLoop") {
		t.Fatalf("status: %s", status)
	}
	why := h.Answer(context.Background(), "why")
	if !strings.Contains(why, "Bad config") {
		t.Fatalf("why: %s", why)
	}
	broke := h.Answer(context.Background(), "what broke")
	if !strings.Contains(broke, "CrashLoop") {
		t.Fatalf("broke: %s", broke)
	}
}

func TestParseApproveTarget(t *testing.T) {
	cases := map[string]string{
		"approve ap-restart-1":  "ap-restart-1",
		"<@U1> approve ap-x":    "ap-x",
		"approve proposal ap-y": "ap-y",
		"apply ap-z":            "ap-z",
		"approve":               "",
		"status":                "",
		"<@U1>":                 "", // mention-only → empty after strip
		"   ":                   "",
	}
	for in, want := range cases {
		if got := ParseApproveTarget(in); got != want {
			t.Fatalf("%q: got %q want %q", in, got, want)
		}
	}
}
