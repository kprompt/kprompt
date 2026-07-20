package team

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestLoginPersistsCredentials(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("KPROMPT_HOME", dir)

	approved := false
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/device/code", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(StartDeviceCodeResult{
			DeviceCode:              "dc_test",
			UserCode:                "ABCD-EFGH",
			VerificationURI:         "https://app.example/connect",
			VerificationURIComplete: "https://app.example/connect?code=ABCD-EFGH",
			ExpiresIn:               60,
			Interval:                1,
		})
	})
	mux.HandleFunc("/v1/device/token", func(w http.ResponseWriter, r *http.Request) {
		if !approved {
			approved = true
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(PollDeviceTokenResult{Status: "pending"})
			return
		}
		_ = json.NewEncoder(w).Encode(PollDeviceTokenResult{
			Status:    "approved",
			APIToken:  "kp_testtoken",
			TokenHint: "kp_test…",
			Org:       &Org{ID: "org_1", Name: "Acme"},
			Member:    &Member{Email: "a@acme.test", Role: "admin"},
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	creds, err := Login(context.Background(), LoginOptions{
		APIURL: srv.URL,
		Sleep:  func(d time.Duration) {},
	})
	if err != nil {
		t.Fatal(err)
	}
	if creds.APIToken != "kp_testtoken" || creds.OrgName != "Acme" {
		t.Fatalf("bad creds: %+v", creds)
	}
	loaded, ok, err := LoadCredentials()
	if err != nil || !ok {
		t.Fatalf("load: ok=%v err=%v", ok, err)
	}
	if loaded.APIToken != "kp_testtoken" {
		t.Fatalf("persisted token missing: %+v", loaded)
	}
}

func TestResolvePrefersEnv(t *testing.T) {
	t.Setenv(EnvAPIURL, "https://custom.example")
	t.Setenv(EnvAPIToken, "kp_env")
	creds := Credentials{APIURL: "https://file.example", APIToken: "kp_file"}
	if got := ResolveAPIURL(creds); got != "https://custom.example" {
		t.Fatalf("url=%s", got)
	}
	if got := ResolveToken(creds); got != "kp_env" {
		t.Fatalf("token=%s", got)
	}
}
