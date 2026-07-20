package team

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/kprompt/kprompt/internal/config"
)

// Policy is the org control-plane policy document (GET /v1/policy).
type Policy struct {
	OrgID           string    `json:"org_id" yaml:"org_id"`
	Version         int       `json:"version" yaml:"version"`
	UpdatedAt       time.Time `json:"updated_at" yaml:"updated_at"`
	MaxRisk         string    `json:"max_risk" yaml:"max_risk"`
	DenyIntents     []string  `json:"deny_intents" yaml:"deny_intents"`
	AllowNamespaces []string  `json:"allow_namespaces" yaml:"allow_namespaces"`
	DenyNamespaces  []string  `json:"deny_namespaces" yaml:"deny_namespaces"`
	RequireApprove  bool      `json:"require_approve" yaml:"require_approve"`
}

// Policy fetches the effective org policy for the authenticated token.
func (c *Client) Policy(ctx context.Context) (Policy, error) {
	var out Policy
	err := c.doJSON(ctx, "GET", "/v1/policy", nil, c.Token, &out)
	return out, err
}

// PolicyPath returns ~/.kprompt/policy.yaml (cached copy; not a secret).
func PolicyPath() (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "policy.yaml"), nil
}

// SavePolicy caches the org policy locally.
func SavePolicy(p Policy) error {
	path, err := PolicyPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(p)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// LoadPolicy reads the cached policy. Missing file → ok=false.
func LoadPolicy() (Policy, bool, error) {
	path, err := PolicyPath()
	if err != nil {
		return Policy{}, false, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Policy{}, false, nil
		}
		return Policy{}, false, err
	}
	var p Policy
	if err := yaml.Unmarshal(data, &p); err != nil {
		return Policy{}, false, fmt.Errorf("parse policy cache: %w", err)
	}
	return p, true, nil
}

// ClearPolicy removes the cached policy file.
func ClearPolicy() error {
	path, err := PolicyPath()
	if err != nil {
		return err
	}
	err = os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// PullPolicy fetches and caches org policy using current credentials.
func PullPolicy(ctx context.Context) (Policy, error) {
	creds, _, err := LoadCredentials()
	if err != nil {
		return Policy{}, err
	}
	token := ResolveToken(creds)
	if token == "" {
		return Policy{}, fmt.Errorf("not enrolled — run: kprompt login")
	}
	client := NewClient(ResolveAPIURL(creds), token)
	pol, err := client.Policy(ctx)
	if err != nil {
		return Policy{}, err
	}
	if err := SavePolicy(pol); err != nil {
		return Policy{}, err
	}
	return pol, nil
}
