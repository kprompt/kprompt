package llm

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCompleteStructuredJSONObject(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": `{"ok":true}`}},
			},
		})
	}))
	defer srv.Close()

	c := NewOpenAI("k", srv.URL, "m")
	schema := json.RawMessage(`{"type":"object","properties":{"ok":{"type":"boolean"}},"required":["ok"]}`)
	out, err := c.CompleteStructured(t.Context(), CompletionRequest{User: "hi"}, schema)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != `{"ok":true}` {
		t.Fatalf("out = %s", out)
	}
	rf, _ := gotBody["response_format"].(map[string]any)
	if rf["type"] != "json_object" {
		t.Fatalf("response_format = %#v, want json_object", gotBody["response_format"])
	}
}

func TestCompleteStructuredJSONSchema(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": `{"action":"list"}`}},
			},
		})
	}))
	defer srv.Close()

	c := NewOpenAI("k", srv.URL, "m").WithStructuredFormat(StructuredJSONSchema)
	schema := json.RawMessage(`{"type":"object","properties":{"action":{"type":"string"}},"required":["action"]}`)
	if _, err := c.CompleteStructured(t.Context(), CompletionRequest{User: "list pods"}, schema); err != nil {
		t.Fatal(err)
	}
	rf, _ := gotBody["response_format"].(map[string]any)
	if rf["type"] != "json_schema" {
		t.Fatalf("type = %#v", rf["type"])
	}
	js, _ := rf["json_schema"].(map[string]any)
	if js["name"] != "kprompt_response" {
		t.Fatalf("json_schema.name = %#v", js["name"])
	}
	sch, _ := js["schema"].(map[string]any)
	if sch["type"] != "object" {
		t.Fatalf("schema = %#v", sch)
	}
}

func TestNewLMStudioUsesJSONSchema(t *testing.T) {
	p, err := New("lmstudio", "", "", "google/gemma-4-31b")
	if err != nil {
		t.Fatal(err)
	}
	o, ok := p.(*OpenAI)
	if !ok {
		t.Fatalf("type %T", p)
	}
	if o.structuredFormat != StructuredJSONSchema {
		t.Fatalf("structuredFormat = %q, want %q", o.structuredFormat, StructuredJSONSchema)
	}
	if !strings.Contains(o.baseURL, "1234") {
		t.Fatalf("baseURL = %q", o.baseURL)
	}
}
