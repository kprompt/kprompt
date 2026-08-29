package drift

import (
	"context"
	"strings"
	"testing"

	"k8s.io/client-go/rest"

	"github.com/kprompt/kprompt/internal/tools/gitops"
)

func TestRunOutOfSyncFindings(t *testing.T) {
	a := &Analyzer{
		Config: &rest.Config{Host: "https://example"},
		Status: func(context.Context, *rest.Config, gitops.StatusRequest) (gitops.StatusReport, error) {
			return gitops.StatusReport{
				Summary: "2 apps",
				Apps: []gitops.AppStatus{
					{Engine: "flux", Kind: "Kustomization", Name: "apps", Namespace: "flux-system", Sync: "OutOfSync", Health: "Degraded", Revision: "abc", Message: "dependency not ready"},
					{Engine: "argocd", Kind: "Application", Name: "payments", Namespace: "argocd", Sync: "Synced", Health: "Healthy"},
				},
			}, nil
		},
		Resources: func(context.Context, *rest.Config, gitops.AppStatus) ([]gitops.ResourceDrift, error) {
			return nil, nil
		},
	}
	inv, err := a.Run(context.Background(), Request{Prompt: "check drift", Namespace: "flux-system"})
	if err != nil {
		t.Fatal(err)
	}
	if len(inv.Findings) != 1 || inv.Findings[0].Code != CodeOutOfSync {
		t.Fatalf("findings=%+v", inv.Findings)
	}
	if inv.Findings[0].Evidence[0].Resource == nil || inv.Findings[0].Evidence[0].Resource.Name != "apps" {
		t.Fatalf("resource ref missing: %+v", inv.Findings[0])
	}
	if !strings.Contains(inv.Summary, "out-of-sync") {
		t.Fatalf("summary=%q", inv.Summary)
	}
}

func TestRunGitOpsMissing(t *testing.T) {
	a := &Analyzer{
		Config: &rest.Config{Host: "https://example"},
		Status: func(context.Context, *rest.Config, gitops.StatusRequest) (gitops.StatusReport, error) {
			return gitops.StatusReport{
				Summary: "GitOps controllers not available",
				Notes:   []string{"no Flux or Argo CD CRDs detected"},
			}, nil
		},
	}
	inv, err := a.Run(context.Background(), Request{Prompt: "drift"})
	if err != nil {
		t.Fatal(err)
	}
	if len(inv.Findings) != 1 || inv.Findings[0].Code != CodeMissing {
		t.Fatalf("findings=%+v", inv.Findings)
	}
	if len(inv.Degraded) == 0 || inv.Degraded[0] != "gitops" {
		t.Fatalf("degraded=%v", inv.Degraded)
	}
	if !strings.Contains(inv.SuggestedPlanHint, "docs/gitops-pr.md") || !strings.Contains(inv.SuggestedPlanHint, "--gitops --gitops-repo") {
		t.Fatalf("suggested hint=%q", inv.SuggestedPlanHint)
	}
	if strings.Contains(inv.SuggestedPlanHint, "T-072") || strings.Contains(inv.SuggestedPlanHint, "not shipped") {
		t.Fatalf("stale suggested hint=%q", inv.SuggestedPlanHint)
	}
}

func TestRunResourceDrifts(t *testing.T) {
	a := &Analyzer{
		Config: &rest.Config{Host: "https://example"},
		Status: func(context.Context, *rest.Config, gitops.StatusRequest) (gitops.StatusReport, error) {
			return gitops.StatusReport{
				Apps: []gitops.AppStatus{
					{Engine: "argocd", Kind: "Application", Name: "shop", Namespace: "argocd", Sync: "OutOfSync", Health: "Degraded"},
				},
			}, nil
		},
		Resources: func(context.Context, *rest.Config, gitops.AppStatus) ([]gitops.ResourceDrift, error) {
			return []gitops.ResourceDrift{{
				Kind: "Deployment", Name: "api", Namespace: "shop", Status: "OutOfSync", APIVersion: "apps/v1",
			}}, nil
		},
	}
	inv, err := a.Run(context.Background(), Request{Prompt: "check drift"})
	if err != nil {
		t.Fatal(err)
	}
	codes := map[string]int{}
	for _, f := range inv.Findings {
		codes[f.Code]++
	}
	if codes[CodeOutOfSync] != 1 || codes[CodeResourceDrift] != 1 {
		t.Fatalf("codes=%v findings=%+v", codes, inv.Findings)
	}
}

func TestRunFluxInventoryResourceDrifts(t *testing.T) {
	a := &Analyzer{
		Config: &rest.Config{Host: "https://example"},
		Status: func(context.Context, *rest.Config, gitops.StatusRequest) (gitops.StatusReport, error) {
			return gitops.StatusReport{
				Apps: []gitops.AppStatus{
					{Engine: "flux", Kind: "Kustomization", Name: "apps", Namespace: "flux-system", Sync: "OutOfSync", Health: "Degraded"},
				},
			}, nil
		},
		Resources: func(_ context.Context, _ *rest.Config, app gitops.AppStatus) ([]gitops.ResourceDrift, error) {
			if app.Engine != "flux" {
				t.Fatalf("engine=%s", app.Engine)
			}
			return []gitops.ResourceDrift{{
				Kind: "Deployment", Name: "api", Namespace: "shop", Status: "OutOfSync", APIVersion: "apps/v1",
			}}, nil
		},
	}
	inv, err := a.Run(context.Background(), Request{Prompt: "check flux drift"})
	if err != nil {
		t.Fatal(err)
	}
	codes := map[string]int{}
	for _, f := range inv.Findings {
		codes[f.Code]++
		if f.Code == CodeResourceDrift && f.Evidence[0].Source != "flux" {
			t.Fatalf("source=%q", f.Evidence[0].Source)
		}
	}
	if codes[CodeOutOfSync] != 1 || codes[CodeResourceDrift] != 1 {
		t.Fatalf("codes=%v", codes)
	}
}

func TestRunFluxInventoryDegrade(t *testing.T) {
	a := &Analyzer{
		Config: &rest.Config{Host: "https://example"},
		Status: func(context.Context, *rest.Config, gitops.StatusRequest) (gitops.StatusReport, error) {
			return gitops.StatusReport{
				Apps: []gitops.AppStatus{
					{Engine: "flux", Kind: "Kustomization", Name: "apps", Namespace: "flux-system", Sync: "OutOfSync"},
				},
			}, nil
		},
		Resources: func(context.Context, *rest.Config, gitops.AppStatus) ([]gitops.ResourceDrift, error) {
			return nil, errFluxInventoryUnavailable
		},
	}
	inv, err := a.Run(context.Background(), Request{Prompt: "drift"})
	if err != nil {
		t.Fatal(err)
	}
	if !contains(inv.Degraded, "flux-inventory") {
		t.Fatalf("degraded=%v", inv.Degraded)
	}
}

type unavailableErr string

func (e unavailableErr) Error() string { return string(e) }

const errFluxInventoryUnavailable = unavailableErr("flux inventory unavailable")

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

func TestRunClean(t *testing.T) {
	a := &Analyzer{
		Config: &rest.Config{Host: "https://example"},
		Status: func(context.Context, *rest.Config, gitops.StatusRequest) (gitops.StatusReport, error) {
			return gitops.StatusReport{
				Apps: []gitops.AppStatus{
					{Engine: "flux", Kind: "Kustomization", Name: "apps", Namespace: "flux-system", Sync: "Synced", Health: "Healthy"},
				},
			}, nil
		},
	}
	inv, err := a.Run(context.Background(), Request{Prompt: "drift"})
	if err != nil {
		t.Fatal(err)
	}
	if len(inv.Findings) != 0 {
		t.Fatalf("findings=%+v", inv.Findings)
	}
	if !strings.Contains(inv.Summary, "No drift") {
		t.Fatalf("summary=%q", inv.Summary)
	}
	if !strings.Contains(inv.SuggestedPlanHint, "--gitops --gitops-repo") || !strings.Contains(inv.SuggestedPlanHint, "docs/gitops-pr.md") {
		t.Fatalf("suggested hint=%q", inv.SuggestedPlanHint)
	}
	if strings.Contains(inv.SuggestedPlanHint, "T-072") || strings.Contains(inv.SuggestedPlanHint, "not shipped") {
		t.Fatalf("stale suggested hint=%q", inv.SuggestedPlanHint)
	}
}
