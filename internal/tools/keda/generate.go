package keda

import (
	"fmt"
	"strings"

	"sigs.k8s.io/yaml"
)

// ScaledObjectRequest is the structured input for ScaledObject manifest generation.
type ScaledObjectRequest struct {
	Name         string
	Namespace    string
	TargetName   string // Deployment to scale
	MinReplicas  int32
	MaxReplicas  int32
	Trigger      string // cpu | redis | cron
	Queue        string // redis list name
	Address      string // redis address
	CPUThreshold string // e.g. "50"
}

// GenerateScaledObject builds a KEDA ScaledObject YAML.
func GenerateScaledObject(req ScaledObjectRequest) (manifest string, summary string, err error) {
	req = normalizeRequest(req)
	if req.TargetName == "" {
		return "", "", fmt.Errorf("scaledobject scaleTargetRef.name is required")
	}
	if req.Name == "" {
		req.Name = DefaultScaledObjectName(req.TargetName, req.Trigger)
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	if req.MaxReplicas <= 0 {
		req.MaxReplicas = 10
	}
	if req.MinReplicas < 0 {
		req.MinReplicas = 0
	}
	if req.Trigger == "" {
		req.Trigger = "cpu"
	}

	trigger, err := buildTrigger(req)
	if err != nil {
		return "", "", err
	}

	doc := map[string]any{
		"apiVersion": ScaledObjectGroup + "/v1alpha1",
		"kind":       ScaledObjectKind,
		"metadata": map[string]any{
			"name":      req.Name,
			"namespace": req.Namespace,
			"labels": map[string]any{
				"app.kubernetes.io/managed-by": "kprompt",
			},
		},
		"spec": map[string]any{
			"scaleTargetRef": map[string]any{
				"name": req.TargetName,
			},
			"minReplicaCount": req.MinReplicas,
			"maxReplicaCount": req.MaxReplicas,
			"triggers":        []map[string]any{trigger},
		},
	}

	raw, err := yaml.Marshal(doc)
	if err != nil {
		return "", "", err
	}
	summary = fmt.Sprintf(
		"KEDA ScaledObject/%s: scale Deployment/%s min=%d max=%d trigger=%s",
		req.Name, req.TargetName, req.MinReplicas, req.MaxReplicas, req.Trigger,
	)
	return string(raw), summary, nil
}

// DefaultScaledObjectName builds a DNS-safe ScaledObject name.
func DefaultScaledObjectName(target, trigger string) string {
	target = sanitizeName(target)
	trigger = sanitizeName(trigger)
	if trigger == "" || trigger == "cpu" {
		return sanitizeName(target + "-keda")
	}
	return sanitizeName(target + "-" + trigger)
}

func buildTrigger(req ScaledObjectRequest) (map[string]any, error) {
	switch req.Trigger {
	case "redis":
		list := req.Queue
		if list == "" {
			list = "jobs"
		}
		addr := req.Address
		if addr == "" {
			addr = "redis:6379"
		}
		return map[string]any{
			"type": "redis",
			"metadata": map[string]any{
				"address":    addr,
				"listName":   list,
				"listLength": "5",
			},
		}, nil
	case "cron":
		// Keep replicas at min for the whole day — useful for scale-to-zero / idle windows.
		return map[string]any{
			"type": "cron",
			"metadata": map[string]any{
				"timezone":        "UTC",
				"start":           "0 0 * * *",
				"end":             "0 0 * * *",
				"desiredReplicas": fmt.Sprintf("%d", req.MinReplicas),
			},
		}, nil
	case "cpu", "http", "prometheus":
		threshold := req.CPUThreshold
		if threshold == "" {
			threshold = "50"
		}
		return map[string]any{
			"type":       "cpu",
			"metricType": "Utilization",
			"metadata": map[string]any{
				"value": threshold,
			},
		}, nil
	default:
		return nil, fmt.Errorf("unsupported keda trigger %q (cpu, redis, cron)", req.Trigger)
	}
}

func normalizeRequest(req ScaledObjectRequest) ScaledObjectRequest {
	req.Name = sanitizeName(req.Name)
	req.Namespace = strings.TrimSpace(req.Namespace)
	req.TargetName = sanitizeName(req.TargetName)
	req.Trigger = strings.ToLower(strings.TrimSpace(req.Trigger))
	req.Queue = strings.TrimSpace(req.Queue)
	req.Address = strings.TrimSpace(req.Address)
	req.CPUThreshold = strings.TrimSpace(req.CPUThreshold)
	return req
}

func sanitizeName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			return r
		}
		return '-'
	}, name)
	name = strings.Trim(name, "-")
	if name == "" {
		return "kprompt-scaledobject"
	}
	if len(name) > 63 {
		name = name[:63]
	}
	return name
}
