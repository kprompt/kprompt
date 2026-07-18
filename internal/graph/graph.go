package graph

import (
	"context"
	"fmt"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"

	"github.com/kprompt/kprompt/internal/cluster"
)

const (
	ScopeCluster   = "cluster"
	ScopeNamespace = "namespace"

	NodeService        = "Service"
	NodePod            = "Pod"
	NodeNetworkPolicy  = "NetworkPolicy"
	EdgeRoutes         = "routes"
	EdgeSelects        = "selects"
	SourceKubernetes   = "k8s"
)

// Request configures a read-only service dependency graph.
type Request struct {
	Namespace             string // empty → cluster-wide
	IncludeNetworkPolicy  bool
}

// Node is one graph vertex.
type Node struct {
	ID        string            `json:"id"`
	Kind      string            `json:"kind"`
	Name      string            `json:"name"`
	Namespace string            `json:"namespace,omitempty"`
	Labels    map[string]string `json:"labels,omitempty"`
}

// Edge is one directed dependency.
type Edge struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Type   string `json:"type"` // routes | selects | allows | denies
	Detail string `json:"detail,omitempty"`
	Source string `json:"source"` // k8s | otel (T-060)
}

// Report is the stable human + JSON contract for service dependency graphs (T-059).
type Report struct {
	Type      string   `json:"type"`
	Scope     string   `json:"scope"`
	Namespace string   `json:"namespace,omitempty"`
	Summary   string   `json:"summary"`
	Nodes     []Node   `json:"nodes"`
	Edges     []Edge   `json:"edges"`
	Notes     []string `json:"notes,omitempty"`
}

// Build collects Services + EndpointSlices (+ optional NetworkPolicies) into a graph.
func Build(ctx context.Context, client kubernetes.Interface, req Request) (Report, error) {
	if client == nil {
		return Report{}, fmt.Errorf("kubernetes client is required for service graph")
	}
	ns := strings.TrimSpace(req.Namespace)
	scope := ScopeCluster
	if ns != "" {
		scope = ScopeNamespace
	}
	rep := Report{
		Type:      "service-graph",
		Scope:     scope,
		Namespace: ns,
		Nodes:     make([]Node, 0, 32),
		Edges:     make([]Edge, 0, 64),
	}
	limit := int64(cluster.DefaultReadLimit)
	nodes := map[string]Node{}
	var notes []string

	svcs, err := client.CoreV1().Services(ns).List(ctx, metav1.ListOptions{Limit: limit})
	if err != nil {
		if apierrors.IsForbidden(err) {
			notes = append(notes, fmt.Sprintf("skipped Services: %v", err))
		} else {
			return Report{}, fmt.Errorf("list services: %w", err)
		}
	} else {
		for _, svc := range svcs.Items {
			id := nodeID(NodeService, svc.Namespace, svc.Name)
			nodes[id] = Node{
				ID: id, Kind: NodeService, Name: svc.Name, Namespace: svc.Namespace,
				Labels: copyLabels(svc.Labels),
			}
		}
	}

	slices, err := client.DiscoveryV1().EndpointSlices(ns).List(ctx, metav1.ListOptions{Limit: limit})
	if err != nil {
		if apierrors.IsForbidden(err) {
			notes = append(notes, fmt.Sprintf("skipped EndpointSlices: %v", err))
		} else {
			return Report{}, fmt.Errorf("list endpointslices: %w", err)
		}
	} else {
		for _, slice := range slices.Items {
			svcName := slice.Labels[discoveryv1.LabelServiceName]
			if svcName == "" {
				continue
			}
			svcID := nodeID(NodeService, slice.Namespace, svcName)
			if _, ok := nodes[svcID]; !ok {
				nodes[svcID] = Node{
					ID: svcID, Kind: NodeService, Name: svcName, Namespace: slice.Namespace,
				}
			}
			for _, ep := range slice.Endpoints {
				podName, podNS := endpointPod(ep, slice.Namespace)
				if podName == "" {
					continue
				}
				podID := nodeID(NodePod, podNS, podName)
				if _, ok := nodes[podID]; !ok {
					nodes[podID] = Node{
						ID: podID, Kind: NodePod, Name: podName, Namespace: podNS,
					}
				}
				detail := fmt.Sprintf("EndpointSlice/%s", slice.Name)
				if len(slice.Ports) > 0 && slice.Ports[0].Port != nil {
					detail = fmt.Sprintf("%s port %d", detail, *slice.Ports[0].Port)
				}
				rep.Edges = append(rep.Edges, Edge{
					From: svcID, To: podID, Type: EdgeRoutes, Detail: detail, Source: SourceKubernetes,
				})
			}
		}
	}

	if req.IncludeNetworkPolicy {
		policies, err := client.NetworkingV1().NetworkPolicies(ns).List(ctx, metav1.ListOptions{Limit: limit})
		if err != nil {
			if apierrors.IsForbidden(err) {
				notes = append(notes, fmt.Sprintf("skipped NetworkPolicies: %v", err))
			} else {
				notes = append(notes, fmt.Sprintf("NetworkPolicies unavailable: %v", err))
			}
		} else {
			var svcList []corev1.Service
			if svcs != nil {
				svcList = svcs.Items
			}
			for _, np := range policies.Items {
				npID := nodeID(NodeNetworkPolicy, np.Namespace, np.Name)
				nodes[npID] = Node{
					ID: npID, Kind: NodeNetworkPolicy, Name: np.Name, Namespace: np.Namespace,
					Labels: copyLabels(np.Labels),
				}
				for _, svc := range svcList {
					if svc.Namespace != np.Namespace {
						continue
					}
					if networkPolicySelectsService(np, svc) {
						rep.Edges = append(rep.Edges, Edge{
							From: npID, To: nodeID(NodeService, svc.Namespace, svc.Name),
							Type: EdgeSelects, Detail: "podSelector", Source: SourceKubernetes,
						})
					}
				}
			}
		}
	} else {
		notes = append(notes, "NetworkPolicy edges omitted (pass includeNetworkPolicy to enable)")
	}

	// Deduplicate edges.
	edgeSeen := map[string]struct{}{}
	unique := make([]Edge, 0, len(rep.Edges))
	for _, e := range rep.Edges {
		k := e.From + "|" + e.To + "|" + e.Type + "|" + e.Source
		if _, ok := edgeSeen[k]; ok {
			continue
		}
		edgeSeen[k] = struct{}{}
		unique = append(unique, e)
	}
	rep.Edges = unique

	ids := make([]string, 0, len(nodes))
	for id := range nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		rep.Nodes = append(rep.Nodes, nodes[id])
		if int64(len(rep.Nodes)) >= limit {
			notes = append(notes, fmt.Sprintf("truncated at %d nodes", limit))
			break
		}
	}
	sort.Slice(rep.Edges, func(i, j int) bool {
		if rep.Edges[i].From != rep.Edges[j].From {
			return rep.Edges[i].From < rep.Edges[j].From
		}
		return rep.Edges[i].To < rep.Edges[j].To
	})
	if int64(len(rep.Edges)) > limit {
		rep.Edges = rep.Edges[:limit]
		notes = append(notes, fmt.Sprintf("truncated at %d edges", limit))
	}

	svcN, podN, npN := 0, 0, 0
	for _, n := range rep.Nodes {
		switch n.Kind {
		case NodeService:
			svcN++
		case NodePod:
			podN++
		case NodeNetworkPolicy:
			npN++
		}
	}
	scopeLabel := "cluster"
	if scope == ScopeNamespace {
		scopeLabel = fmt.Sprintf("namespace %q", ns)
	}
	rep.Summary = fmt.Sprintf(
		"Service dependency graph for %s: %d services, %d pods, %d network policies, %d edges (Kubernetes topology; OTel enrichment is T-060).",
		scopeLabel, svcN, podN, npN, len(rep.Edges),
	)
	rep.Notes = notes
	return rep, nil
}

func nodeID(kind, namespace, name string) string {
	if namespace == "" {
		return kind + "/" + name
	}
	return namespace + "/" + kind + "/" + name
}

func copyLabels(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func endpointPod(ep discoveryv1.Endpoint, sliceNS string) (name, namespace string) {
	if ep.TargetRef == nil {
		return "", ""
	}
	if !strings.EqualFold(ep.TargetRef.Kind, "Pod") {
		return "", ""
	}
	name = ep.TargetRef.Name
	namespace = ep.TargetRef.Namespace
	if namespace == "" {
		namespace = sliceNS
	}
	return name, namespace
}

func networkPolicySelectsService(np networkingv1.NetworkPolicy, svc corev1.Service) bool {
	sel, err := metav1.LabelSelectorAsSelector(&np.Spec.PodSelector)
	if err != nil {
		return false
	}
	// Empty selector matches all pods in the namespace → all services.
	if sel.Empty() {
		return true
	}
	if len(svc.Spec.Selector) == 0 {
		return false
	}
	return sel.Matches(labels.Set(svc.Spec.Selector))
}
