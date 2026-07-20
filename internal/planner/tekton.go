package planner

import (
	"strings"

	"github.com/kprompt/kprompt/internal/intent"
	"github.com/kprompt/kprompt/internal/tools/tekton"
)

func buildTekton(in intent.Intent, ns string) (ExecutionPlan, error) {
	name := strings.TrimSpace(in.Target.Name)
	task, _ := in.Task()
	repo, _ := in.RepoURL()
	if repo == "" {
		repo, _ = in.Repo()
	}
	if name == "" {
		name = tekton.DefaultPipelineRunName(task, repo)
	}
	image, _ := in.Image()

	manifest, summary, err := tekton.GeneratePipelineRun(tekton.PipelineRequest{
		Name:      name,
		Namespace: ns,
		Repo:      repo,
		Image:     image,
		Task:      task,
	})
	if err != nil {
		return ExecutionPlan{}, err
	}

	return ExecutionPlan{
		Intent: in,
		Actions: []Action{{
			Op:       OpPipelineRunCreate,
			Backend:  "tekton",
			Manifest: manifest,
			Diff:     summary,
			Object: ObjectRef{
				APIVersion: tekton.PipelineGroup + "/v1",
				Kind:       tekton.PipelineRunKind,
				Name:       name,
				Namespace:  ns,
			},
		}},
		Summary:          summary,
		RequiresApproval: true,
	}, nil
}
