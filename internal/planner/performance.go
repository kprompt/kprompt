package planner

import (
	"fmt"
	"strings"
	"time"

	"github.com/kprompt/kprompt/internal/intent"
)

const (
	defaultPerformanceWindow = 15 * time.Minute
	maxPerformanceWindow     = 24 * time.Hour
)

func buildPerformance(in intent.Intent, ns string) (ExecutionPlan, error) {
	name := strings.TrimSpace(in.Target.Name)
	if name == "" {
		return ExecutionPlan{}, fmt.Errorf("performance intent missing target.name")
	}
	window := defaultPerformanceWindow
	if raw, ok := in.Window(); ok {
		parsed, err := time.ParseDuration(raw)
		if err != nil {
			return ExecutionPlan{}, fmt.Errorf("params.window: %w", err)
		}
		if parsed < time.Minute {
			return ExecutionPlan{}, fmt.Errorf("params.window must be at least 1m")
		}
		if parsed > maxPerformanceWindow {
			return ExecutionPlan{}, fmt.Errorf("params.window must not exceed 24h")
		}
		window = parsed
	}
	if in.Params == nil {
		in.Params = map[string]any{}
	}
	windowLabel := formatPerformanceWindow(window)
	in.Params["window"] = windowLabel

	summary := fmt.Sprintf(
		"Analyze Deployment/%s performance in %s over %s using Prometheus",
		name,
		ns,
		windowLabel,
	)
	return ExecutionPlan{
		Intent: in,
		Actions: []Action{{
			Op:      OpPromQuery,
			Backend: "prometheus",
			Object: ObjectRef{
				Kind:      "Deployment",
				Name:      name,
				Namespace: ns,
			},
			Diff: "query CPU, memory, p95 latency, replica, and HPA metrics",
		}},
		Summary:          summary,
		RequiresApproval: false,
	}, nil
}

func formatPerformanceWindow(window time.Duration) string {
	if window%time.Hour == 0 {
		return fmt.Sprintf("%dh", int64(window/time.Hour))
	}
	if window%time.Minute == 0 {
		return fmt.Sprintf("%dm", int64(window/time.Minute))
	}
	return fmt.Sprintf("%ds", int64(window/time.Second))
}
