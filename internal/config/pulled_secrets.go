package config

import (
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

var (
	pulledSecretsOnce sync.Once
	pulledSecrets     map[string]string
)

// ProviderSecretsPath returns ~/.kprompt/provider-secrets.yaml (Team pull cache).
func ProviderSecretsPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "provider-secrets.yaml"), nil
}

// loadPulledProviderSecrets reads the Team pull cache once per process.
// Missing or invalid file → empty map. Never logs values.
func loadPulledProviderSecrets() map[string]string {
	pulledSecretsOnce.Do(func() {
		pulledSecrets = map[string]string{}
		path, err := ProviderSecretsPath()
		if err != nil {
			return
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return
		}
		var m map[string]string
		if err := yaml.Unmarshal(data, &m); err != nil || m == nil {
			return
		}
		for k, v := range m {
			k = strings.ToLower(strings.TrimSpace(k))
			v = strings.TrimSpace(v)
			if k != "" && v != "" {
				pulledSecrets[k] = v
			}
		}
	})
	return pulledSecrets
}

// PulledAPIKey returns a key from ~/.kprompt/provider-secrets.yaml if present.
func PulledAPIKey(provider string) string {
	p := strings.ToLower(strings.TrimSpace(provider))
	if p == "" {
		return ""
	}
	return loadPulledProviderSecrets()[p]
}

// ResetPulledSecretsCache clears the in-process cache (tests).
func ResetPulledSecretsCache() {
	pulledSecretsOnce = sync.Once{}
	pulledSecrets = nil
}
