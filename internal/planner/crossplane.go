package planner

import (
	"strings"

	"github.com/kprompt/kprompt/internal/intent"
	"github.com/kprompt/kprompt/internal/tools/crossplane"
)

func buildCrossplane(in intent.Intent, ns string) (ExecutionPlan, error) {
	resource, _ := in.StringParam("resource")
	if resource == "" {
		resource = "postgres"
	}
	name := strings.TrimSpace(in.Target.Name)
	if name == "" {
		name = crossplane.DefaultClaimName(resource)
	}
	apiVersion, _ := in.StringParam("apiVersion")
	kind, _ := in.StringParam("claimKind")
	if kind == "" {
		kind, _ = in.StringParam("kind")
	}
	composition, _ := in.StringParam("composition")
	provider, _ := in.StringParam("provider")
	size, _ := in.StringParam("size")
	secret, _ := in.StringParam("secret")
	if secret == "" {
		secret, _ = in.StringParam("connectionSecret")
	}
	storageGB := 0
	if v, ok := in.Params["storageGB"]; ok {
		switch n := v.(type) {
		case float64:
			storageGB = int(n)
		case int:
			storageGB = n
		case int32:
			storageGB = int(n)
		}
	}

	manifest, summary, err := crossplane.GenerateClaim(crossplane.ClaimRequest{
		Name:        name,
		Namespace:   ns,
		Resource:    resource,
		APIVersion:  apiVersion,
		Kind:        kind,
		Composition: composition,
		Provider:    provider,
		StorageGB:   storageGB,
		Size:        size,
		SecretName:  secret,
	})
	if err != nil {
		return ExecutionPlan{}, err
	}

	claimKind := kind
	if claimKind == "" {
		claimKind = strings.TrimSpace(in.Target.Kind)
	}
	if claimKind == "" {
		claimKind = "Claim"
	}
	apiVer := apiVersion
	if apiVer == "" {
		switch strings.ToLower(resource) {
		case "bucket", "s3":
			apiVer = "storage.example.org/v1alpha1"
		case "redis", "cache":
			apiVer = "cache.example.org/v1alpha1"
		default:
			apiVer = "database.example.org/v1alpha1"
		}
	}

	return ExecutionPlan{
		Intent: in,
		Actions: []Action{{
			Op:       OpClaimCreate,
			Backend:  "crossplane",
			Manifest: manifest,
			Diff:     summary + "\n\n" + manifest,
			Object: ObjectRef{
				APIVersion: apiVer,
				Kind:       claimKind,
				Name:       name,
				Namespace:  ns,
			},
		}},
		Summary:          summary,
		RequiresApproval: true,
	}, nil
}
