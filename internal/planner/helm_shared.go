package planner

import (
	"fmt"
	"os/exec"

	"github.com/kprompt/kprompt/internal/intent"
	"github.com/kprompt/kprompt/internal/tools"
	toolshelm "github.com/kprompt/kprompt/internal/tools/helm"
)

func requireHelmBinary() error {
	if _, err := exec.LookPath("helm"); err != nil {
		return fmt.Errorf("%s", tools.MissingHint(tools.IDHelm))
	}
	return nil
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
