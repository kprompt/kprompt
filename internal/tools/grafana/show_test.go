package grafana

import (
	"context"
	"testing"
)

type fakeQuerier struct {
	dashboards []DashboardSummary
	dashboard  Dashboard
	listCalls  int
	getUID     string
}

func (f *fakeQuerier) ListDashboards(
	context.Context,
	SearchRequest,
) ([]DashboardSummary, error) {
	f.listCalls++
	return f.dashboards, nil
}

func (f *fakeQuerier) GetDashboard(
	_ context.Context,
	uid string,
) (Dashboard, error) {
	f.getUID = uid
	return f.dashboard, nil
}

func TestShowDashboardByUID(t *testing.T) {
	querier := &fakeQuerier{
		dashboard: Dashboard{UID: "payments", Title: "Payments"},
	}
	result, err := ShowDashboard(
		context.Background(),
		querier,
		ShowRequest{UID: "payments"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Dashboard == nil || result.Dashboard.UID != "payments" {
		t.Fatalf("result=%+v", result)
	}
	if querier.listCalls != 0 || querier.getUID != "payments" {
		t.Fatalf("calls list=%d get=%q", querier.listCalls, querier.getUID)
	}
}

func TestShowDashboardFetchesSingleSearchMatch(t *testing.T) {
	querier := &fakeQuerier{
		dashboards: []DashboardSummary{{
			UID:   "payments",
			Title: "Payments Overview",
		}},
		dashboard: Dashboard{UID: "payments", Title: "Payments Overview"},
	}
	result, err := ShowDashboard(
		context.Background(),
		querier,
		ShowRequest{Query: "payments"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Dashboard == nil || querier.getUID != "payments" {
		t.Fatalf("result=%+v get=%q", result, querier.getUID)
	}
}

func TestShowDashboardReturnsAmbiguousMatches(t *testing.T) {
	querier := &fakeQuerier{
		dashboards: []DashboardSummary{
			{UID: "payments-prod", Title: "Payments Production"},
			{UID: "payments-dev", Title: "Payments Development"},
		},
	}
	result, err := ShowDashboard(
		context.Background(),
		querier,
		ShowRequest{Query: "payments"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Dashboard != nil || len(result.Dashboards) != 2 {
		t.Fatalf("result=%+v", result)
	}
}
