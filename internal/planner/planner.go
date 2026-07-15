package planner

import (
	"fmt"

	"github.com/kprompt/kprompt/internal/intent"
)

// Op is a planned Kubernetes operation.
type Op string

const (
	OpCreate Op = "create"
	OpUpdate Op = "update"
	OpScale  Op = "scale"
	OpDelete Op = "delete"
	OpGet    Op = "get"
)

// ObjectRef is a Kubernetes object identity.
type ObjectRef struct {
	APIVersion string
	Kind       string
	Name       string
	Namespace  string
}

// Action is one step in an execution plan.
type Action struct {
	Op       Op
	Object   ObjectRef
	Manifest string
	Diff     string
	Replicas *int32
}

// ExecutionPlan is the reviewable output of planning.
type ExecutionPlan struct {
	Intent           intent.Intent
	Actions          []Action
	Summary          string
	RequiresApproval bool
}

// Build creates an ExecutionPlan from a structured Intent.
// v0 supports scale as the first mutation path.
func Build(in intent.Intent) (ExecutionPlan, error) {
	ns := in.Target.Namespace
	if ns == "" {
		ns = "default"
	}
	switch in.Kind {
	case intent.KindScale:
		name := in.Target.Name
		if name == "" {
			return ExecutionPlan{}, fmt.Errorf("scale intent missing target.name")
		}
		replicas, ok := in.Replicas()
		if !ok || replicas < 0 {
			return ExecutionPlan{}, fmt.Errorf("scale intent missing valid params.replicas")
		}
		kind := in.Target.Kind
		if kind == "" {
			kind = "Deployment"
		}
		rep := replicas
		plan := ExecutionPlan{
			Intent: in,
			Actions: []Action{{
				Op: OpScale,
				Object: ObjectRef{
					APIVersion: "apps/v1",
					Kind:       kind,
					Name:       name,
					Namespace:  ns,
				},
				Replicas: &rep,
				Diff:     fmt.Sprintf("scale %s/%s to %d replicas", kind, name, replicas),
			}},
			Summary:          fmt.Sprintf("Scale %s/%s in %s to %d replicas", kind, name, ns, replicas),
			RequiresApproval: true,
		}
		return plan, nil
	case intent.KindGet, intent.KindExplain:
		return ExecutionPlan{
			Intent:  in,
			Summary: fmt.Sprintf("%s %s (read-only; not implemented in v0 beyond planning)", in.Kind, in.Target.Name),
			Actions: []Action{{
				Op: OpGet,
				Object: ObjectRef{
					Kind:      first(in.Target.Kind, "Pod"),
					Name:      in.Target.Name,
					Namespace: ns,
				},
			}},
		}, nil
	case intent.KindDeploy:
		return ExecutionPlan{}, fmt.Errorf("deploy planning is not implemented in v0 (scale is the first mutation path)")
	case intent.KindDeny:
		return ExecutionPlan{Intent: in, Summary: "Denied intent", RequiresApproval: false}, nil
	default:
		return ExecutionPlan{}, fmt.Errorf("unsupported intent kind %q", in.Kind)
	}
}

func first(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
