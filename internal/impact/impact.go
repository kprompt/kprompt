// Package impact finds static consumers and related blast-radius signals (S-005 · T-083).
//
// The MVP is intentionally read-only and deterministic. It walks Kubernetes
// objects in one namespace; runtime call edges remain an explicit OTel gap.
package impact

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	"github.com/kprompt/kprompt/internal/cluster"
	"github.com/kprompt/kprompt/internal/incident"
)

// Request identifies a Service or Deployment whose consumers should be found.
type Request struct {
	Name      string
	Namespace string
	Kind      string // Service or Deployment
	Prompt    string
}

// Analyzer walks reverse dependency edges and emits an Investigation document.
type Analyzer struct {
	Client  kubernetes.Interface
	Dynamic dynamic.Interface // optional; when set, VirtualService routes are walked
}

// Run returns static consumers plus workload relationships for the target.
func (a *Analyzer) Run(ctx context.Context, req Request) (incident.Investigation, error) {
	if a == nil || a.Client == nil {
		return incident.Investigation{}, fmt.Errorf("impact: client required")
	}
	ns := strings.TrimSpace(req.Namespace)
	if ns == "" {
		ns = "default"
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return incident.Investigation{}, fmt.Errorf("impact: target name required")
	}
	kind := cluster.NormalizeKind(req.Kind)
	if kind != "Service" && kind != "Deployment" {
		kind = "Service"
	}

	out := incident.NewInvestigation(req.Prompt, ns)
	out.Target = &incident.ResourceRef{Kind: kind, Name: name, Namespace: ns}
	out.Degraded = []string{"otel", "mesh"}

	var err error
	switch kind {
	case "Deployment":
		err = a.deploymentImpact(ctx, ns, name, &out)
	default:
		err = a.serviceImpact(ctx, ns, name, &out)
	}
	if err != nil {
		return incident.Investigation{}, err
	}
	sortInvestigation(&out)
	if err := incident.ValidateInvestigation(out); err != nil {
		return out, err
	}
	return out, nil
}

func (a *Analyzer) serviceImpact(ctx context.Context, ns, name string, out *incident.Investigation) error {
	svc, err := a.Client.CoreV1().Services(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	deps, err := a.Client.AppsV1().Deployments(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("list deployments: %w", err)
	}

	consumers := 0
	backends := 0
	for i := range deps.Items {
		dep := &deps.Items[i]
		if len(svc.Spec.Selector) > 0 &&
			labels.SelectorFromSet(svc.Spec.Selector).Matches(labels.Set(dep.Spec.Template.Labels)) {
			backends++
			addFinding(out, "Impact.Backend", incident.SeverityInfo,
				fmt.Sprintf("Deployment/%s is selected by Service/%s", dep.Name, name),
				"Service selector matches the Deployment pod template",
				"Deployment", dep.Name, ns, "Backend")
		}
		if detail := deploymentReference(dep.Spec.Template.Spec.Containers, name, ns); detail != "" {
			consumers++
			addFinding(out, "Impact.Consumer", incident.SeverityMedium,
				fmt.Sprintf("Deployment/%s consumes Service/%s", dep.Name, name),
				detail, "Deployment", dep.Name, ns, "Consumer")
		}
	}

	ingresses, err := a.Client.NetworkingV1().Ingresses(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		out.Degraded = appendUnique(out.Degraded, "ingress")
		ingresses = nil
	}
	routes := 0
	if ingresses != nil {
		for i := range ingresses.Items {
			ing := &ingresses.Items[i]
			if ingressRoutesTo(ing, name) {
				routes++
				addFinding(out, "Impact.Ingress", incident.SeverityInfo,
					fmt.Sprintf("Ingress/%s routes to Service/%s", ing.Name, name),
					"Ingress backend sends traffic to the target Service",
					"Ingress", ing.Name, ns, "RoutesTo")
			}
		}
	}

	vsRoutes, meshWalked := a.attachVirtualServices(ctx, ns, []string{name}, out)
	if meshWalked {
		clearDegraded(out, "mesh")
	}

	summary := fmt.Sprintf(
		"Impact for Service/%s in %s: %d static consumer(s), %d backend Deployment(s), %d Ingress route(s)",
		name, ns, consumers, backends, routes,
	)
	if meshWalked {
		summary += fmt.Sprintf(", %d VirtualService route(s)", vsRoutes)
	}
	out.Summary = summary
	out.Confidence = staticConfidence(consumers, routes+vsRoutes)
	if len(out.Findings) == 0 {
		addFinding(out, "Impact.NoneFound", incident.SeverityInfo,
			"No static consumers found",
			"No Deployment env/command/args, Ingress backend, or VirtualService destination referenced this Service; runtime callers need OTel",
			"Service", name, ns, "Target")
	}
	return nil
}

func (a *Analyzer) deploymentImpact(ctx context.Context, ns, name string, out *incident.Investigation) error {
	dep, err := a.Client.AppsV1().Deployments(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	svcs, err := a.Client.CoreV1().Services(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("list services: %w", err)
	}
	deps, err := a.Client.AppsV1().Deployments(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("list deployments: %w", err)
	}

	consumerNames := map[string]struct{}{}
	selectedServices := make([]string, 0)
	for i := range svcs.Items {
		svc := &svcs.Items[i]
		if len(svc.Spec.Selector) == 0 ||
			!labels.SelectorFromSet(svc.Spec.Selector).Matches(labels.Set(dep.Spec.Template.Labels)) {
			continue
		}
		selectedServices = append(selectedServices, svc.Name)
		addFinding(out, "Impact.Service", incident.SeverityInfo,
			fmt.Sprintf("Service/%s selects Deployment/%s", svc.Name, name),
			"Service selector matches the Deployment pod template",
			"Service", svc.Name, ns, "Selects")
		for j := range deps.Items {
			candidate := &deps.Items[j]
			if detail := deploymentReference(candidate.Spec.Template.Spec.Containers, svc.Name, ns); detail != "" {
				consumerNames[candidate.Name] = struct{}{}
				addFinding(out, "Impact.Consumer", incident.SeverityMedium,
					fmt.Sprintf("Deployment/%s consumes Service/%s", candidate.Name, svc.Name),
					detail, "Deployment", candidate.Name, ns, "Consumer")
			}
		}
	}

	hpas, err := a.Client.AutoscalingV2().HorizontalPodAutoscalers(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		out.Degraded = appendUnique(out.Degraded, "hpa")
		hpas = nil
	}
	hpaCount := 0
	if hpas != nil {
		for i := range hpas.Items {
			hpa := &hpas.Items[i]
			if strings.EqualFold(hpa.Spec.ScaleTargetRef.Kind, "Deployment") &&
				hpa.Spec.ScaleTargetRef.Name == name {
				hpaCount++
				addFinding(out, "Impact.HPA", incident.SeverityInfo,
					fmt.Sprintf("HorizontalPodAutoscaler/%s scales Deployment/%s", hpa.Name, name),
					fmt.Sprintf("current=%d desired=%d max=%d",
						hpa.Status.CurrentReplicas, hpa.Status.DesiredReplicas, hpa.Spec.MaxReplicas),
					"HorizontalPodAutoscaler", hpa.Name, ns, "Scales")
			}
		}
	}

	pdbs, err := a.Client.PolicyV1().PodDisruptionBudgets(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		out.Degraded = appendUnique(out.Degraded, "pdb")
		pdbs = nil
	}
	pdbCount := 0
	if pdbs != nil {
		for i := range pdbs.Items {
			pdb := &pdbs.Items[i]
			if pdb.Spec.Selector == nil {
				continue
			}
			sel, err := metav1.LabelSelectorAsSelector(pdb.Spec.Selector)
			if err != nil || !sel.Matches(labels.Set(dep.Spec.Template.Labels)) {
				continue
			}
			pdbCount++
			addFinding(out, "Impact.PDB", incident.SeverityInfo,
				fmt.Sprintf("PodDisruptionBudget/%s protects Deployment/%s", pdb.Name, name),
				fmt.Sprintf("disruptionsAllowed=%d currentHealthy=%d desiredHealthy=%d",
					pdb.Status.DisruptionsAllowed, pdb.Status.CurrentHealthy, pdb.Status.DesiredHealthy),
				"PodDisruptionBudget", pdb.Name, ns, "Protects")
		}
	}

	vsRoutes, meshWalked := a.attachVirtualServices(ctx, ns, selectedServices, out)
	if meshWalked {
		clearDegraded(out, "mesh")
	}

	summary := fmt.Sprintf(
		"Impact for Deployment/%s in %s: %d consumer Deployment(s) via %d Service(s), %d HPA(s), %d PDB(s)",
		name, ns, len(consumerNames), len(selectedServices), hpaCount, pdbCount,
	)
	if meshWalked {
		summary += fmt.Sprintf(", %d VirtualService route(s)", vsRoutes)
	}
	out.Summary = summary
	out.Confidence = staticConfidence(len(consumerNames), len(selectedServices)+vsRoutes)
	if len(out.Findings) == 0 {
		addFinding(out, "Impact.NoneFound", incident.SeverityInfo,
			"No static consumers found",
			"No Service, Deployment reference, HPA, PDB, or VirtualService relationship was found; runtime callers need OTel",
			"Deployment", name, ns, "Target")
	}
	return nil
}

func addFinding(
	out *incident.Investigation,
	code, severity, title, message, kind, name, namespace, reason string,
) {
	ref := incident.ResourceRef{Kind: kind, Name: name, Namespace: namespace}
	ev := incident.EvidenceRef{
		Type:     incident.EvidenceObject,
		Resource: &ref,
		Reason:   reason,
		Message:  message,
		Source:   "kubernetes",
	}
	out.Findings = append(out.Findings, incident.Finding{
		Code:      code,
		Severity:  severity,
		Title:     title,
		Message:   message,
		Namespace: namespace,
		Evidence:  []incident.EvidenceRef{ev},
	})
	out.Evidence = append(out.Evidence, ev)
}

func deploymentReference(
	containers []corev1.Container,
	serviceName, namespace string,
) string {
	for _, c := range containers {
		for _, env := range c.Env {
			if referencesService(env.Value, serviceName, namespace) {
				return fmt.Sprintf("container %q env %s references Service/%s", c.Name, env.Name, serviceName)
			}
			if envNameReferencesService(env.Name, serviceName) {
				return fmt.Sprintf("container %q env name %s indicates Service/%s", c.Name, env.Name, serviceName)
			}
		}
		for _, field := range append(append([]string(nil), c.Command...), c.Args...) {
			if referencesService(field, serviceName, namespace) {
				return fmt.Sprintf("container %q command/args reference Service/%s", c.Name, serviceName)
			}
		}
	}
	return ""
}

func referencesService(value, serviceName, namespace string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	serviceName = strings.ToLower(strings.TrimSpace(serviceName))
	namespace = strings.ToLower(strings.TrimSpace(namespace))
	if value == "" || serviceName == "" {
		return false
	}
	candidates := map[string]struct{}{
		serviceName:                                          {},
		serviceName + "." + namespace:                        {},
		serviceName + "." + namespace + ".svc":               {},
		serviceName + "." + namespace + ".svc.cluster.local": {},
	}
	matches := func(host string) bool {
		host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
		_, ok := candidates[host]
		return ok
	}
	if matches(value) {
		return true
	}
	if parsed, err := url.Parse(value); err == nil && parsed.Host != "" {
		return matches(parsed.Hostname())
	}
	for _, field := range strings.Fields(value) {
		field = strings.Trim(field, `"'(),[]`)
		if i := strings.LastIndex(field, "="); i >= 0 {
			field = field[i+1:]
		}
		if strings.HasPrefix(field, "/") {
			continue // a URL path is not a service reference
		}
		if i := strings.IndexByte(field, '/'); i >= 0 {
			field = field[:i]
		}
		if host, _, ok := strings.Cut(field, ":"); ok {
			field = host
		}
		if matches(field) {
			return true
		}
	}
	return false
}

func envNameReferencesService(envName, serviceName string) bool {
	envName = strings.ToUpper(strings.TrimSpace(envName))
	serviceName = strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(serviceName), "-", "_"))
	if envName == "" || serviceName == "" {
		return false
	}
	for _, suffix := range []string{"_HOST", "_URL", "_URI", "_ADDR", "_ADDRESS", "_SERVICE"} {
		if envName == serviceName+suffix {
			return true
		}
	}
	return false
}

func ingressRoutesTo(ing *networkingv1.Ingress, serviceName string) bool {
	if ing == nil {
		return false
	}
	if ing.Spec.DefaultBackend != nil && ing.Spec.DefaultBackend.Service != nil &&
		ing.Spec.DefaultBackend.Service.Name == serviceName {
		return true
	}
	for _, rule := range ing.Spec.Rules {
		if rule.HTTP == nil {
			continue
		}
		for _, path := range rule.HTTP.Paths {
			if path.Backend.Service != nil && path.Backend.Service.Name == serviceName {
				return true
			}
		}
	}
	return false
}

func staticConfidence(consumers, relationships int) float64 {
	if consumers > 0 {
		return 0.8
	}
	if relationships > 0 {
		return 0.7
	}
	return 0.55
}

func sortInvestigation(out *incident.Investigation) {
	if out == nil {
		return
	}
	sort.SliceStable(out.Findings, func(i, j int) bool {
		if out.Findings[i].Code != out.Findings[j].Code {
			return out.Findings[i].Code < out.Findings[j].Code
		}
		return out.Findings[i].Title < out.Findings[j].Title
	})
	sort.SliceStable(out.Evidence, func(i, j int) bool {
		ri, rj := out.Evidence[i].Resource, out.Evidence[j].Resource
		if ri == nil || rj == nil {
			return ri != nil
		}
		if ri.Kind != rj.Kind {
			return ri.Kind < rj.Kind
		}
		return ri.Name < rj.Name
	})
}

func appendUnique(in []string, value string) []string {
	for _, existing := range in {
		if existing == value {
			return in
		}
	}
	return append(in, value)
}
