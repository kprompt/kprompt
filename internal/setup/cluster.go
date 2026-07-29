package setup

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/kprompt/kprompt/internal/intent"
	"github.com/kprompt/kprompt/internal/planner"
	"github.com/kprompt/kprompt/internal/safety"
)

// Default namespaces for cluster-lane installs (T-064).
const (
	DefaultArgoNamespace        = "argo"
	DefaultPrometheusNamespace  = "monitoring"
	DefaultPrometheusRelease    = "kprompt-prom"
	argoWorkflowsInstallYAML    = "https://github.com/argoproj/argo-workflows/releases/download/v3.6.2/install.yaml"
	prometheusCommunityRepoURL  = "https://prometheus-community.github.io/helm-charts"
	prometheusCommunityRepoName = "prometheus-community"
	kubePrometheusStackChart    = "prometheus-community/kube-prometheus-stack"
)

// ClusterNeeded returns cluster-lane steps that still need install.
func ClusterNeeded(plan Plan) []Step {
	out := make([]Step, 0)
	for _, s := range plan.Steps {
		if s.Lane == LaneCluster && s.Status == StatusNeeded {
			out = append(out, s)
		}
	}
	return out
}

// ClusterApplyOptions configures approve-gated cluster installs.
type ClusterApplyOptions struct {
	KubeContext string
	ArgoNS      string
	PromNS      string
	PromRelease string
	Runner      CommandRunner
}

// ClusterResult is one cluster-lane apply outcome.
type ClusterResult struct {
	Component string `json:"component"`
	Status    string `json:"status"` // installed | skipped | failed | denied | unsupported
	Detail    string `json:"detail,omitempty"`
	Namespace string `json:"namespace,omitempty"`
}

// ClusterApplyReport summarizes cluster installs.
type ClusterApplyReport struct {
	Applied []ClusterResult `json:"applied"`
	Notes   []string        `json:"notes,omitempty"`
}

// BuildClusterPlan builds a PlanResult-shaped ExecutionPlan for one cluster step.
// Install-only — never uninstall / wipe.
func BuildClusterPlan(step Step, opts ClusterApplyOptions) (planner.ExecutionPlan, error) {
	argoNS := strings.TrimSpace(opts.ArgoNS)
	if argoNS == "" {
		argoNS = DefaultArgoNamespace
	}
	promNS := strings.TrimSpace(opts.PromNS)
	if promNS == "" {
		promNS = DefaultPrometheusNamespace
	}
	promRel := strings.TrimSpace(opts.PromRelease)
	if promRel == "" {
		promRel = DefaultPrometheusRelease
	}
	ctxKubectl := kubectlContextArgs(opts.KubeContext)
	ctxHelm := helmContextArgs(opts.KubeContext)

	switch step.Component {
	case "argo-workflows":
		actions := []planner.Action{
			{
				Op:      planner.OpCreate,
				Backend: "kubectl",
				Object:  planner.ObjectRef{Kind: "Namespace", Name: argoNS},
				Command: append([]string{"kubectl"}, append(ctxKubectl, "create", "namespace", argoNS)...),
				Diff:    fmt.Sprintf("ensure namespace %s exists", argoNS),
			},
			{
				Op:      planner.OpCreate,
				Backend: "kubectl",
				Object:  planner.ObjectRef{APIVersion: "argoproj.io/v1alpha1", Kind: "Workflow", Namespace: argoNS},
				Command: append([]string{"kubectl"}, append(ctxKubectl, "apply", "-n", argoNS, "-f", argoWorkflowsInstallYAML)...),
				Diff:    "apply Argo Workflows controller manifests (install-only)",
			},
		}
		return planner.ExecutionPlan{
			Intent: intent.Intent{
				Kind:   intent.KindInstall,
				Target: intent.Target{Kind: "Namespace", Name: argoNS, Namespace: argoNS},
			},
			Actions:          actions,
			Summary:          fmt.Sprintf("Install Argo Workflows into namespace %q (kubectl apply; no uninstall)", argoNS),
			RequiresApproval: true,
		}, nil

	case "prometheus":
		actions := []planner.Action{
			{
				Op:      planner.OpHelmRepo,
				Backend: "helm",
				Command: append([]string{"helm"}, append(ctxHelm, "repo", "add", prometheusCommunityRepoName, prometheusCommunityRepoURL)...),
				Diff:    "add prometheus-community helm repo",
			},
			{
				Op:      planner.OpHelmRepoUpdate,
				Backend: "helm",
				Command: append([]string{"helm"}, append(ctxHelm, "repo", "update", prometheusCommunityRepoName)...),
				Diff:    "helm repo update",
			},
			{
				Op:      planner.OpHelmInstall,
				Backend: "helm",
				Object: planner.ObjectRef{
					Kind: "Release", Name: promRel, Namespace: promNS,
				},
				Command: append([]string{"helm"}, append(ctxHelm,
					"install", promRel, kubePrometheusStackChart,
					"-n", promNS, "--create-namespace",
				)...),
				Diff: fmt.Sprintf("helm install %s into %s (install-only; no uninstall)", promRel, promNS),
			},
		}
		return planner.ExecutionPlan{
			Intent: intent.Intent{
				Kind:   intent.KindInstall,
				Target: intent.Target{Kind: "Namespace", Name: promNS, Namespace: promNS},
			},
			Actions:          actions,
			Summary:          fmt.Sprintf("Install kube-prometheus-stack release %q into namespace %q (Helm; no uninstall)", promRel, promNS),
			RequiresApproval: true,
		}, nil

	default:
		return planner.ExecutionPlan{}, fmt.Errorf("unsupported cluster component %q", step.Component)
	}
}

func kubectlContextArgs(contextName string) []string {
	contextName = strings.TrimSpace(contextName)
	if contextName == "" {
		return nil
	}
	return []string{"--context", contextName}
}

func helmContextArgs(contextName string) []string {
	contextName = strings.TrimSpace(contextName)
	if contextName == "" {
		return nil
	}
	return []string{"--kube-context", contextName}
}

// ApplyCluster runs approve-gated cluster installs after safety evaluation (T-064).
func ApplyCluster(ctx context.Context, plan Plan, opts ClusterApplyOptions, out io.Writer) (ClusterApplyReport, error) {
	if opts.Runner == nil {
		opts.Runner = DefaultRunner{}
	}
	rep := ClusterApplyReport{Notes: []string{
		"Cluster apply only installs operators. Wipe-class uninstalls are denied.",
		fmt.Sprintf("Namespace defaults: Argo=%s · Prometheus stack=%s",
			orDefault(opts.ArgoNS, DefaultArgoNamespace),
			orDefault(opts.PromNS, DefaultPrometheusNamespace)),
	}}

	needed := ClusterNeeded(plan)
	if len(needed) == 0 {
		rep.Notes = append(rep.Notes, "No cluster-lane steps needed.")
		return rep, nil
	}

	for _, step := range needed {
		execPlan, err := BuildClusterPlan(step, opts)
		if err != nil {
			rep.Applied = append(rep.Applied, ClusterResult{
				Component: step.Component,
				Status:    "unsupported",
				Detail:    err.Error(),
			})
			continue
		}
		risk := safety.EvaluatePlan(execPlan)
		if risk.Denied {
			rep.Applied = append(rep.Applied, ClusterResult{
				Component: step.Component,
				Status:    "denied",
				Detail:    risk.Message,
			})
			return rep, fmt.Errorf("safety denied %s: %s", step.Component, risk.Message)
		}
		if err := assertInstallOnly(execPlan); err != nil {
			rep.Applied = append(rep.Applied, ClusterResult{
				Component: step.Component,
				Status:    "denied",
				Detail:    err.Error(),
			})
			return rep, err
		}

		fmt.Fprintf(out, "Installing %s (risk=%s)…\n", step.Component, risk.Risk)
		ns := ""
		if len(execPlan.Actions) > 0 {
			ns = execPlan.Actions[len(execPlan.Actions)-1].Object.Namespace
		}
		if err := runClusterPlan(ctx, opts.Runner, execPlan, out); err != nil {
			rep.Applied = append(rep.Applied, ClusterResult{
				Component: step.Component,
				Status:    "failed",
				Detail:    err.Error(),
				Namespace: ns,
			})
			return rep, fmt.Errorf("%s: %w", step.Component, err)
		}
		detail := execPlan.Summary
		if step.Component == "prometheus" {
			detail += " — then: kprompt config set tools.prometheus.url http://kprompt-prom-kube-prometheus-stack-prometheus.monitoring.svc:9090"
		}
		rep.Applied = append(rep.Applied, ClusterResult{
			Component: step.Component,
			Status:    "installed",
			Detail:    detail,
			Namespace: ns,
		})
	}
	return rep, nil
}

func runClusterPlan(ctx context.Context, r CommandRunner, plan planner.ExecutionPlan, out io.Writer) error {
	for _, a := range plan.Actions {
		if len(a.Command) == 0 {
			return fmt.Errorf("empty command for op %s", a.Op)
		}
		if err := assertSafeArgv(a.Command); err != nil {
			return err
		}
		fmt.Fprintf(out, "  $ %s\n", strings.Join(a.Command, " "))
		name, args := a.Command[0], a.Command[1:]
		err := r.Run(ctx, name, args, nil)
		if err != nil && a.Op == planner.OpCreate && name == "kubectl" && containsArg(args, "namespace") {
			// Namespace already exists is OK.
			if strings.Contains(err.Error(), "AlreadyExists") || strings.Contains(err.Error(), "already exists") {
				fmt.Fprintln(out, "  (namespace already exists — continuing)")
				continue
			}
		}
		// helm repo add may fail if repo exists — treat as OK.
		if err != nil && a.Op == planner.OpHelmRepo && strings.Contains(strings.ToLower(err.Error()), "already exists") {
			fmt.Fprintln(out, "  (helm repo already exists — continuing)")
			continue
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func assertInstallOnly(plan planner.ExecutionPlan) error {
	for _, a := range plan.Actions {
		if err := assertSafeArgv(a.Command); err != nil {
			return err
		}
		switch a.Op {
		case planner.OpCreate, planner.OpHelmRepo, planner.OpHelmRepoUpdate, planner.OpHelmInstall:
			continue
		default:
			return fmt.Errorf("setup cluster apply refuses op %q (install-only)", a.Op)
		}
	}
	return nil
}

func assertSafeArgv(argv []string) error {
	joined := strings.ToLower(strings.Join(argv, " "))
	if strings.Contains(joined, "uninstall") ||
		(strings.Contains(joined, "helm") && strings.Contains(joined, " delete ")) ||
		strings.Contains(joined, " --all") && (strings.Contains(joined, "delete") || strings.Contains(joined, "uninstall")) {
		return fmt.Errorf("🛡️ Refusing wipe-class cluster command: %s", strings.Join(argv, " "))
	}
	if strings.Contains(joined, "delete") && strings.Contains(joined, "namespace") && !strings.Contains(joined, "create") {
		return fmt.Errorf("🛡️ Refusing namespace delete in setup apply: %s", strings.Join(argv, " "))
	}
	return nil
}

func containsArg(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

func orDefault(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

// FormatClusterApply writes cluster apply results.
func FormatClusterApply(w io.Writer, rep ClusterApplyReport) {
	fmt.Fprintln(w, "\nCluster apply:")
	if len(rep.Applied) == 0 {
		fmt.Fprintln(w, "  (nothing)")
	}
	for _, a := range rep.Applied {
		line := fmt.Sprintf("  - [%s] %s", a.Status, a.Component)
		if a.Namespace != "" {
			line += " -n " + a.Namespace
		}
		if a.Detail != "" {
			line += ": " + a.Detail
		}
		fmt.Fprintln(w, line)
	}
	for _, n := range rep.Notes {
		fmt.Fprintf(w, "  note: %s\n", n)
	}
}

// NamespaceDefaultsDoc documents install namespaces.
func NamespaceDefaultsDoc() string {
	return fmt.Sprintf(
		"Cluster install namespaces:\n  Argo Workflows → %s\n  kube-prometheus-stack → %s (release %s)\n",
		DefaultArgoNamespace, DefaultPrometheusNamespace, DefaultPrometheusRelease,
	)
}
