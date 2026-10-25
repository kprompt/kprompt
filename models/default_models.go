package models

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed default_model_v2.json
var defaultModelJSON []byte

// DefaultModel represents the refreshed default model structure.
type DefaultModel struct {
	Version string            `json:"version"`
	Params  map[string]string `json:"params"`
}

// LoadDefaultModel loads the refreshed default model from embedded JSON.
func LoadDefaultModel() (*DefaultModel, error) {
	var model DefaultModel
	if err := json.Unmarshal(defaultModelJSON, &model); err != nil {
		return nil, fmt.Errorf("failed to unmarshal default model: %w", err)
	}
	return &model, nil
}
