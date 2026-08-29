package investigate

import (
	"context"
	"fmt"
	"strings"

	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kprompt/kprompt/internal/cluster"
	"github.com/kprompt/kprompt/internal/incident"
)

// ingressHops finds Ingress objects whose backends select Services that match podLabels.
func (inv *Investigator) ingressHops(ctx context.Context, ns string, podLabels map[string]string) (
	hops []cluster.ChainStep,
	findings []incident.Finding,
	evidence []incident.EvidenceRef,
	walked bool,
) {
	if len(podLabels) == 0 {
		return nil, nil, nil, false
	}

	ings, err := inv.Client.NetworkingV1().Ingresses(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		findings = append(findings, incident.Finding{
			Code:     "IngressListError",
			Severity: incident.SeverityLow,
			Title:    "Could not list Ingresses",
			Message:  err.Error(),
		})
		return hops, findings, evidence, false
	}
	walked = true

	// Cache Service selectors for backends we care about.
	svcSel := map[string]map[string]string{}
	getSel := func(name string) (map[string]string, bool) {
		if sel, ok := svcSel[name]; ok {
			return sel, sel != nil
		}
		svc, err := inv.Client.CoreV1().Services(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil || svc.Spec.Selector == nil {
			svcSel[name] = nil
			return nil, false
		}
		svcSel[name] = svc.Spec.Selector
		return svc.Spec.Selector, true
	}

	for _, ing := range ings.Items {
		backends := ingressBackendServices(ing)
		var matched []string
		for _, svcName := range backends {
			sel, ok := getSel(svcName)
			if !ok || !selectorMatches(sel, podLabels) {
				continue
			}
			matched = append(matched, svcName)
		}
		if len(matched) == 0 {
			continue
		}
		hosts := ingressHosts(ing)
		detail := "backends " + strings.Join(matched, ",")
		if hosts != "" {
			detail = hosts + " · " + detail
		}
		hops = append(hops, cluster.ChainStep{
			Level:  "Ingress",
			Name:   ing.Name,
			Detail: detail,
		})
		evidence = append(evidence, incident.EvidenceRef{
			Type: incident.EvidenceObject,
			Resource: &incident.ResourceRef{
				Kind: "Ingress", Name: ing.Name, Namespace: ns,
			},
			Reason:  "Selected",
			Message: detail,
			Source:  "kubernetes",
		})
		if class := ingressClassName(ing); class != "" {
			findings = append(findings, incident.Finding{
				Code:      "IngressAttached",
				Severity:  incident.SeverityInfo,
				Title:     "Ingress/" + ing.Name + " routes to workload",
				Message:   fmt.Sprintf("class=%s · %s", class, detail),
				Namespace: ns,
			})
		}
	}
	return hops, findings, evidence, walked
}

func ingressBackendServices(ing networkingv1.Ingress) []string {
	seen := map[string]struct{}{}
	var out []string
	add := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		if _, ok := seen[name]; ok {
			return
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	if ing.Spec.DefaultBackend != nil && ing.Spec.DefaultBackend.Service != nil {
		add(ing.Spec.DefaultBackend.Service.Name)
	}
	for _, rule := range ing.Spec.Rules {
		if rule.HTTP == nil {
			continue
		}
		for _, p := range rule.HTTP.Paths {
			if p.Backend.Service != nil {
				add(p.Backend.Service.Name)
			}
		}
	}
	return out
}

func ingressHosts(ing networkingv1.Ingress) string {
	var hosts []string
	seen := map[string]struct{}{}
	for _, rule := range ing.Spec.Rules {
		h := strings.TrimSpace(rule.Host)
		if h == "" {
			continue
		}
		if _, ok := seen[h]; ok {
			continue
		}
		seen[h] = struct{}{}
		hosts = append(hosts, h)
	}
	if len(hosts) == 0 {
		return ""
	}
	return "hosts " + strings.Join(hosts, ",")
}

func ingressClassName(ing networkingv1.Ingress) string {
	if ing.Spec.IngressClassName != nil {
		return strings.TrimSpace(*ing.Spec.IngressClassName)
	}
	if v := strings.TrimSpace(ing.Annotations["kubernetes.io/ingress.class"]); v != "" {
		return v
	}
	return ""
}
