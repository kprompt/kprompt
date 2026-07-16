package tools

import (
	"os/exec"
)

func detectHelm(settings Settings) Result {
	r := Result{
		ID:           IDHelm,
		Name:         "Helm",
		Capabilities: []Capability{CapInstall, CapUpgrade, CapMutate},
	}
	if !settings.HelmEnabled {
		r.Status = StatusDisabled
		r.Detail = "disabled in config or KPROMPT_HELM_ENABLED=0"
		r.Hint = MissingHint(IDHelm)
		return r
	}
	path, err := exec.LookPath("helm")
	if err != nil {
		r.Status = StatusUnavailable
		r.Detail = "helm not on PATH"
		r.Hint = MissingHint(IDHelm)
		return r
	}
	r.Status = StatusAvailable
	r.Detail = path
	return r
}
