package discord

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kprompt/kprompt/internal/incident"
)

func TestFormatAlert(t *testing.T) {
	text := FormatAlert(sampleAlert())
	for _, want := range []string{"payments", "fired", "CrashLoopBackOff", "90%", "Redis DNS", "payment-api", "inc-42"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in:\n%s", want, text)
		}
	}
}

func TestNotifySuccess(t *testing.T) {
	var got webhookPayload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method=%s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("content-type %s", r.Header.Get("Content-Type"))
		}
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &got); err != nil {
			t.Errorf("decode: %v", err)
		}
		w.WriteHeader(204)
	}))
	defer srv.Close()

	c := New(Config{URL: srv.URL, HTTPClient: srv.Client()})
	if err := c.Notify(context.Background(), sampleAlert()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.Content, "CrashLoopBackOff") || !strings.Contains(got.Content, "inc-42") {
		t.Fatalf("content=%q", got.Content)
	}
	if len(got.AllowedMentions.Parse) != 0 {
		t.Fatalf("allowed mentions should be disabled: %+v", got.AllowedMentions)
	}
}

func TestNotifyRequiresWebhookURL(t *testing.T) {
	err := New(Config{}).Notify(context.Background(), sampleAlert())
	if err == nil || !strings.Contains(err.Error(), EnvWebhookURL) {
		t.Fatalf("expected %s error, got %v", EnvWebhookURL, err)
	}
}

func TestNotifyRetriesThenSucceeds(t *testing.T) {
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if n.Add(1) < 3 {
			w.WriteHeader(503)
			_, _ = w.Write([]byte("busy"))
			return
		}
		w.WriteHeader(204)
	}))
	defer srv.Close()

	c := New(Config{
		URL:        srv.URL,
		Attempts:   3,
		Backoff:    10 * time.Millisecond,
		HTTPClient: srv.Client(),
	})
	if err := c.Notify(context.Background(), sampleAlert()); err != nil {
		t.Fatal(err)
	}
	if n.Load() != 3 {
		t.Fatalf("attempts=%d", n.Load())
	}
}

func TestNotifyNoRetryOn400(t *testing.T) {
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		w.WriteHeader(400)
		_, _ = w.Write([]byte("bad request"))
	}))
	defer srv.Close()

	c := New(Config{
		URL:        srv.URL,
		Attempts:   3,
		Backoff:    time.Millisecond,
		HTTPClient: srv.Client(),
	})
	err := c.Notify(context.Background(), sampleAlert())
	if err == nil || !strings.Contains(err.Error(), "webhook HTTP 400") {
		t.Fatalf("expected webhook HTTP 400 error, got %v", err)
	}
	if strings.Contains(err.Error(), srv.URL) {
		t.Fatalf("error leaked webhook URL: %v", err)
	}
	if n.Load() != 1 {
		t.Fatalf("expected single attempt, got %d", n.Load())
	}
}

func TestConfigEnabled(t *testing.T) {
	if (Config{}).Enabled() {
		t.Fatal("empty")
	}
	if !(Config{URL: "https://discord.com/api/webhooks/x/y"}).Enabled() {
		t.Fatal("webhook")
	}
}

func sampleAlert() incident.AgentAlert {
	return incident.NewAgentAlert(incident.Incident{
		ID:             "inc-42",
		Namespace:      "payments",
		Severity:       incident.SeverityHigh,
		Confidence:     0.9,
		Summary:        "CrashLoopBackOff",
		RootCause:      "Redis DNS timeout",
		Recommendation: "Check redis-service",
		Affected:       []incident.ResourceRef{{Kind: "Deployment", Name: "payment-api"}},
	}, incident.AlertFired, time.Now().UTC())
}
