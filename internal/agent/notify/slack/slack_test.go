package slack

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kprompt/kprompt/internal/incident"
)

func TestFormatAlert(t *testing.T) {
	a := incident.NewAgentAlert(incident.Incident{
		ID:             "inc-1",
		Namespace:      "payments",
		Severity:       incident.SeverityCritical,
		Confidence:     0.94,
		Summary:        "CrashLoopBackOff",
		RootCause:      "Redis DNS timeout",
		Recommendation: "Check redis-service",
		Affected:       []incident.ResourceRef{{Kind: "Deployment", Name: "payment-api"}},
	}, incident.AlertFired, time.Now().UTC())
	a.ProposalID = "ap-1"
	a.ProposalAction = "restartDeployment"
	a.ProposalRisk = "medium"
	a.ProposalHint = "kprompt agent proposals apply -n payments --id ap-1 --approve"
	a.ProposalImpact = "Traffic returns to the last successful ReplicaSet"
	text := FormatAlert(a)
	for _, want := range []string{
		"payments", "CrashLoopBackOff", "94%", "Redis DNS", "payment-api", "inc-1", "ap-1",
		"proposals apply", "Approve (CLI)", "Approve (Slack ask)", "approve ap-1", "Impact:", "medium",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in:\n%s", want, text)
		}
	}
}

func TestNotifyBotThreaded(t *testing.T) {
	var posts []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var m map[string]any
		_ = json.Unmarshal(body, &m)
		posts = append(posts, m)
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			t.Errorf("missing bearer")
		}
		ts := "111.222"
		if v, ok := m["thread_ts"].(string); ok && v != "" {
			ts = "333.444"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true, "ts": ts, "channel": "C1",
		})
	}))
	defer srv.Close()

	c := New(Config{
		BotToken:   "xoxb-test",
		Channel:    "C1",
		APIURL:     srv.URL,
		HTTPClient: srv.Client(),
	})
	alert := sampleAlert()
	res, err := c.Notify(context.Background(), alert, "")
	if err != nil {
		t.Fatal(err)
	}
	if res.ThreadTS != "111.222" || res.Mode != "bot" {
		t.Fatalf("%+v", res)
	}
	res2, err := c.Notify(context.Background(), alert, res.ThreadTS)
	if err != nil {
		t.Fatal(err)
	}
	if res2.ThreadTS != "111.222" {
		t.Fatalf("thread root should stay %q got %q", "111.222", res2.ThreadTS)
	}
	if len(posts) != 2 {
		t.Fatalf("posts=%d", len(posts))
	}
	if posts[1]["thread_ts"] != "111.222" {
		t.Fatalf("expected reply in thread: %+v", posts[1])
	}
}

func TestNotifyWebhook(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got = string(body)
		w.WriteHeader(200)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	c := New(Config{WebhookURL: srv.URL, HTTPClient: srv.Client()})
	_, err := c.Notify(context.Background(), sampleAlert(), "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "CrashLoopBackOff") {
		t.Fatalf("body=%s", got)
	}
}

func TestConfigEnabled(t *testing.T) {
	if (Config{}).Enabled() {
		t.Fatal("empty")
	}
	if !(Config{WebhookURL: "https://hooks.slack.com/x"}).Enabled() {
		t.Fatal("webhook")
	}
	if !(Config{BotToken: "x", Channel: "C"}).Threaded() {
		t.Fatal("threaded")
	}
}

func sampleAlert() incident.AgentAlert {
	return incident.NewAgentAlert(incident.Incident{
		ID:         "inc-42",
		Namespace:  "payments",
		Severity:   incident.SeverityHigh,
		Confidence: 0.9,
		Summary:    "CrashLoopBackOff",
		RootCause:  "x",
	}, incident.AlertFired, time.Now().UTC())
}
