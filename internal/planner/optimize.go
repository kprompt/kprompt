package planner

import (
	"fmt"
	"strings"
	"time"

	"github.com/kprompt/kprompt/internal/intent"
)

const (
	defaultOptimizeWindow = time.Hour
	maxOptimizeWindow     = 24 * time.Hour
)

func buildOptimize(in intent.Intent) (ExecutionPlan, error) {
	window := defaultOptimizeWindow
	if raw, ok := in.Window(); ok {
		parsed, err := time.ParseDuration(raw)
		if err != nil {
			return ExecutionPlan{}, fmt.Errorf("params.window: %w", err)
		}
		if parsed < time.Minute {
			return ExecutionPlan{}, fmt.Errorf("params.window must be at least 1m")
		}
		if parsed > maxOptimizeWindow {
			return ExecutionPlan{}, fmt.Errorf("params.window must not exceed 24h")
		}
		window = parsed
	}
	if in.Params == nil {
		in.Params = map[string]any{}
	}
	windowLabel := formatPerformanceWindow(window)
	in.Params["window"] = windowLabel

	scopeNS := strings.TrimSpace(in.Target.Namespace)
	if scope, ok := in.StringParam("scope"); ok && scope == "cluster" {
		scopeNS = ""
		in.Target.Namespace = ""
	}

	summary := "Optimize cluster (read-only report scaffold)"
	if scopeNS != "" {
		summary = fmt.Sprintf("Optimize namespace %s (read-only report scaffold)", scopeNS)
	}
	summary += " over " + windowLabel

	if strings.TrimSpace(in.Target.Kind) == "" {
		in.Target.Kind = "Cluster"
	}

	return ExecutionPlan{
		Intent: in,
		Actions: []Action{{
			Op:      OpOptimize,
			Backend: "optimize",
			Object: ObjectRef{
				Kind:      in.Target.Kind,
				Namespace: scopeNS,
			},
			Diff: "collect inventory / idle / rightsizing / HPA signals (scaffold in T-052)",
		}},
		Summary:          summary,
		RequiresApproval: false,
	}, nil
}
