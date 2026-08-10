package coordinator

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandlerEndpoints(t *testing.T) {
	h := &Handler{Service: New()}
	srv := httptest.NewServer(h.routes())
	defer srv.Close()

	get := func(path string) *http.Response {
		res, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		return res
	}

	// healthz
	if res := get("/healthz"); res.StatusCode != http.StatusOK {
		t.Fatalf("healthz status=%d", res.StatusCode)
	}

	// record an outcome via POST /v1/outcome
	rec := OutcomeRecord{Namespace: "payments", Action: "restartDeployment", Result: "apply_success"}
	body, _ := json.Marshal(rec)
	res, err := http.Post(srv.URL+"/v1/outcome", "application/json", bytes.NewReader(body))
	if err != nil || res.StatusCode != http.StatusAccepted {
		t.Fatalf("post outcome: err=%v status=%v", err, res)
	}

	// invalid outcome (missing fields) -> 400
	bad, _ := json.Marshal(OutcomeRecord{Namespace: "x"})
	res, _ = http.Post(srv.URL+"/v1/outcome", "application/json", bytes.NewReader(bad))
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad outcome status=%d", res.StatusCode)
	}

	// GET /v1/outcomes returns the summary
	var sum OutcomeSummary
	res = get("/v1/outcomes")
	if err := json.NewDecoder(res.Body).Decode(&sum); err != nil {
		t.Fatalf("decode outcomes: %v", err)
	}
	if sum.Total != 1 {
		t.Fatalf("expected 1 outcome, got %d", sum.Total)
	}

	// GET /v1/recent, /v1/knowledge, /v1/blast-radius all 200
	for _, path := range []string{"/v1/recent", "/v1/knowledge", "/v1/blast-radius", "/v1/blast-radius?namespace=payments"} {
		if res := get(path); res.StatusCode != http.StatusOK {
			t.Fatalf("%s status=%d", path, res.StatusCode)
		}
	}

	// method guards: POST-only and GET-only endpoints reject the wrong verb.
	res, _ = http.Post(srv.URL+"/v1/outcomes", "application/json", nil)
	if res.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("outcomes POST should be 405, got %d", res.StatusCode)
	}
}

func TestLookupActionAndFormat(t *testing.T) {
	s := New()
	for i := 0; i < 3; i++ {
		if err := s.RecordOutcome(OutcomeRecord{Namespace: "payments", Action: "restartDeployment", Result: "apply_success"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.RecordOutcome(OutcomeRecord{Namespace: "payments", Action: "restartDeployment", Result: "apply_failed"}); err != nil {
		t.Fatal(err)
	}
	sum := s.OutcomeSummarize()

	stat, ok := sum.LookupAction("restartDeployment", "payments")
	if !ok || stat.Total != 4 || stat.Success != 3 || stat.Failed != 1 {
		t.Fatalf("ns lookup: %+v ok=%v", stat, ok)
	}
	agg, ok := sum.LookupAction("restartDeployment", "")
	if !ok || agg.Total != 4 {
		t.Fatalf("agg lookup: %+v ok=%v", agg, ok)
	}
	if _, ok := sum.LookupAction("nope", ""); ok {
		t.Fatal("unknown action must not be found")
	}
	if _, ok := sum.LookupAction("", ""); ok {
		t.Fatal("empty action must not be found")
	}

	out := FormatOutcomeSummary(sum)
	if out == "" {
		t.Fatal("FormatOutcomeSummary empty")
	}
}
