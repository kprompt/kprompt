package otel

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSearchJaegerTraces(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/traces" {
			t.Fatalf("path=%q", r.URL.Path)
		}
		if r.URL.Query().Get("service") != "payments" {
			t.Fatalf("service=%q", r.URL.Query().Get("service"))
		}
		if r.URL.Query().Get("operation") != "POST /charge" {
			t.Fatalf("operation=%q", r.URL.Query().Get("operation"))
		}
		_, _ = w.Write([]byte(`{
			"data":[{
				"traceID":"trace-1",
				"spans":[
					{
						"traceID":"trace-1",
						"spanID":"root",
						"operationName":"POST /charge",
						"startTime":1700000000000000,
						"duration":120000,
						"processID":"p1",
						"tags":[{"key":"otel.status_code","type":"string","value":"OK"}]
					},
					{
						"traceID":"trace-1",
						"spanID":"db",
						"operationName":"INSERT payment",
						"references":[{"refType":"CHILD_OF","spanID":"root"}],
						"startTime":1700000000020000,
						"duration":50000,
						"processID":"p2",
						"tags":[]
					}
				],
				"processes":{
					"p1":{"serviceName":"payments","tags":[]},
					"p2":{"serviceName":"postgres","tags":[]}
				}
			}]
		}`))
	}))
	defer server.Close()

	client, err := New(server.URL, WithBackend(BackendJaeger))
	if err != nil {
		t.Fatal(err)
	}
	traces, err := client.SearchTraces(context.Background(), SearchRequest{
		Service:   "payments",
		Operation: "POST /charge",
		Start:     time.Unix(1699999900, 0),
		End:       time.Unix(1700000100, 0),
		Limit:     5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(traces) != 1 {
		t.Fatalf("traces=%d", len(traces))
	}
	trace := traces[0]
	if trace.RootService != "payments" || trace.RootOperation != "POST /charge" {
		t.Fatalf("trace=%+v", trace)
	}
	if len(trace.Spans) != 2 || trace.Spans[1].ParentSpanID != "root" {
		t.Fatalf("spans=%+v", trace.Spans)
	}
}

func TestSearchTempoTraces(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/search":
			query := r.URL.Query().Get("q")
			if !strings.Contains(query, `resource.service.name = "payments"`) ||
				!strings.Contains(query, `name = "POST /charge"`) {
				t.Fatalf("traceQL=%q", query)
			}
			_, _ = w.Write([]byte(`{
				"traces":[{
					"traceID":"abc123",
					"rootServiceName":"payments",
					"rootTraceName":"POST /charge",
					"startTimeUnixNano":"1700000000000000000",
					"durationMs":125
				}]
			}`))
		case "/api/traces/abc123":
			_, _ = w.Write([]byte(`{
				"resourceSpans":[{
					"resource":{"attributes":[
						{"key":"service.name","value":{"stringValue":"payments"}}
					]},
					"scopeSpans":[{
						"spans":[{
							"traceId":"abc123",
							"spanId":"span1",
							"name":"POST /charge",
							"startTimeUnixNano":"1700000000000000000",
							"endTimeUnixNano":"1700000000125000000",
							"status":{"code":"STATUS_CODE_OK"},
							"attributes":[
								{"key":"http.response.status_code","value":{"intValue":"200"}}
							]
						}]
					}]
				}]
			}`))
		default:
			t.Fatalf("path=%q", r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := New(server.URL, WithBackend(BackendTempo))
	if err != nil {
		t.Fatal(err)
	}
	traces, err := client.SearchTraces(context.Background(), SearchRequest{
		Service:   "payments",
		Operation: "POST /charge",
		Start:     time.Unix(1699999900, 0),
		End:       time.Unix(1700000100, 0),
		Limit:     5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(traces) != 1 || len(traces[0].Spans) != 1 {
		t.Fatalf("traces=%+v", traces)
	}
	if traces[0].Spans[0].Status != "OK" {
		t.Fatalf("span=%+v", traces[0].Spans[0])
	}
	if traces[0].Duration != 125*time.Millisecond {
		t.Fatalf("duration=%s", traces[0].Duration)
	}
}

func TestAutoFallsBackToTempo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/traces":
			http.NotFound(w, r)
		case "/api/search":
			_, _ = w.Write([]byte(`{"traces":[]}`))
		default:
			t.Fatalf("path=%q", r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := New(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	traces, err := client.SearchTraces(context.Background(), SearchRequest{
		Service: "api",
		Start:   time.Now().Add(-time.Hour),
		End:     time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(traces) != 0 {
		t.Fatalf("traces=%+v", traces)
	}
}

func TestTraceBackendTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(100 * time.Millisecond)
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer server.Close()

	client, err := New(
		server.URL,
		WithBackend(BackendJaeger),
		WithTimeout(10*time.Millisecond),
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.SearchTraces(context.Background(), SearchRequest{
		Service: "api",
		Start:   time.Now().Add(-time.Hour),
		End:     time.Now(),
	})
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("err=%v", err)
	}
}

func TestSearchValidation(t *testing.T) {
	client, err := New("https://traces.example")
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.SearchTraces(context.Background(), SearchRequest{
		Service: "api",
		Start:   time.Now().Add(-48 * time.Hour),
		End:     time.Now(),
	})
	if err == nil {
		t.Fatal("expected range error")
	}
}
