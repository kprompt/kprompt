package investigate

import (
	"context"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/kprompt/kprompt/internal/cluster"
	"github.com/kprompt/kprompt/internal/incident"
	toolistio "github.com/kprompt/kprompt/internal/tools/istio"
)

// meshHops finds Istio VirtualServices whose route destinations target Services
// that select the workload. walked=true when the mesh API was queried (or absent).
func (inv *Investigator) meshHops(ctx context.Context, ns string, podLabels map[string]string) (
	hops []cluster.ChainStep,
	findings []incident.Finding,
	evidence []incident.EvidenceRef,
	walked bool,
) {
	if inv.Dynamic == nil {
		return nil, nil, nil, false
	}
	if len(podLabels) == 0 {
		return nil, nil, nil, true
	}

	svcNames, err := inv.matchingServiceNames(ctx, ns, podLabels)
	if err != nil {
		findings = append(findings, incident.Finding{
			Code:     "MeshServiceListError",
			Severity: incident.SeverityLow,
			Title:    "Could not list Services for mesh match",
			Message:  err.Error(),
		})
		return hops, findings, evidence, false
	}
	if len(svcNames) == 0 {
		return nil, nil, nil, true
	}

	list, err := inv.Dynamic.Resource(toolistio.VirtualServiceGVR).Namespace(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) || toolistio.IsNoMatchGVR(err) {
			return nil, nil, nil, true
		}
		findings = append(findings, incident.Finding{
			Code:     "VirtualServiceListError",
			Severity: incident.SeverityLow,
			Title:    "Could not list VirtualServices",
			Message:  err.Error(),
		})
		return hops, findings, evidence, false
	}
	walked = true

	for i := range list.Items {
		vs := &list.Items[i]
		dests := toolistio.VirtualServiceDestinationHosts(vs)
		var matched []string
		for _, host := range dests {
			if svc := toolistio.MatchHostToService(host, ns, svcNames); svc != "" {
				matched = append(matched, svc)
			}
		}
		if len(matched) == 0 {
			continue
		}
		matched = uniqueStrings(matched)
		hosts, _, _ := unstructured.NestedStringSlice(vs.Object, "spec", "hosts")
		detail := "destinations " + strings.Join(matched, ",")
		if len(hosts) > 0 {
			detail = "hosts " + strings.Join(hosts, ",") + " · " + detail
		}
		if toolistio.IsCanaryVirtualService(vs) {
			detail += " · weighted/canary routes"
		}
		hops = append(hops, cluster.ChainStep{
			Level:  "VirtualService",
			Name:   vs.GetName(),
			Detail: detail,
		})
		evidence = append(evidence, incident.EvidenceRef{
			Type: incident.EvidenceObject,
			Resource: &incident.ResourceRef{
				Kind: "VirtualService", Name: vs.GetName(), Namespace: ns,
				APIVersion: toolistio.VirtualServiceGroup + "/v1beta1",
			},
			Reason:  "Selected",
			Message: detail,
			Source:  "istio",
		})
		findings = append(findings, incident.Finding{
			Code:      "VirtualServiceAttached",
			Severity:  incident.SeverityInfo,
			Title:     "VirtualService/" + vs.GetName() + " routes to workload",
			Message:   detail,
			Namespace: ns,
		})
	}
	return hops, findings, evidence, walked
}

func (inv *Investigator) matchingServiceNames(ctx context.Context, ns string, podLabels map[string]string) ([]string, error) {
	svcs, err := inv.Client.CoreV1().Services(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	var names []string
	for _, svc := range svcs.Items {
		if svc.Spec.Selector == nil || len(svc.Spec.Selector) == 0 {
			continue
		}
		if selectorMatches(svc.Spec.Selector, podLabels) {
			names = append(names, svc.Name)
		}
	}
	return names, nil
}

func uniqueStrings(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
