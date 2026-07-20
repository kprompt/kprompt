package crossplane

import (
	"fmt"
	"strings"

	"sigs.k8s.io/yaml"
)

// ClaimRequest is the structured input for a Crossplane claim manifest.
type ClaimRequest struct {
	Name        string
	Namespace   string
	Resource    string // postgres | bucket | redis (template key)
	APIVersion  string // override
	Kind        string // override claim kind
	Composition string // optional compositionRef.name
	Provider    string // compositionSelector matchLabels.provider
	StorageGB   int
	Size        string // bucket size class or redis size hint
	SecretName  string // writeConnectionSecretToRef.name
}

// GenerateClaim builds a namespaced Crossplane claim YAML (cloud mutation — high risk).
func GenerateClaim(req ClaimRequest) (manifest string, summary string, err error) {
	req = normalizeClaimRequest(req)
	tmpl, err := claimTemplate(req.Resource)
	if err != nil {
		return "", "", err
	}
	if req.APIVersion != "" {
		tmpl.APIVersion = req.APIVersion
	}
	if req.Kind != "" {
		tmpl.Kind = req.Kind
	}
	if req.Name == "" {
		req.Name = DefaultClaimName(req.Resource)
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	if req.SecretName == "" {
		req.SecretName = req.Name + "-conn"
	}
	if req.StorageGB <= 0 {
		req.StorageGB = tmpl.DefaultStorageGB
	}

	spec := map[string]any{
		"parameters": tmpl.parameters(req),
	}
	if req.Composition != "" {
		spec["compositionRef"] = map[string]any{"name": req.Composition}
	} else if req.Provider != "" {
		spec["compositionSelector"] = map[string]any{
			"matchLabels": map[string]any{"provider": req.Provider},
		}
	}
	spec["writeConnectionSecretToRef"] = map[string]any{
		"name": req.SecretName,
	}

	doc := map[string]any{
		"apiVersion": tmpl.APIVersion,
		"kind":       tmpl.Kind,
		"metadata": map[string]any{
			"name":      req.Name,
			"namespace": req.Namespace,
			"labels": map[string]any{
				"app.kubernetes.io/managed-by": "kprompt",
			},
			"annotations": map[string]any{
				"kprompt.io/crossplane-resource": req.Resource,
			},
		},
		"spec": spec,
	}

	raw, err := yaml.Marshal(doc)
	if err != nil {
		return "", "", err
	}
	summary = fmt.Sprintf(
		"Crossplane claim %s/%s (%s) — cloud resource, requires strong approval",
		tmpl.Kind, req.Name, req.Resource,
	)
	return string(raw), summary, nil
}

// DefaultClaimName builds a DNS-safe claim name.
func DefaultClaimName(resource string) string {
	resource = sanitizeName(resource)
	if resource == "" {
		return "kprompt-claim"
	}
	return sanitizeName(resource + "-claim")
}

type claimTmpl struct {
	APIVersion       string
	Kind             string
	DefaultStorageGB int
	parameters       func(ClaimRequest) map[string]any
}

func claimTemplate(resource string) (claimTmpl, error) {
	switch strings.ToLower(strings.TrimSpace(resource)) {
	case "postgres", "postgresql", "database", "db":
		return claimTmpl{
			APIVersion:       "database.example.org/v1alpha1",
			Kind:             "PostgreSQLInstance",
			DefaultStorageGB: 20,
			parameters: func(req ClaimRequest) map[string]any {
				return map[string]any{
					"storageGB": req.StorageGB,
					"engine":    "PostgreSQL",
				}
			},
		}, nil
	case "bucket", "s3", "objectstorage":
		return claimTmpl{
			APIVersion:       "storage.example.org/v1alpha1",
			Kind:             "Bucket",
			DefaultStorageGB: 0,
			parameters: func(req ClaimRequest) map[string]any {
				p := map[string]any{"versioning": true}
				if req.Size != "" {
					p["size"] = req.Size
				}
				return p
			},
		}, nil
	case "redis", "cache":
		return claimTmpl{
			APIVersion:       "cache.example.org/v1alpha1",
			Kind:             "RedisInstance",
			DefaultStorageGB: 0,
			parameters: func(req ClaimRequest) map[string]any {
				p := map[string]any{"engine": "Redis"}
				if req.Size != "" {
					p["size"] = req.Size
				} else {
					p["size"] = "small"
				}
				return p
			},
		}, nil
	default:
		if resource == "" {
			return claimTmpl{}, fmt.Errorf("crossplane claim requires a resource type (postgres, bucket, redis)")
		}
		return claimTmpl{}, fmt.Errorf("unsupported crossplane resource %q (postgres, bucket, redis)", resource)
	}
}

func normalizeClaimRequest(req ClaimRequest) ClaimRequest {
	req.Name = sanitizeName(req.Name)
	req.Namespace = strings.TrimSpace(req.Namespace)
	req.Resource = strings.ToLower(strings.TrimSpace(req.Resource))
	req.APIVersion = strings.TrimSpace(req.APIVersion)
	req.Kind = strings.TrimSpace(req.Kind)
	req.Composition = sanitizeName(req.Composition)
	req.Provider = strings.TrimSpace(req.Provider)
	req.Size = strings.TrimSpace(req.Size)
	req.SecretName = sanitizeName(req.SecretName)
	return req
}

func sanitizeName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '.' || r == '/' {
			// keep slash only for apiVersion path elsewhere — strip here
			if r == '/' || r == '.' {
				return '-'
			}
			return r
		}
		return '-'
	}, name)
	name = strings.Trim(name, "-")
	if name == "" {
		return ""
	}
	if len(name) > 63 {
		name = name[:63]
	}
	return name
}
