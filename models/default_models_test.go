package models

import (
	"testing"
)

func TestLoadDefaultModel(t *testing.T) {
	model, err := LoadDefaultModel()
	if err != nil {
		t.Fatalf("LoadDefaultModel() error = %v", err)
	}
	if model.Version != "2.0.0" {
		t.Errorf("expected version 2.0.0, got %s", model.Version)
	}
	if val, ok := model.Params["paramA"]; !ok || val != "valueA" {
		t.Errorf("expected paramA to be valueA, got %v", val)
	}
	if val, ok := model.Params["paramB"]; !ok || val != "valueB" {
		t.Errorf("expected paramB to be valueB, got %v", val)
	}
	if val, ok := model.Params["paramC"]; !ok || val != "valueC" {
		t.Errorf("expected paramC to be valueC, got %v", val)
	}
}
