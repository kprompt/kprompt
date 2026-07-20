package team

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"

	"github.com/kprompt/kprompt/internal/config"
)

// ExportSecretsResponse is GET /v1/org/secrets/export.
type ExportSecretsResponse struct {
	Secrets map[string]string `json:"secrets"`
}

// ExportSecrets fetches decrypted provider keys for the enrolled org.
func (c *Client) ExportSecrets(ctx context.Context) (map[string]string, error) {
	var out ExportSecretsResponse
	err := c.doJSON(ctx, "GET", "/v1/org/secrets/export", nil, c.Token, &out)
	if err != nil {
		return nil, err
	}
	if out.Secrets == nil {
		return map[string]string{}, nil
	}
	return out.Secrets, nil
}

// ProviderSecretsPath returns ~/.kprompt/provider-secrets.yaml (0600).
func ProviderSecretsPath() (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "provider-secrets.yaml"), nil
}

// SaveProviderSecrets writes pulled keys locally (mode 0600). Not for git.
func SaveProviderSecrets(secrets map[string]string) error {
	path, err := ProviderSecretsPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if secrets == nil {
		secrets = map[string]string{}
	}
	data, err := yaml.Marshal(secrets)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// ClearProviderSecrets removes the local pulled secrets file.
func ClearProviderSecrets() error {
	path, err := ProviderSecretsPath()
	if err != nil {
		return err
	}
	err = os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// PullSecrets fetches org provider keys and caches them locally.
func PullSecrets(ctx context.Context) (map[string]string, error) {
	creds, _, err := LoadCredentials()
	if err != nil {
		return nil, err
	}
	token := ResolveToken(creds)
	if token == "" {
		return nil, fmt.Errorf("not enrolled — run: kprompt login")
	}
	client := NewClient(ResolveAPIURL(creds), token)
	secrets, err := client.ExportSecrets(ctx)
	if err != nil {
		return nil, err
	}
	if err := SaveProviderSecrets(secrets); err != nil {
		return nil, err
	}
	return secrets, nil
}

// FormatSecretProviders lists provider names in the pulled map (no key values).
func FormatSecretProviders(secrets map[string]string) string {
	names := make([]string, 0, len(secrets))
	for k := range secrets {
		names = append(names, k)
	}
	sort.Strings(names)
	if len(names) == 0 {
		return "(none)"
	}
	out := ""
	for i, n := range names {
		if i > 0 {
			out += ", "
		}
		out += n
	}
	return out
}
