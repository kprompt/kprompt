package coordinator

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNormalizeBaseURL(t *testing.T) {
	cases := map[string]string{
		"http://x:9090/v1/handoff":   "http://x:9090",
		"http://x:9090/v1/outcome":   "http://x:9090",
		"http://x:9090/v1/outcomes/": "http://x:9090",
		"http://x:9090/":             "http://x:9090",
		"  http://x:9090  ":          "http://x:9090",
		"":                           "",
	}
	for in, want := range cases {
		if got := NormalizeBaseURL(in); got != want {
			t.Errorf("NormalizeBaseURL(%q)=%q want %q", in, got, want)
		}
	}
}

func TestHTTPClientOutcomes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/outcomes" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"total":3,"durable":true,"byResult":{"apply_success":2}}`))
	}))
	defer srv.Close()

	c := &HTTPClient{BaseURL: srv.URL + "/v1/handoff"} // exercise NormalizeBaseURL path
	sum, err := c.Outcomes(context.Background())
	if err != nil {
		t.Fatalf("Outcomes: %v", err)
	}
	if sum.Total != 3 || !sum.Durable || sum.ByResult["apply_success"] != 2 {
		t.Fatalf("unexpected summary: %+v", sum)
	}
}

func TestHTTPClientOutcomesErrors(t *testing.T) {
	// nil receiver.
	var nilc *HTTPClient
	if _, err := nilc.Outcomes(context.Background()); err == nil {
		t.Fatal("expected nil client error")
	}
	// empty base URL.
	if _, err := (&HTTPClient{}).Outcomes(context.Background()); err == nil {
		t.Fatal("expected BaseURL required error")
	}
	// HTTP error status.
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer bad.Close()
	if _, err := (&HTTPClient{BaseURL: bad.URL}).Outcomes(context.Background()); err == nil {
		t.Fatal("expected HTTP status error")
	}
	// invalid JSON.
	junk := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	defer junk.Close()
	if _, err := (&HTTPClient{BaseURL: junk.URL}).Outcomes(context.Background()); err == nil {
		t.Fatal("expected decode error")
	}
}
