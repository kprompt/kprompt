package planner

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/kprompt/kprompt/internal/intent"
	"github.com/kprompt/kprompt/internal/tools"
	toolshelm "github.com/kprompt/kprompt/internal/tools/helm"
)

func buildInstall(in intent.Intent, ns string) (ExecutionPlan, error) {
	if _, err := exec.LookPath("helm"); err != nil {
		return ExecutionPlan{}, fmt.Errorf("%s", tools.MissingHint(tools.IDHelm))
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

	repoCmd := toolshelm.RepoAddCommand(chart.RepoName, chart.RepoURL)
	installCmd := toolshelm.InstallCommand(release, chart.ChartRef, ns, strings.TrimSpace(in.Context), replicas)

	actions := []Action{
		{
			Op:      OpHelmRepo,
			Backend: "helm",
			Command: repoCmd,
			Diff:    strings.Join(repoCmd, " "),
			Object: ObjectRef{
				Kind: "HelmRepo",
				Name: chart.RepoName,
			},
		},
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

func resolveHelmChart(in intent.Intent, name string) (toolshelm.Chart, bool) {
	if chartRef, ok := in.Chart(); ok {
		repo, _ := in.Repo()
		if url, hasURL := in.RepoURL(); hasURL {
			if c, ok := toolshelm.FromParams(chartRef, repo, url); ok {
				return c, true
			}
		}
	}
	return toolshelm.Lookup(name)
}
