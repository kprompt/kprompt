package planner

import (
	"fmt"
	"strings"

	"github.com/kprompt/kprompt/internal/intent"
)

func buildGraph(in intent.Intent) (ExecutionPlan, error) {
	scopeNS := strings.TrimSpace(in.Target.Namespace)
	if scope, ok := in.StringParam("scope"); ok && scope == "cluster" {
		scopeNS = ""
		in.Target.Namespace = ""
	}
	if in.Params == nil {
		in.Params = map[string]any{}
	}
	includeNP := true
	if v, ok := in.Params["includeNetworkPolicy"]; ok {
		switch t := v.(type) {
		case bool:
			includeNP = t
		case string:
			includeNP = strings.EqualFold(t, "true") || t == "1"
		}
	}
	in.Params["includeNetworkPolicy"] = includeNP

	summary := "Service dependency graph (Kubernetes Services + EndpointSlices)"
	if scopeNS != "" {
		summary = fmt.Sprintf("Service dependency graph for namespace %s", scopeNS)
	}
	if includeNP {
		summary += " including NetworkPolicy hints"
	}

	if strings.TrimSpace(in.Target.Kind) == "" {
		in.Target.Kind = "ServiceGraph"
	}

	return ExecutionPlan{
		Intent: in,
		Actions: []Action{{
			Op:      OpGraph,
			Backend: "kubernetes",
			Object: ObjectRef{
				Kind:      in.Target.Kind,
				Namespace: scopeNS,
			},
			Diff: "build adjacency from Services, EndpointSlices, and optional NetworkPolicies (read-only)",
		}},
		Summary:          summary,
		RequiresApproval: false,
	}, nil
}
