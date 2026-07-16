package planner

import (
	"fmt"
	"strings"

	"github.com/kprompt/kprompt/internal/intent"
	toolshelm "github.com/kprompt/kprompt/internal/tools/helm"
)

func buildInstall(in intent.Intent, ns string) (ExecutionPlan, error) {
	if err := requireHelmBinary(); err != nil {
		return ExecutionPlan{}, err
	}
	name := strings.TrimSpace(in.Target.Name)
	if name == "" {
		return ExecutionPlan{}, fmt.Errorf("install intent missing target.name")
	}
	release := name
	if r, ok := in.Release(); ok {
		release = r
	}

	chart, ok := resolveHelmChart(in, name)
	if !ok {
		return ExecutionPlan{}, fmt.Errorf("no Helm chart recipe for %q — set params.chart + params.repo_url or use: kprompt \"deploy %s\"", name, name)
	}

	replicas := int32(0)
	if r, ok := in.Replicas(); ok && r > 0 {
		replicas = r
	}
	if ns == "" {
		ns = "default"
	}

	kubeContext := strings.TrimSpace(in.Context)
	repoCmd := toolshelm.RepoAddCommand(chart.RepoName, chart.RepoURL)
	installCmd := toolshelm.InstallCommand(release, chart.ChartRef, ns, kubeContext, replicas)

	actions := []Action{
		helmRepoAction(chart.RepoName, repoCmd),
		{
			Op:      OpHelmInstall,
			Backend: "helm",
			Command: installCmd,
			Diff:    strings.Join(installCmd, " "),
			Object: ObjectRef{
				Kind:      "HelmRelease",
				Name:      release,
				Namespace: ns,
			},
		},
	}
	summary := fmt.Sprintf("Helm install %s (%s) in %s", release, chart.ChartRef, ns)

	return ExecutionPlan{
		Intent:           in,
		Actions:          actions,
		Summary:          summary,
		RequiresApproval: true,
	}, nil
}

func helmRepoAction(repoName string, repoCmd []string) Action {
	return Action{
		Op:      OpHelmRepo,
		Backend: "helm",
		Command: repoCmd,
		Diff:    strings.Join(repoCmd, " "),
		Object: ObjectRef{
			Kind: "HelmRepo",
			Name: repoName,
		},
	}
}
