package team

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/kprompt/kprompt/internal/config"
)

const (
	EnvAPIURL   = "KPROMPT_API_URL"
	EnvAPIToken = "KPROMPT_API_TOKEN"
)

// Credentials holds the enrolled Team API token (never written to config.yaml).
type Credentials struct {
	APIURL      string `yaml:"api_url"`
	APIToken    string `yaml:"api_token"`
	OrgID       string `yaml:"org_id,omitempty"`
	OrgName     string `yaml:"org_name,omitempty"`
	MemberEmail string `yaml:"member_email,omitempty"`
	TokenHint   string `yaml:"token_hint,omitempty"`
}

// CredentialsPath returns ~/.kprompt/credentials.yaml (mode 0600 when written).
func CredentialsPath() (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "credentials.yaml"), nil
}

// LoadCredentials reads the credentials file. Missing file returns empty ok=false.
func LoadCredentials() (Credentials, bool, error) {
	path, err := CredentialsPath()
	if err != nil {
		return Credentials{}, false, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Credentials{}, false, nil
		}
		return Credentials{}, false, err
	}
	var c Credentials
	if err := yaml.Unmarshal(data, &c); err != nil {
		return Credentials{}, false, fmt.Errorf("parse credentials: %w", err)
	}
	return c, true, nil
}

// SaveCredentials writes credentials with 0600 permissions.
func SaveCredentials(c Credentials) error {
	path, err := CredentialsPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// ClearCredentials removes the local credentials file.
func ClearCredentials() error {
	path, err := CredentialsPath()
	if err != nil {
		return err
	}
	err = os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// ResolveAPIURL returns effective API base URL (env > credentials > default).
func ResolveAPIURL(creds Credentials) string {
	if v := strings.TrimSpace(os.Getenv(EnvAPIURL)); v != "" {
		return strings.TrimRight(v, "/")
	}
	if strings.TrimSpace(creds.APIURL) != "" {
		return strings.TrimRight(creds.APIURL, "/")
	}
	return DefaultAPIURL
}

// ResolveToken returns effective API token (env > credentials).
func ResolveToken(creds Credentials) string {
	if v := strings.TrimSpace(os.Getenv(EnvAPIToken)); v != "" {
		return v
	}
	return strings.TrimSpace(creds.APIToken)
}
