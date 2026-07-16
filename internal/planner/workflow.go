package planner

import (
	"fmt"
	"strings"

	"github.com/kprompt/kprompt/internal/intent"
	"github.com/kprompt/kprompt/internal/tools/argo"
)

func buildWorkflow(in intent.Intent, ns string) (ExecutionPlan, error) {
	name := strings.TrimSpace(in.Target.Name)
	task, _ := in.Task()
	model, _ := in.Model()
	if name == "" {
		if model == "" {
			return ExecutionPlan{}, fmt.Errorf("workflow intent missing target.name or params.model")
		}
		name = argo.DefaultWorkflowName(task, model)
	}

	image, _ := in.Image()
	dataset, _ := in.Dataset()
	gpu := in.WantGPU()

	req := argo.WorkflowRequest{
		Name:      name,
		Namespace: ns,
		Task:      task,
		Model:     model,
		Image:     image,
		Dataset:   dataset,
		GPU:       gpu,
	}
	if cmd, ok := in.Command(); ok {
		req.Command = cmd
	}
	if args, ok := in.Args(); ok {
		req.Args = args
	}

	manifest, summary, err := argo.GenerateWorkflow(req)
	if err != nil {
		return ExecutionPlan{}, err
	}

	return ExecutionPlan{
		Intent: in,
		Actions: []Action{{
			Op:       OpWorkflowCreate,
			Backend:  "argo",
			Manifest: manifest,
			Diff:     summary,
			Object: ObjectRef{
				APIVersion: argo.WorkflowGroup + "/v1alpha1",
				Kind:       argo.WorkflowKind,
				Name:       name,
				Namespace:  ns,
			},
		}},
		Summary:          summary,
		RequiresApproval: true,
	}, nil
}
