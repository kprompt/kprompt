package autopilot

import (
	"fmt"
	"sort"
	"strings"

	"github.com/kprompt/kprompt/internal/agent/ctxbuild"
	"github.com/kprompt/kprompt/internal/agent/patterns"
)

// candidate is one allowlisted action that matched incident signals (RT-002).
type candidate struct {
	Action   string
	Kind     string
	Name     string
	Replicas *int32
	Base     float64 // detector priority (higher first)
}

func detectAction(agentCtx ctxbuild.AgentContext) (action, kind, name string, replicas *int32, ok bool) {
	cands := detectCandidates(agentCtx)
	if len(cands) == 0 {
		return "", "", "", nil, false
	}
	c := cands[0]
	return c.Action, c.Kind, c.Name, c.Replicas, true
}

// detectCandidates returns all matching Autopilot actions, base-ranked (RT-002).
func detectCandidates(agentCtx ctxbuild.AgentContext) []candidate {
	blob := strings.ToLower(agentCtx.Incident.Summary + " " + agentCtx.Incident.RootCause + " " + agentCtx.Incident.Recommendation)
	for _, e := range agentCtx.Incident.Evidence {
		blob += " " + strings.ToLower(e.Reason+" "+e.Message)
	}
	for _, e := range agentCtx.RecentEvents {
		blob += " " + strings.ToLower(e.Reason+" "+e.Message)
	}

	_, name := resolveTarget(agentCtx)
	if name == "" {
		return nil
	}

	var out []candidate

	failedRollout := strings.Contains(blob, "progressdeadline") ||
		(strings.Contains(blob, "rollout") && (strings.Contains(blob, "failed") || strings.Contains(blob, "timed out") || strings.Contains(blob, "timeout")))
	// Classic bad-deploy shape before ProgressDeadline fires: ImagePull/CrashLoop on a
	// new revision while change-cause / ReplicaSet language is present → prefer rollback
	// over restart (restart cannot heal a bad image tag).
	workloadBad := strings.Contains(blob, "crashloop") || strings.Contains(blob, "oom") ||
		strings.Contains(blob, "imagepull") || strings.Contains(blob, "errimagepull") ||
		strings.Contains(blob, "image pull") || strings.Contains(blob, "backoff")
	revisionSignal := strings.Contains(blob, "revision") || strings.Contains(blob, "change-cause") ||
		strings.Contains(blob, "change cause") || strings.Contains(blob, "replicaset")
	if !failedRollout && workloadBad && revisionSignal {
		failedRollout = true
	}
	if failedRollout {
		out = append(out, candidate{
			Action: ActionRollbackFailedRollout,
			Kind:   "Deployment",
			Name:   trimPodToDeploy(name),
			Base:   40,
		})
	}

	restartish := workloadBad
	if restartish {
		out = append(out, candidate{
			Action: ActionRestartDeployment,
			Kind:   "Deployment",
			Name:   trimPodToDeploy(name),
			Base:   30,
		})
	}

	evictish := strings.Contains(blob, "node not ready") || strings.Contains(blob, "nodenotready") ||
		strings.Contains(blob, "diskpressure") || strings.Contains(blob, "memorypressure") ||
		strings.Contains(blob, "pidpressure") || strings.Contains(blob, "evict")
	if evictish {
		pod := name
		if agentCtx.Pod != nil && agentCtx.Pod.Name != "" {
			pod = agentCtx.Pod.Name
		} else if agentCtx.Target != nil && strings.EqualFold(agentCtx.Target.Kind, "Pod") {
			pod = agentCtx.Target.Name
		}
		out = append(out, candidate{
			Action: ActionEvictPod,
			Kind:   "Pod",
			Name:   pod,
			Base:   20,
		})
	}

	scaleish := strings.Contains(blob, "scale up") || strings.Contains(blob, "scale down") ||
		strings.Contains(blob, "scale deployment") ||
		(strings.Contains(blob, "replicas") && (strings.Contains(blob, "zero") || strings.Contains(blob, "under-repl")))
	if scaleish && agentCtx.Deployment != nil {
		var want int32 = 1
		if agentCtx.Deployment.DesiredReplicas > 0 {
			want = agentCtx.Deployment.DesiredReplicas
		}
		if strings.Contains(blob, "scale up") || strings.Contains(blob, "under-repl") {
			want = agentCtx.Deployment.DesiredReplicas + 1
			if want < 1 {
				want = 1
			}
			if want > 20 {
				want = 20 // safety cap
			}
		}
		if strings.Contains(blob, "scale down") && agentCtx.Deployment.DesiredReplicas > 1 {
			want = agentCtx.Deployment.DesiredReplicas - 1
		}
		out = append(out, candidate{
			Action:   ActionScaleDeployment,
			Kind:     "Deployment",
			Name:     trimPodToDeploy(name),
			Replicas: &want,
			Base:     10,
		})
	}

	sort.SliceStable(out, func(i, j int) bool { return out[i].Base > out[j].Base })
	return out
}

// rankCandidates applies Learn bias from a matched pattern (RT-002 · AG-034).
// Prefer LastActionID on success history; dampen when weight is low / FP-heavy.
func rankCandidates(cands []candidate, match patterns.Pattern, matched bool) []candidate {
	if len(cands) <= 1 || !matched {
		return cands
	}
	type scored struct {
		c candidate
		s float64
	}
	var list []scored
	w := match.Weight
	if w <= 0 {
		w = 1
	}
	for _, c := range cands {
		s := c.Base
		if match.LastActionID != "" && c.Action == match.LastActionID {
			s += 15 * w
			if match.Confirmed > 0 {
				s += float64(match.Confirmed)
			}
		}
		if match.FalsePositives > match.Confirmed && match.FalsePositives >= 2 && c.Action == match.LastActionID {
			s -= 20
		}
		if w < 0.5 && c.Action == match.LastActionID {
			s -= 10
		}
		list = append(list, scored{c: c, s: s})
	}
	sort.SliceStable(list, func(i, j int) bool { return list[i].s > list[j].s })
	out := make([]candidate, len(list))
	for i := range list {
		out[i] = list[i].c
	}
	return out
}

// biasActionConfidence adjusts proposal ActionConfidence from pattern weight (RT-002).
// Alert Confidence gate still uses the raw confidence; this is explainability + ranking only.
func biasActionConfidence(confidence float64, match patterns.Pattern, matched bool) (biased float64, note string) {
	biased = confidence
	if !matched || match.Count < patterns.MinPriorCount {
		return biased, ""
	}
	boost := patterns.EffectiveBoost(match)
	w := match.Weight
	if w <= 0 {
		w = 1
	}
	delta := boost * (w - 0.5)
	biased = clamp01(confidence + delta)
	note = fmt.Sprintf("Learn bias: weight=%.2f confirmed=%d fp=%d Δ=%.2f", w, match.Confirmed, match.FalsePositives, delta)
	if match.LastActionID != "" {
		note += fmt.Sprintf(" lastAction=%s", match.LastActionID)
	}
	return biased, note
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func resolveTarget(agentCtx ctxbuild.AgentContext) (kind, name string) {
	kind = "Deployment"
	if agentCtx.Deployment != nil {
		name = agentCtx.Deployment.Name
	}
	if agentCtx.Target != nil && agentCtx.Target.Name != "" {
		name = firstNonEmpty(name, agentCtx.Target.Name)
		if agentCtx.Target.Kind != "" {
			kind = agentCtx.Target.Kind
		}
	}
	if name == "" && agentCtx.Incident.PrimaryResource != nil {
		name = agentCtx.Incident.PrimaryResource.Name
		if agentCtx.Incident.PrimaryResource.Kind != "" {
			kind = agentCtx.Incident.PrimaryResource.Kind
		}
	}
	if name == "" && agentCtx.Pod != nil {
		name = agentCtx.Pod.Name
		kind = "Pod"
	}
	return kind, name
}

func trimPodToDeploy(name string) string {
	parts := strings.Split(name, "-")
	if len(parts) >= 3 {
		return strings.Join(parts[:len(parts)-2], "-")
	}
	return name
}

func planFor(action, ns, name string, replicas *int32) PlanBody {
	switch action {
	case ActionRollbackFailedRollout:
		return PlanBody{
			Summary: fmt.Sprintf("Rollback Deployment/%s in %s after failed rollout", name, ns),
			Steps: []string{
				fmt.Sprintf("kubectl -n %s rollout undo deployment/%s", ns, name),
				fmt.Sprintf("kubectl -n %s rollout status deployment/%s", ns, name),
			},
		}
	case ActionRestartDeployment:
		return PlanBody{
			Summary: fmt.Sprintf("Restart Deployment/%s in %s (rollout restart)", name, ns),
			Steps: []string{
				fmt.Sprintf("kubectl -n %s rollout restart deployment/%s", ns, name),
				fmt.Sprintf("kubectl -n %s rollout status deployment/%s", ns, name),
			},
		}
	case ActionScaleDeployment:
		n := int32(1)
		if replicas != nil {
			n = *replicas
		}
		return PlanBody{
			Summary: fmt.Sprintf("Scale Deployment/%s in %s to %d replicas", name, ns, n),
			Steps: []string{
				fmt.Sprintf("kubectl -n %s scale deployment/%s --replicas=%d", ns, name, n),
			},
		}
	case ActionEvictPod:
		return PlanBody{
			Summary: fmt.Sprintf("Evict Pod/%s in %s (graceful delete)", name, ns),
			Steps: []string{
				fmt.Sprintf("kubectl -n %s delete pod/%s --grace-period=30", ns, name),
			},
		}
	default:
		return PlanBody{Summary: "unknown", Steps: nil}
	}
}

func enrichExplain(p *Proposal, action, ns, name string) {
	if p == nil {
		return
	}
	switch action {
	case ActionRollbackFailedRollout:
		p.Why = "Deployment rollout failed or timed out; prior revision is likely healthier"
		p.ExpectedImpact = "Traffic returns to the last successful ReplicaSet"
		p.Rollback = fmt.Sprintf("kubectl -n %s rollout undo deployment/%s", ns, name)
		p.Risk = firstNonEmpty(p.Risk, "medium")
	case ActionRestartDeployment:
		p.Why = "Workload is crashlooping / OOM / image-pull failing; a restart may clear a bad pod state"
		p.ExpectedImpact = "New pods scheduled; brief disruption possible"
		p.Rollback = "No cluster object change beyond pod replacement; redeploy previous image if needed"
		p.Risk = firstNonEmpty(p.Risk, "medium")
	case ActionScaleDeployment:
		p.Why = "Replica count appears wrong for current demand or availability"
		p.ExpectedImpact = "Replica count changes; capacity / cost impact"
		p.Rollback = "Scale back to the previous replica count"
		p.Risk = firstNonEmpty(p.Risk, "medium")
	case ActionEvictPod:
		p.Why = "Node pressure / eviction signal; reschedule pod onto a healthier node"
		p.ExpectedImpact = "Pod deleted and recreated by its controller"
		p.Rollback = "Controller recreates the pod; no further rollback"
		p.Risk = firstNonEmpty(p.Risk, "low")
	}
}

func riskFor(action string) string {
	switch action {
	case ActionEvictPod:
		return "low"
	case ActionRestartDeployment, ActionScaleDeployment, ActionRollbackFailedRollout:
		return "medium"
	default:
		return "high"
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
