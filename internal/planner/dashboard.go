package planner

import (
	"fmt"
	"strings"

	"github.com/kprompt/kprompt/internal/intent"
)

func buildDashboard(in intent.Intent) (ExecutionPlan, error) {
	name := strings.TrimSpace(in.Target.Name)
	uid, hasUID := in.DashboardUID()
	uid = strings.TrimSpace(uid)

	summary := "List Grafana dashboards"
	diff := "query Grafana dashboard search API"
	target := name
	if hasUID {
		target = uid
		summary = fmt.Sprintf("Show Grafana dashboard UID %s", uid)
		diff = fmt.Sprintf("fetch Grafana dashboard UID %s and panel metadata", uid)
	} else if name != "" {
		summary = fmt.Sprintf("Find Grafana dashboard %q", name)
		diff = fmt.Sprintf("search Grafana dashboards for %q and fetch an exact match", name)
	}
	return ExecutionPlan{
		Intent: in,
		Actions: []Action{{
			Op:      OpGrafanaQuery,
			Backend: "grafana",
			Object: ObjectRef{
				Kind: "Dashboard",
				Name: target,
			},
			Diff: diff,
		}},
		Summary:          summary,
		RequiresApproval: false,
	}, nil
}
