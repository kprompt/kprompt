package grafana

import (
	"context"
	"fmt"
	"strings"
)

// ShowDashboard resolves an explicit UID or searches for a dashboard.
func ShowDashboard(
	ctx context.Context,
	querier Querier,
	req ShowRequest,
) (ShowResult, error) {
	if querier == nil {
		return ShowResult{}, fmt.Errorf("Grafana querier is required")
	}
	req.UID = strings.TrimSpace(req.UID)
	req.Query = strings.TrimSpace(req.Query)
	if req.UID != "" {
		dashboard, err := querier.GetDashboard(ctx, req.UID)
		if err != nil {
			return ShowResult{}, err
		}
		return ShowResult{Dashboard: &dashboard}, nil
	}
	if req.Limit == 0 {
		req.Limit = 20
	}
	dashboards, err := querier.ListDashboards(ctx, SearchRequest{
		Query: req.Query,
		Limit: req.Limit,
	})
	if err != nil {
		return ShowResult{}, err
	}

	match := exactDashboard(dashboards, req.Query)
	if match == nil && req.Query != "" && len(dashboards) == 1 {
		match = &dashboards[0]
	}
	if match != nil && match.UID != "" {
		dashboard, err := querier.GetDashboard(ctx, match.UID)
		if err != nil {
			return ShowResult{}, err
		}
		return ShowResult{Query: req.Query, Dashboard: &dashboard}, nil
	}
	return ShowResult{
		Query:      req.Query,
		Dashboards: dashboards,
	}, nil
}

func exactDashboard(
	dashboards []DashboardSummary,
	query string,
) *DashboardSummary {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}
	for index := range dashboards {
		if strings.EqualFold(dashboards[index].UID, query) ||
			strings.EqualFold(dashboards[index].Title, query) {
			return &dashboards[index]
		}
	}
	return nil
}
