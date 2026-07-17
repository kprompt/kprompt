package grafana

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestListDashboards(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/grafana/api/search" {
			t.Fatalf("path=%q", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Fatalf("authorization=%q", r.Header.Get("Authorization"))
		}
		if r.URL.Query().Get("type") != "dash-db" ||
			r.URL.Query().Get("query") != "payments" ||
			r.URL.Query().Get("tag") != "prod" ||
			r.URL.Query().Get("limit") != "25" {
			t.Fatalf("query=%v", r.URL.Query())
		}
		_, _ = w.Write([]byte(`[
			{
				"uid":"payments",
				"title":"Payments Overview",
				"url":"/d/payments/payments-overview",
				"tags":["prod","payments"],
				"type":"dash-db",
				"folderUid":"services",
				"folderTitle":"Services",
				"isStarred":true
			},
			{"uid":"folder","title":"Services","type":"dash-folder"}
		]`))
	}))
	defer server.Close()

	client, err := New(server.URL+"/grafana", "secret")
	if err != nil {
		t.Fatal(err)
	}
	dashboards, err := client.ListDashboards(context.Background(), SearchRequest{
		Query: "payments",
		Tag:   "prod",
		Limit: 25,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(dashboards) != 1 {
		t.Fatalf("dashboards=%+v", dashboards)
	}
	got := dashboards[0]
	if got.UID != "payments" || got.FolderUID != "services" || !got.Starred {
		t.Fatalf("dashboard=%+v", got)
	}
}

func TestGetDashboardPanelMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/dashboards/uid/payments" {
			t.Fatalf("path=%q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{
			"dashboard":{
				"uid":"payments",
				"title":"Payments Overview",
				"tags":["prod"],
				"panels":[
					{
						"id":1,
						"title":"Request rate",
						"type":"timeseries",
						"datasource":{"uid":"prom-main","type":"prometheus"},
						"gridPos":{"x":0,"y":0,"w":12,"h":8},
						"targets":[{"refId":"A","expr":"rate(http_requests_total[5m])"}]
					},
					{
						"id":2,
						"title":"Database",
						"type":"row",
						"panels":[{
							"id":3,
							"title":"Slow queries",
							"type":"table",
							"datasource":"postgres",
							"gridPos":{"x":0,"y":8,"w":24,"h":8},
							"targets":[{
								"refId":"B",
								"query":"select * from slow_queries",
								"datasource":{"uid":"postgres-main"}
							}]
						}]
					}
				]
			},
			"meta":{"url":"/d/payments/payments-overview"}
		}`))
	}))
	defer server.Close()

	client, err := New(server.URL, "")
	if err != nil {
		t.Fatal(err)
	}
	dashboard, err := client.GetDashboard(context.Background(), "payments")
	if err != nil {
		t.Fatal(err)
	}
	if dashboard.Title != "Payments Overview" || len(dashboard.Panels) != 2 {
		t.Fatalf("dashboard=%+v", dashboard)
	}
	if dashboard.Panels[0].Datasource.UID != "prom-main" ||
		dashboard.Panels[0].Targets[0].Expression != "rate(http_requests_total[5m])" {
		t.Fatalf("panel=%+v", dashboard.Panels[0])
	}
	if dashboard.Panels[1].Datasource.Name != "postgres" ||
		dashboard.Panels[1].Targets[0].Datasource != "postgres-main" {
		t.Fatalf("nested panel=%+v", dashboard.Panels[1])
	}
}

func TestGrafanaAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"invalid API key"}`))
	}))
	defer server.Close()

	client, err := New(server.URL, "bad")
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.ListDashboards(context.Background(), SearchRequest{})
	if err == nil || !strings.Contains(err.Error(), "invalid API key") {
		t.Fatalf("err=%v", err)
	}
}

func TestGrafanaTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(100 * time.Millisecond)
		_, _ = w.Write([]byte(`[]`))
	}))
	defer server.Close()

	client, err := New(server.URL, "", WithTimeout(10*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.ListDashboards(context.Background(), SearchRequest{})
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("err=%v", err)
	}
}

func TestGrafanaResponseLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"uid":"payments"}]`))
	}))
	defer server.Close()

	client, err := New(server.URL, "", WithMaxBodyBytes(8))
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.ListDashboards(context.Background(), SearchRequest{})
	if !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("err=%v", err)
	}
}

func TestGrafanaValidation(t *testing.T) {
	for _, raw := range []string{"", "localhost:3000", "ftp://grafana.example"} {
		if _, err := New(raw, ""); err == nil {
			t.Fatalf("expected error for %q", raw)
		}
	}
	client, err := New("https://grafana.example", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.ListDashboards(context.Background(), SearchRequest{Limit: 1001}); err == nil {
		t.Fatal("expected limit error")
	}
	if _, err := client.GetDashboard(context.Background(), "bad/uid"); err == nil {
		t.Fatal("expected UID error")
	}
}
