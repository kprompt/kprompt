package planner

import (
	"fmt"
	"strings"

	"github.com/kprompt/kprompt/internal/intent"
	"github.com/kprompt/kprompt/internal/tools/keda"
)

func buildKEDA(in intent.Intent, ns string) (ExecutionPlan, error) {
	target := strings.TrimSpace(in.Target.Name)
	if t, ok := in.StringParam("target"); ok {
		target = t
	}
	if target == "" {
		return ExecutionPlan{}, fmt.Errorf("keda intent requires a workload name (Deployment to scale)")
	}
	trigger, _ := in.StringParam("trigger")
	if trigger == "" {
		trigger = "cpu"
	}
	name := keda.DefaultScaledObjectName(target, trigger)
	if so, ok := in.StringParam("scaledObject"); ok {
		name = so
	}

	minRep := int32(0)
	if v, ok := in.Params["minReplicas"]; ok {
		switch n := v.(type) {
		case float64:
			minRep = int32(n)
		case int:
			minRep = int32(n)
		case int32:
			minRep = n
		}
	} else if rep, ok := in.Replicas(); ok {
		minRep = rep
	}
	maxRep := int32(10)
	if v, ok := in.Params["maxReplicas"]; ok {
		switch n := v.(type) {
		case float64:
			maxRep = int32(n)
		case int:
			maxRep = int32(n)
		case int32:
			maxRep = n
		}
	}
	queue, _ := in.StringParam("queue")
	if queue == "" {
		queue, _ = in.StringParam("listName")
	}
	addr, _ := in.StringParam("address")
	cpu, _ := in.StringParam("cpuThreshold")

	manifest, summary, err := keda.GenerateScaledObject(keda.ScaledObjectRequest{
		Name:         name,
		Namespace:    ns,
		TargetName:   target,
		MinReplicas:  minRep,
		MaxReplicas:  maxRep,
		Trigger:      trigger,
		Queue:        queue,
		Address:      addr,
		CPUThreshold: cpu,
	})
	if err != nil {
		return ExecutionPlan{}, err
	}

	return ExecutionPlan{
		Intent: in,
		Actions: []Action{{
			Op:       OpScaledObjectCreate,
			Backend:  "keda",
			Manifest: manifest,
			Diff:     summary,
			Object: ObjectRef{
				APIVersion: keda.ScaledObjectGroup + "/v1alpha1",
				Kind:       keda.ScaledObjectKind,
				Name:       name,
				Namespace:  ns,
			},
		}},
		Summary:          summary,
		RequiresApproval: true,
	}, nil
}
