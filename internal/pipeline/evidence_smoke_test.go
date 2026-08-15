package pipeline

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/kprompt/kprompt/internal/config"
	"github.com/kprompt/kprompt/internal/incident"
	"github.com/kprompt/kprompt/internal/llm"
	"github.com/kprompt/kprompt/internal/output"
	"github.com/kprompt/kprompt/internal/pretrust"
)

// Evidence / pretrust smoke (Investigate → Verify): JSON Investigation must carry
// EvidenceRef anchors, and soft-agree high confidence without evidence is clamped.

func TestPlanResultSmokeWhyJSONHasEvidence(t *testing.T) {
	ns := "payments"
	client := fake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "ledger", Namespace: ns},
			Status: corev1.PodStatus{
				Phase: corev1.PodPending,
				Conditions: []corev1.PodCondition{{
					Type:    corev1.PodScheduled,
					Status:  corev1.ConditionFalse,
					Reason:  "Unschedulable",
					Message: "persistentvolumeclaim \"ledger-data\" not found",
				}},
			},
		},
	)
	var out bytes.Buffer
	var got output.PlanResult
	err := RunWith(context.Background(), config.Resolved{
		Namespace: ns,
		Output:    "json",
		Prompt:    "why is ledger Pending",
	}, &out, Deps{
		Provider:      llm.WhyStub("ledger", ns, "Pod"),
		Client:        client,
		SkipOrgPolicy: true,
		OnResult:      func(doc output.PlanResult) { got = doc },
	})
	if err != nil {
		t.Fatal(err)
	}
	assertPlanResultJSON(t, out.Bytes(), &got)
	inv := decodeInvestigation(t, got.Result)
	if len(inv.Evidence) == 0 {
		t.Fatalf("why Investigation must include EvidenceRef: %+v", inv)
	}
	if inv.Confidence <= 0 {
		t.Fatalf("confidence=%v", inv.Confidence)
	}
	if !hasUsableEvidenceJSON(inv) {
		t.Fatalf("evidence not usable: %+v", inv.Evidence)
	}
}

func TestPlanResultSmokeSoftAgreeClampedBeforeJSON(t *testing.T) {
	// Direct pretrust contract used by stampInvestigationPretrust — empty evidence +
	// high confidence must never ship as-is in PlanResult.
	inv := incident.NewInvestigation("why is api crashing", "payments")
	inv.Summary = "CrashLoop on api"
	inv.RootCause = "CrashLoopBackOff"
	inv.Confidence = 0.92
	inv.Target = &incident.ResourceRef{Kind: "Deployment", Name: "api", Namespace: "payments"}
	inv.Findings = []incident.Finding{{
		Code: "CrashLoopBackOff", Severity: incident.SeverityHigh,
		Title: "Crash looping", Message: "container in CrashLoopBackOff",
	}}

	stampInvestigationPretrust(context.Background(), nil, &inv)

	if inv.Confidence > pretrust.SoftAgreeConfidenceCap {
		t.Fatalf("soft-agree confidence not clamped: %v", inv.Confidence)
	}
	blob := strings.Join(inv.Degraded, " ")
	if !strings.Contains(blob, "EvidenceRef") && !strings.Contains(blob, "soft-agree") && !strings.Contains(blob, "pretrust") {
		t.Fatalf("expected degraded pretrust note, got %v", inv.Degraded)
	}
}

func decodeInvestigation(t *testing.T, raw json.RawMessage) incident.Investigation {
	t.Helper()
	if len(raw) == 0 {
		t.Fatal("empty result")
	}
	var inv incident.Investigation
	if err := json.Unmarshal(raw, &inv); err != nil {
		t.Fatalf("unmarshal Investigation: %v\n%s", err, raw)
	}
	if inv.Kind != "" && inv.Kind != incident.KindInvestigation {
		t.Fatalf("kind=%q", inv.Kind)
	}
	return inv
}

func hasUsableEvidenceJSON(inv incident.Investigation) bool {
	for _, e := range inv.Evidence {
		if strings.TrimSpace(e.Type) == "" {
			continue
		}
		if e.Resource != nil && e.Resource.Name != "" {
			return true
		}
		if strings.TrimSpace(e.Reason) != "" || strings.TrimSpace(e.Message) != "" || strings.TrimSpace(e.Source) != "" {
			return true
		}
	}
	return false
}
