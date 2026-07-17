package planner

import (
	"fmt"
	"strings"
	"time"

	"github.com/kprompt/kprompt/internal/intent"
)

const defaultTraceWindow = time.Hour

func buildTrace(in intent.Intent) (ExecutionPlan, error) {
	service := strings.TrimSpace(in.Target.Name)
	if service == "" {
		return ExecutionPlan{}, fmt.Errorf("trace intent missing target.name (service name)")
	}
	window := defaultTraceWindow
	if raw, ok := in.Window(); ok {
		parsed, err := time.ParseDuration(raw)
		if err != nil {
			return ExecutionPlan{}, fmt.Errorf("params.window: %w", err)
		}
		if parsed < time.Minute {
			return ExecutionPlan{}, fmt.Errorf("params.window must be at least 1m")
		}
		if parsed > 24*time.Hour {
			return ExecutionPlan{}, fmt.Errorf("params.window must not exceed 24h")
		}
		window = parsed
	}
	if in.Params == nil {
		in.Params = map[string]any{}
	}
	windowLabel := formatPerformanceWindow(window)
	in.Params["window"] = windowLabel

	diff := fmt.Sprintf("search recent traces for service=%s over %s", service, windowLabel)
	if operation, ok := in.Operation(); ok {
		diff += fmt.Sprintf(" operation=%q", operation)
	}
	return ExecutionPlan{
		Intent: in,
		Actions: []Action{{
			Op:      OpTraceQuery,
			Backend: "opentelemetry",
			Object: ObjectRef{
				Kind: "Service",
				Name: service,
			},
			Diff: diff,
		}},
		Summary:          fmt.Sprintf("Walk the latest %s trace using OpenTelemetry", service),
		RequiresApproval: false,
	}, nil
}
