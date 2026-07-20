package team

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAppendAudit(t *testing.T) {
	var got AuditEventInput
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/audit/events", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer kp_test" {
			t.Fatalf("auth: %s", r.Header.Get("Authorization"))
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"aud_1"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := NewClient(srv.URL, "kp_test")
	err := c.AppendAudit(context.Background(), AuditEventInput{
		Prompt:      "scale api to 4",
		PlanSummary: "Scale Deployment/api",
		Risk:        "medium",
		Decision:    "applied",
		Namespace:   "default",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Decision != "applied" || got.Prompt != "scale api to 4" {
		t.Fatalf("got %+v", got)
	}
}
