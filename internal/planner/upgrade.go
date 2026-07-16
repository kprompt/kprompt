package planner

import (
	"fmt"
	"strings"

	"github.com/kprompt/kprompt/internal/intent"
	toolshelm "github.com/kprompt/kprompt/internal/tools/helm"
)

func buildUpgrade(in intent.Intent, ns string) (ExecutionPlan, error) {
	if err := requireHelmBinary(); err != nil {
		return ExecutionPlan{}, err
	}
	name := strings.TrimSpace(in.Target.Name)
	if name == "" {
		return ExecutionPlan{}, fmt.Errorf("upgrade intent missing target.name")
	}
	version, ok := in.ChartVersion()
	if !ok || strings.TrimSpace(version) == "" {
		return ExecutionPlan{}, fmt.Errorf("upgrade intent missing params.version (chart version)")
	}
	version = strings.TrimSpace(version)

	release := name
	if r, ok := in.Release(); ok {
		release = r
	}

	chart, ok := resolveHelmChart(in, name)
	if !ok {
		return ExecutionPlan{}, fmt.Errorf("no Helm chart recipe for %q — set params.chart + params.repo_url", name)
	}
	if ns == "" {
		ns = "default"
	}
	kubeContext := strings.TrimSpace(in.Context)

	current, _ := toolshelm.CurrentChartLabel(release, ns, kubeContext)
	upgradeDiff := toolshelm.UpgradeDiffLine(current, chart.ChartRef, version)

	repoCmd := toolshelm.RepoAddCommand(chart.RepoName, chart.RepoURL)
	updateCmd := toolshelm.RepoUpdateCommand(chart.RepoName)
	upgradeCmd := toolshelm.UpgradeCommand(release, chart.ChartRef, ns, kubeContext, version)

	actions := []Action{
		helmRepoAction(chart.RepoName, repoCmd),
		{
			Op:      OpHelmRepoUpdate,
			Backend: "helm",
			Command: updateCmd,
			Diff:    strings.Join(updateCmd, " "),
			Object: ObjectRef{
				Kind: "HelmRepo",
				Name: chart.RepoName,
			},
		},
		{
			Op:      OpHelmUpgrade,
			Backend: "helm",
			Command: upgradeCmd,
			Diff:    upgradeDiff,
			Object: ObjectRef{
				Kind:      "HelmRelease",
				Name:      release,
				Namespace: ns,
			},
		},
	}
	summary := fmt.Sprintf("Helm upgrade %s (%s) to chart version %s in %s", release, chart.ChartRef, version, ns)

	return ExecutionPlan{
		Intent:           in,
		Actions:          actions,
		Summary:          summary,
		RequiresApproval: true,
	}, nil
}
