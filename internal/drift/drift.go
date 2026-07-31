// Package drift reports live cluster vs GitOps desired state (S-008 · T-086).
//
// The MVP reads Flux Kustomization / Argo CD Application sync+health via
// tools/gitops and emits ADR-0014 Investigation findings. It never mutates.
// Approve-gated sync plans may be offered separately (pairs T-043).
package drift

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"k8s.io/client-go/rest"

	"github.com/kprompt/kprompt/internal/incident"
	"github.com/kprompt/kprompt/internal/tools/gitops"
)

const (
	CodeOutOfSync     = "Drift.OutOfSync"
	CodeUnhealthy     = "Drift.Unhealthy"
	CodeResourceDrift = "Drift.ResourceOutOfSync"
	CodeMissing       = "Drift.GitOpsMissing"
)

// Request scopes a drift scan.
type Request struct {
	Namespace string // empty = cluster-wide
	Name      string // optional single app
	Engine    string // flux | argocd | auto
	Prompt    string
}

// Analyzer compares GitOps desired state to live sync/health.
type Analyzer struct {
	Config *rest.Config
	// Status overrides SummarizeStatus (tests).
	Status func(ctx context.Context, cfg *rest.Config, req gitops.StatusRequest) (gitops.StatusReport, error)
	// Resources lists per-app drifted child resources (Argo status.resources). Optional.
	Resources func(ctx context.Context, cfg *rest.Config, app gitops.AppStatus) ([]gitops.ResourceDrift, error)
}

// Run returns drift findings for GitOps apps in scope.
func (a *Analyzer) Run(ctx context.Context, req Request) (incident.Investigation, error) {
	if a == nil || a.Config == nil {
		return incident.Investigation{}, fmt.Errorf("drift: rest config required")
	}
	statusFn := a.Status
	if statusFn == nil {
		statusFn = gitops.SummarizeStatus
	}
	ns := strings.TrimSpace(req.Namespace)
	out := incident.NewInvestigation(req.Prompt, ns)
	if ns == "" {
		out.Namespace = "all"
		out.Target = &incident.ResourceRef{Kind: "Cluster", Name: "cluster"}
	} else {
		out.Target = &incident.ResourceRef{Kind: "Namespace", Name: ns, Namespace: ns}
	}

	rep, err := statusFn(ctx, a.Config, gitops.StatusRequest{
		Namespace: ns,
		Name:      strings.TrimSpace(req.Name),
		Engine:    req.Engine,
	})
	if err != nil {
		return incident.Investigation{}, err
	}

	for _, n := range rep.Notes {
		if strings.Contains(strings.ToLower(n), "not installed") ||
			strings.Contains(strings.ToLower(n), "not available") ||
			strings.Contains(strings.ToLower(n), "no flux or argo") {
			out.Degraded = appendUnique(out.Degraded, "gitops")
		}
	}

	if len(rep.Apps) == 0 && (containsNote(rep.Notes, "not installed") ||
		containsNote(rep.Notes, "not available") ||
		strings.Contains(strings.ToLower(rep.Summary), "not available")) {
		out.Findings = append(out.Findings, incident.Finding{
			Code:     CodeMissing,
			Severity: incident.SeverityInfo,
			Title:    "GitOps not installed",
			Message:  "Neither Flux Kustomization nor Argo CD Application CRDs were detected. Drift vs Git cannot be assessed until a GitOps controller is installed.",
			Evidence: []incident.EvidenceRef{{
				Type:    incident.EvidenceGitOps,
				Source:  "gitops",
				Message: strings.Join(rep.Notes, "; "),
			}},
		})
		out.Summary = "GitOps controllers not available — drift scan degraded"
		out.Confidence = 0.95
		out.SuggestedPlanHint = "Install Flux or Argo CD, then re-run: kprompt \"check drift\". For PR mode, see docs/gitops-pr.md and use kprompt --gitops --gitops-repo owner/name \"...\"."
		sortFindings(&out)
		if err := incident.ValidateInvestigation(out); err != nil {
			return out, err
		}
		return out, nil
	}

	outOfSync, unhealthy, resourceDrifts := 0, 0, 0
	for _, app := range rep.Apps {
		ref := appRef(app)
		syncOO := isOutOfSync(app.Sync)
		healthBad := isUnhealthy(app.Health)

		if syncOO {
			outOfSync++
			msg := fmt.Sprintf("%s %s/%s is out of sync with Git", app.Engine, app.Kind, app.Name)
			if app.Revision != "" {
				msg += " (revision " + app.Revision + ")"
			}
			if app.Message != "" {
				msg += ": " + app.Message
			}
			out.Findings = append(out.Findings, incident.Finding{
				Code:      CodeOutOfSync,
				Severity:  incident.SeverityHigh,
				Title:     "GitOps app out of sync",
				Message:   msg,
				Namespace: app.Namespace,
				Evidence: []incident.EvidenceRef{{
					Type:     incident.EvidenceGitOps,
					Source:   app.Engine,
					Resource: ref,
					Message:  fmt.Sprintf("sync=%s health=%s", app.Sync, app.Health),
				}},
			})
		} else if healthBad {
			unhealthy++
			msg := fmt.Sprintf("%s %s/%s is synced but unhealthy (%s)", app.Engine, app.Kind, app.Name, app.Health)
			if app.Message != "" {
				msg += ": " + app.Message
			}
			out.Findings = append(out.Findings, incident.Finding{
				Code:      CodeUnhealthy,
				Severity:  incident.SeverityMedium,
				Title:     "GitOps app unhealthy",
				Message:   msg,
				Namespace: app.Namespace,
				Evidence: []incident.EvidenceRef{{
					Type:     incident.EvidenceGitOps,
					Source:   app.Engine,
					Resource: ref,
					Message:  fmt.Sprintf("sync=%s health=%s", app.Sync, app.Health),
				}},
			})
		}

		if syncOO && strings.EqualFold(app.Engine, "argocd") {
			resFn := a.Resources
			if resFn == nil {
				resFn = gitops.ListResourceDrifts
			}
			drifts, err := resFn(ctx, a.Config, app)
			if err != nil {
				out.Degraded = appendUnique(out.Degraded, "argocd-resources")
				continue
			}
			for _, d := range drifts {
				if resourceDrifts >= 20 {
					break
				}
				resourceDrifts++
				rref := &incident.ResourceRef{
					APIVersion: d.APIVersion,
					Kind:       d.Kind,
					Name:       d.Name,
					Namespace:  d.Namespace,
				}
				out.Findings = append(out.Findings, incident.Finding{
					Code:      CodeResourceDrift,
					Severity:  incident.SeverityMedium,
					Title:     "Live resource differs from Git",
					Message:   fmt.Sprintf("%s/%s (from %s/%s) status=%s", d.Kind, d.Name, app.Kind, app.Name, d.Status),
					Namespace: firstNonEmpty(d.Namespace, app.Namespace),
					Evidence: []incident.EvidenceRef{{
						Type:     incident.EvidenceGitOps,
						Source:   "argocd",
						Resource: rref,
						Message:  fmt.Sprintf("parent=%s/%s", app.Namespace, app.Name),
					}},
				})
			}
		}
	}

	scope := "cluster-wide"
	if ns != "" {
		scope = "namespace " + ns
	}
	nFindings := len(out.Findings)
	switch {
	case nFindings == 0 && len(rep.Apps) == 0:
		out.Summary = fmt.Sprintf("No GitOps apps found (%s); nothing to compare", scope)
		out.Confidence = 0.7
	case nFindings == 0:
		out.Summary = fmt.Sprintf("No drift across %d GitOps app(s) (%s)", len(rep.Apps), scope)
		out.Confidence = 0.85
	default:
		out.Summary = fmt.Sprintf(
			"%d drift finding(s) across %d app(s) (%s): %d out-of-sync, %d unhealthy, %d resource-level",
			nFindings, len(rep.Apps), scope, outOfSync, unhealthy, resourceDrifts,
		)
		out.Confidence = 0.9
		out.RootCause = "Live cluster state differs from the GitOps desired revision (manual change, failed sync, or pending reconcile)"
	}
	out.SuggestedPlanHint = "Review findings; approve a GitOps sync to reconcile, or open a PR with kprompt --gitops --gitops-repo owner/name \"...\". See docs/gitops-pr.md. Drift never auto-syncs."

	sortFindings(&out)
	if err := incident.ValidateInvestigation(out); err != nil {
		return out, err
	}
	return out, nil
}

func appRef(app gitops.AppStatus) *incident.ResourceRef {
	api := gitops.ArgoCDGroup + "/v1alpha1"
	if strings.EqualFold(app.Engine, "flux") {
		api = gitops.FluxGroup + "/v1"
	}
	return &incident.ResourceRef{
		APIVersion: api,
		Kind:       app.Kind,
		Name:       app.Name,
		Namespace:  app.Namespace,
	}
}

func isOutOfSync(sync string) bool {
	s := strings.TrimSpace(sync)
	return strings.EqualFold(s, "OutOfSync") || strings.EqualFold(s, "False")
}

func isUnhealthy(health string) bool {
	h := strings.TrimSpace(health)
	if h == "" {
		return false
	}
	if strings.EqualFold(h, "Healthy") || strings.EqualFold(h, "True") || strings.EqualFold(h, "Synced") {
		return false
	}
	// Progressing is informational — not drift.
	if strings.EqualFold(h, "Progressing") {
		return false
	}
	return true
}

func containsNote(notes []string, sub string) bool {
	sub = strings.ToLower(sub)
	for _, n := range notes {
		if strings.Contains(strings.ToLower(n), sub) {
			return true
		}
	}
	return false
}

func appendUnique(in []string, v string) []string {
	for _, x := range in {
		if x == v {
			return in
		}
	}
	return append(in, v)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func sortFindings(out *incident.Investigation) {
	sort.SliceStable(out.Findings, func(i, j int) bool {
		if out.Findings[i].Code != out.Findings[j].Code {
			return out.Findings[i].Code < out.Findings[j].Code
		}
		if out.Findings[i].Namespace != out.Findings[j].Namespace {
			return out.Findings[i].Namespace < out.Findings[j].Namespace
		}
		return out.Findings[i].Title < out.Findings[j].Title
	})
}
