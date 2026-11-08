package autopilot

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/client-go/kubernetes"

	"github.com/kprompt/kprompt/internal/agent/ctxbuild"
	"github.com/kprompt/kprompt/internal/agent/patterns"
	"github.com/kprompt/kprompt/internal/incident"
	"github.com/kprompt/kprompt/internal/planner"
	"github.com/kprompt/kprompt/internal/verify"
)

// AttachVerify runs T-070 verify against the proposal’s mutate plan and stamps
// VerifyStatus / VerifyMessage / Outcome on the proposal (RT-001 · RT-006).
// When verify fails, Decision becomes failed and Applied is cleared.
func AttachVerify(ctx context.Context, client kubernetes.Interface, prop *Proposal) verify.Report {
	if prop == nil {
		return verify.Report{Status: verify.Skipped, Message: "nil proposal"}
	}
	plan, err := verifyPlanFor(*prop)
	if err != nil {
		prop.VerifyStatus = verify.Skipped
		prop.VerifyMessage = err.Error()
		return verify.Report{Status: verify.Skipped, Message: err.Error()}
	}
	rep := verify.Plan(ctx, client, plan)
	prop.VerifyStatus = rep.Status
	prop.VerifyMessage = rep.Message
	if o, ok := patterns.OutcomeFromVerify(rep.Status); ok {
		prop.Outcome = string(o)
	}
	if rep.Status == verify.Failed {
		prop.Decision = DecisionFailed
		prop.Applied = false
		prop.Reason = fmt.Sprintf("verify failed: %s", rep.Message)
	}
	return rep
}

// AttachProposalToAlert stamps AgentAlert with durable proposal metadata (RT-017 · RT-018).
// Skips denied proposals. Never implies apply — hint is human CLI only.
func AttachProposalToAlert(alert *incident.AgentAlert, prop *Proposal) {
	if alert == nil || prop == nil {
		return
	}
	if prop.Decision == DecisionDenied {
		return
	}
	alert.ProposalID = prop.ID
	alert.ProposalAction = prop.ActionID
	alert.ProposalRisk = prop.Risk
	if impact := strings.TrimSpace(prop.ExpectedImpact); impact != "" {
		alert.ProposalImpact = impact
	}
	ns := strings.TrimSpace(prop.Namespace)
	if ns == "" {
		ns = alert.Namespace
	}
	// Same approve vocabulary as CLI --approve / TTY "Apply this plan? [y/N]".
	alert.ProposalHint = fmt.Sprintf(
		"kprompt agent proposals apply -n %s --id %s --approve",
		ns, prop.ID,
	)
}

// WriteLearnOutcome records apply/verify outcome on the pattern store (RT-001).
// No-op when lib is nil. On mutate error before verify, pass OutcomeApplyFailed.
func WriteLearnOutcome(lib *patterns.Library, agentCtx ctxbuild.AgentContext, outcome patterns.Outcome) (patterns.Pattern, error) {
	return WriteLearnOutcomeAction(lib, agentCtx, outcome, "")
}

// WriteLearnOutcomeAction records outcome and stamps LastActionID for RT-002 ranking.
func WriteLearnOutcomeAction(lib *patterns.Library, agentCtx ctxbuild.AgentContext, outcome patterns.Outcome, actionID string) (patterns.Pattern, error) {
	if lib == nil {
		return patterns.Pattern{}, nil
	}
	if outcome == "" {
		return patterns.Pattern{}, nil
	}
	ns := strings.TrimSpace(agentCtx.Namespace)
	if ns == "" {
		ns = strings.TrimSpace(agentCtx.Incident.Namespace)
	}
	if ns == "" {
		return patterns.Pattern{}, fmt.Errorf("autopilot learn: empty namespace")
	}
	if actionID != "" {
		return lib.RecordOutcomeAction(ns, agentCtx, outcome, actionID)
	}
	return lib.RecordOutcome(ns, agentCtx, outcome)
}

// ContextFromProposal builds a minimal AgentContext for pattern Signature matching
// when the full analyze context is unavailable (apply-proposal CLI).
func ContextFromProposal(prop Proposal) ctxbuild.AgentContext {
	kind := strings.TrimSpace(prop.TargetKind)
	if kind == "" {
		kind = "Deployment"
	}
	name := strings.TrimSpace(prop.TargetName)
	ref := &incident.ResourceRef{Kind: kind, Name: name, Namespace: prop.Namespace}
	summary := strings.TrimSpace(prop.Plan.Summary)
	if summary == "" {
		summary = prop.ActionID + " " + name
	}
	// Seed evidence reason from action so Signature buckets align with common detectors.
	reason := reasonForAction(prop.ActionID)
	return ctxbuild.AgentContext{
		Namespace: prop.Namespace,
		Incident: incident.Incident{
			ID:              prop.IncidentID,
			Namespace:       prop.Namespace,
			Summary:         summary,
			PrimaryResource: ref,
			Evidence: []incident.EvidenceRef{{
				Type:    incident.EvidenceEvent,
				Reason:  reason,
				Message: summary,
			}},
		},
		Target: ref,
	}
}

func reasonForAction(actionID string) string {
	switch actionID {
	case ActionRollbackFailedRollout:
		return "ProgressDeadlineExceeded"
	case ActionRestartDeployment:
		return "CrashLoopBackOff"
	case ActionScaleDeployment:
		return "ScalingReplicaSet"
	case ActionEvictPod:
		return "Evicted"
	default:
		return "Other"
	}
}

func verifyPlanFor(prop Proposal) (planner.ExecutionPlan, error) {
	var plan planner.ExecutionPlan
	var err error
	switch prop.ActionID {
	case ActionRestartDeployment:
		plan = planner.ExecutionPlan{
			Summary: prop.Plan.Summary,
			Actions: []planner.Action{{
				Backend: "kubernetes",
				Op:      planner.OpUpdate,
				Object:  planner.ObjectRef{Kind: "Deployment", Name: prop.TargetName, Namespace: prop.Namespace},
			}},
		}
	default:
		plan, err = proposalToPlan(prop)
		if err != nil {
			return planner.ExecutionPlan{}, err
		}
	}
	plan.RequiresApproval = true // verify.Plan skips unless gated
	return plan, nil
}
