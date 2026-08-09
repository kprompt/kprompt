package pipeline

import (
	"testing"

	"github.com/kprompt/kprompt/internal/planner"
)

func TestDeploymentWaitTargetsIncludesDaemonSets(t *testing.T) {
	plan := planner.ExecutionPlan{Actions: []planner.Action{
		{Op: planner.OpCreate, Object: planner.ObjectRef{Kind: "Deployment", Namespace: "default", Name: "api"}},
		{Op: planner.OpUpdate, Object: planner.ObjectRef{Kind: "StatefulSet", Namespace: "default", Name: "db"}},
		{Op: planner.OpScale, Object: planner.ObjectRef{Kind: "DaemonSet", Namespace: "default", Name: "node-agent"}},
		{Op: planner.OpCreate, Object: planner.ObjectRef{Kind: "Service", Namespace: "default", Name: "api"}},
		{Op: planner.OpCreate, Object: planner.ObjectRef{Kind: "DaemonSet", Namespace: "default", Name: "node-agent"}},
	}}

	got := deploymentWaitTargets(plan)
	if len(got) != 3 {
		t.Fatalf("got %d wait targets, want 3: %+v", len(got), got)
	}
	for _, want := range []planner.ObjectRef{
		{Kind: "Deployment", Namespace: "default", Name: "api"},
		{Kind: "StatefulSet", Namespace: "default", Name: "db"},
		{Kind: "DaemonSet", Namespace: "default", Name: "node-agent"},
	} {
		found := false
		for _, ref := range got {
			if ref == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing wait target %+v in %+v", want, got)
		}
	}
}

func TestDeploymentWaitTargetsDistinguishesKindsWithSameName(t *testing.T) {
	plan := planner.ExecutionPlan{Actions: []planner.Action{
		{Op: planner.OpCreate, Object: planner.ObjectRef{Kind: "Deployment", Namespace: "default", Name: "agent"}},
		{Op: planner.OpCreate, Object: planner.ObjectRef{Kind: "DaemonSet", Namespace: "default", Name: "agent"}},
	}}

	got := deploymentWaitTargets(plan)
	if len(got) != 2 {
		t.Fatalf("got %d wait targets, want 2: %+v", len(got), got)
	}
}
