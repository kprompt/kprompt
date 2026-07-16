package prometheus

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestQueryVector(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/prom/api/v1/query" {
			t.Fatalf("path=%q", r.URL.Path)
		}
		if got := r.URL.Query().Get("query"); got != `up{job="api"}` {
			t.Fatalf("query=%q", got)
		}
		if r.URL.Query().Get("time") == "" {
			t.Fatal("expected time parameter")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"status":"success",
			"data":{
				"resultType":"vector",
				"result":[{"metric":{"__name__":"up","job":"api"},"value":[1720000000.5,"1"]}]
			},
			"warnings":["partial data"]
		}`))
	}))
	defer server.Close()

	client, err := New(server.URL + "/prom")
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.Query(
		context.Background(),
		`up{job="api"}`,
		time.Unix(1720000000, 500000000),
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Type != "vector" || len(result.Series) != 1 {
		t.Fatalf("result=%+v", result)
	}
	if result.Series[0].Metric["job"] != "api" || result.Series[0].Samples[0].Value != "1" {
		t.Fatalf("series=%+v", result.Series[0])
	}
	if len(result.Warnings) != 1 {
		t.Fatalf("warnings=%v", result.Warnings)
	}
}

func TestQueryRangeMatrix(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/query_range" {
			t.Fatalf("path=%q", r.URL.Path)
		}
		if got := r.URL.Query().Get("step"); got != "30" {
			t.Fatalf("step=%q", got)
		}
		_, _ = w.Write([]byte(`{
			"status":"success",
			"data":{
				"resultType":"matrix",
				"result":[{"metric":{"pod":"api-1"},"values":[[1000,"0.1"],[1030,"0.2"]]}]
			}
		}`))
	}))
	defer server.Close()

	client, err := New(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.QueryRange(
		context.Background(),
		"rate(http_requests_total[5m])",
		time.Unix(1000, 0),
		time.Unix(1060, 0),
		30*time.Second,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Series) != 1 || len(result.Series[0].Samples) != 2 {
		t.Fatalf("result=%+v", result)
	}
}

func TestQueryReturnsPrometheusError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{
			"status":"error",
			"errorType":"bad_data",
			"error":"invalid parameter query"
		}`))
	}))
	defer server.Close()

	client, err := New(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Query(context.Background(), "up[", time.Time{})
	if err == nil || !strings.Contains(err.Error(), "bad_data") {
		t.Fatalf("err=%v", err)
	}
}

func TestQueryTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(100 * time.Millisecond)
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[]}}`))
	}))
	defer server.Close()

	client, err := New(server.URL, WithTimeout(10*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Query(context.Background(), "up", time.Time{})
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("err=%v", err)
	}
}

func TestQueryResponseLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[]}}`))
	}))
	defer server.Close()

	client, err := New(server.URL, WithMaxBodyBytes(8))
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Query(context.Background(), "up", time.Time{})
	if !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("err=%v", err)
	}
}

func TestQueryRangeValidation(t *testing.T) {
	client, err := New("https://prom.example")
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.QueryRange(
		context.Background(),
		"up",
		time.Unix(100, 0),
		time.Unix(50, 0),
		time.Minute,
	)
	if err == nil {
		t.Fatal("expected invalid range error")
	}
}

func TestNewRejectsInvalidURL(t *testing.T) {
	for _, raw := range []string{"", "localhost:9090", "ftp://prom.example"} {
		if _, err := New(raw); err == nil {
			t.Fatalf("expected error for %q", raw)
		}
	}
}
