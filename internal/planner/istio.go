package planner

import (
	"fmt"
	"strings"

	"github.com/kprompt/kprompt/internal/intent"
	"github.com/kprompt/kprompt/internal/tools/istio"
)

func buildIstio(in intent.Intent) (ExecutionPlan, error) {
	ns := strings.TrimSpace(in.Target.Namespace)
	name := strings.TrimSpace(in.Target.Name)
	if strings.TrimSpace(in.Target.Kind) == "" {
		in.Target.Kind = "VirtualService"
	}

	summary := "Istio VirtualService traffic summary"
	switch {
	case name != "" && ns != "":
		summary = fmt.Sprintf("Istio traffic for VirtualService/%s -n %s", name, ns)
	case ns != "":
		summary = fmt.Sprintf("Istio VirtualService traffic in namespace %s", ns)
	default:
		summary = "Istio VirtualService traffic (cluster-wide)"
	}

	return ExecutionPlan{
		Intent: in,
		Actions: []Action{{
			Op:      OpIstioTraffic,
			Backend: "istio",
			Object: ObjectRef{
				APIVersion: istio.VirtualServiceGroup + "/v1beta1",
				Kind:       istio.VirtualServiceKind,
				Name:       name,
				Namespace:  ns,
			},
			Diff: "list VirtualServices and narrate hosts, routes, and canary weights",
		}},
		Summary:          summary,
		RequiresApproval: false,
	}, nil
}
