package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/kprompt/kprompt/internal/intent"
	"github.com/kprompt/kprompt/internal/planner"
	"github.com/kprompt/kprompt/internal/safety"
)

func TestFromPlanSchema(t *testing.T) {
	rep := int32(3)
	r := FromPlan("scale api to 3", "ctx", planner.ExecutionPlan{
		Intent: intent.Intent{Kind: intent.KindScale, Target: intent.Target{Namespace: "demo"}},
		Actions: []planner.Action{{
			Op:       planner.OpScale,
			Object:   planner.ObjectRef{Kind: "Deployment", Name: "api", Namespace: "demo"},
			Replicas: &rep,
			Diff:     "replicas: 1 → 3",
			Manifest: "SECRET",
		}},
		Summary:          "Scale api",
		RequiresApproval: true,
	}, safety.Result{Risk: safety.RiskMedium, Message: "Mutation requires approval"}, false)

	if r.SchemaVersion != SchemaVersion || r.APIVersion != APIVersion {
		t.Fatalf("%+v", r)
	}
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "SECRET") {
		t.Fatal("manifest leaked")
	}
	var buf bytes.Buffer
	if err := Encode(&buf, r); err != nil {
		t.Fatal(err)
	}
	if !json.Valid(buf.Bytes()) {
		t.Fatalf("invalid json: %s", buf.String())
	}
}
