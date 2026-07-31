package handoff

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kprompt/kprompt/internal/incident"
)

func sampleReport(ns string) incident.InvestigationReport {
	r := incident.NewInvestigationReport(ns, time.Now().UTC())
	r.Summary = "CrashLoop on api"
	r.Confidence = 0.7
	r.Hypotheses = []incident.Hypothesis{{Statement: "OOM", Primary: true, Confidence: 0.7}}
	return r
}

func TestValidateAndNew(t *testing.T) {
	rep := sampleReport("payments")
	env := New("payments", "platform", "dependency may be outside my namespace", rep)
	if err := Validate(env); err != nil {
		t.Fatal(err)
	}
	if env.Kind != Kind || env.SchemaVersion != SchemaVersion {
		t.Fatalf("%+v", env)
	}
}

func TestValidateRejectsEmpty(t *testing.T) {
	if err := Validate(Envelope{Kind: Kind}); err == nil {
		t.Fatal("expected error")
	}
}

func TestNeedsHandoff(t *testing.T) {
	rep := sampleReport("payments")
	rep.Unknowns = []string{"dependency may be outside namespace"}
	_, reason, ok := NeedsHandoff("payments", rep)
	if !ok || !strings.Contains(reason, "Coordinator") {
		t.Fatalf("ok=%v reason=%q", ok, reason)
	}
	rep2 := sampleReport("payments")
	if _, _, ok := NeedsHandoff("payments", rep2); ok {
		t.Fatal("expected no handoff")
	}
}

func TestNeedsHandoffExtractsSuspectNS(t *testing.T) {
	rep := sampleReport("payments")
	rep.Summary = "timeout calling redis.platform.svc.cluster.local"
	suspect, reason, ok := NeedsHandoff("payments", rep)
	if !ok || suspect != "platform" {
		t.Fatalf("suspect=%q ok=%v reason=%q", suspect, ok, reason)
	}

	rep3 := sampleReport("payments")
	rep3.Unknowns = []string{"need verification in namespace platform"}
	suspect, _, ok = NeedsHandoff("payments", rep3)
	if !ok || suspect != "platform" {
		t.Fatalf("phrase suspect=%q ok=%v", suspect, ok)
	}

	rep4 := sampleReport("payments")
	rep4.Summary = "issue in namespace payments"
	if s, _, ok := NeedsHandoff("payments", rep4); ok && s == "payments" {
		t.Fatal("should not treat own namespace as suspect")
	}
}

func TestNeedsHandoffFromEvidence(t *testing.T) {
	rep := sampleReport("payments")
	rep.Summary = "CrashLoop on orders"
	rep.Evidence = []incident.EvidenceRef{{
		Type:    incident.EvidenceLog,
		Message: "dial tcp cache.platform.svc.cluster.local:6379: connection refused",
	}}
	suspect, _, ok := NeedsHandoff("payments", rep)
	if !ok || suspect != "platform" {
		t.Fatalf("suspect=%q ok=%v", suspect, ok)
	}
}

func TestFormatReply(t *testing.T) {
	text := FormatReply(&Reply{
		SuspectNamespace: "platform",
		Reason:           "cross-ns",
		MutateAttempted:  false,
		Merged: incident.InvestigationReport{
			Summary:    "platform cache not ready",
			Confidence: 0.5,
			Evidence:   []incident.EvidenceRef{{Type: incident.EvidenceEvent, Reason: "BackOff"}},
			Unknowns:   []string{"Coordinator: merged origin + suspect reports (verify before mutate)"},
		},
		Routing: []string{"probed namespace platform — merged suspect InvestigationReport"},
	})
	for _, want := range []string{"Coordinator reply", "platform", "50%", "Merged evidence", "probed namespace", "mutate=false"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in:\n%s", want, text)
		}
	}
	if FormatReply(nil) != "" {
		t.Fatal("nil reply should be empty")
	}
}

func TestHTTPClientParsesReply(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(Reply{
			Kind:             KindReply,
			FromNamespace:    "payments",
			SuspectNamespace: "platform",
			Merged:           sampleReport("payments"),
			Routing:          []string{"probed namespace platform"},
			MutateAttempted:  false,
		})
	}))
	defer srv.Close()

	c := &HTTPClient{URL: srv.URL}
	reply, err := c.Handoff(context.Background(), New("payments", "platform", "cross-ns", sampleReport("payments")))
	if err != nil {
		t.Fatal(err)
	}
	if reply == nil || reply.SuspectNamespace != "platform" || len(reply.Routing) == 0 {
		t.Fatalf("%+v", reply)
	}
	if reply.MutateAttempted {
		t.Fatal("mutate must stay false")
	}
}
