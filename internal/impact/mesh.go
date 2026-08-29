package impact

import (
	"context"
	"fmt"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/kprompt/kprompt/internal/incident"
	toolistio "github.com/kprompt/kprompt/internal/tools/istio"
)

// attachVirtualServices adds Impact.VirtualService findings for VS destinations
// matching svcNames. walked=true when the mesh API was queried (or CRD absent).
func (a *Analyzer) attachVirtualServices(ctx context.Context, ns string, svcNames []string, out *incident.Investigation) (routes int, walked bool) {
	if a == nil || a.Dynamic == nil {
		return 0, false
	}
	if len(svcNames) == 0 {
		return 0, true
	}

	list, err := a.Dynamic.Resource(toolistio.VirtualServiceGVR).Namespace(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) || toolistio.IsNoMatchGVR(err) {
			return 0, true
		}
		return 0, false
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
		routes++
		titleSvc := matched[0]
		if len(matched) > 1 {
			titleSvc = strings.Join(matched, ",")
		}
		addFinding(out, "Impact.VirtualService", incident.SeverityInfo,
			fmt.Sprintf("VirtualService/%s routes to Service/%s", vs.GetName(), titleSvc),
			detail,
			"VirtualService", vs.GetName(), ns, "RoutesTo")
		if n := len(out.Evidence); n > 0 {
			last := &out.Evidence[n-1]
			last.Source = "istio"
			if last.Resource != nil {
				last.Resource.APIVersion = toolistio.VirtualServiceGroup + "/v1beta1"
			}
		}
	}
	return routes, walked
}

func clearDegraded(out *incident.Investigation, value string) {
	if out == nil {
		return
	}
	next := out.Degraded[:0]
	for _, d := range out.Degraded {
		if d != value {
			next = append(next, d)
		}
	}
	out.Degraded = next
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
