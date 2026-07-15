package intent

import (
	"encoding/json"
	"testing"
)

func TestParseStructuredScale(t *testing.T) {
	raw := []byte(`{"kind":"scale","target":{"name":"api","namespace":"prod","kind":"Deployment"},"params":{"replicas":10},"confidence":0.9}`)
	in, err := ParseStructured(raw)
	if err != nil {
		t.Fatal(err)
	}
	if in.Kind != KindScale {
		t.Fatalf("kind=%s", in.Kind)
	}
	rep, ok := in.Replicas()
	if !ok || rep != 10 {
		t.Fatalf("replicas=%v ok=%v", rep, ok)
	}
}

func TestSchemaIsValidJSON(t *testing.T) {
	if !json.Valid([]byte(SchemaJSON)) {
		t.Fatal("SchemaJSON invalid")
	}
}
