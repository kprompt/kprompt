package cluster

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	// DefaultReadLimit caps list responses for generic reads.
	DefaultReadLimit = 500
	// MaxReadLimit is the hard ceiling for params.limit.
	MaxReadLimit = 5000
	// DefaultReadTimeout bounds a single get/list call.
	DefaultReadTimeout = 30 * time.Second
	// MaxReadTimeout is the hard ceiling for params.timeout.
	MaxReadTimeout = 2 * time.Minute
)

// ResourceScope is whether a resource lives in a namespace or at cluster scope.
type ResourceScope string

const (
	ScopeNamespaced    ResourceScope = "namespaced"
	ScopeCluster       ResourceScope = "cluster"
	ScopeScopeUnknown  ResourceScope = "unknown"
)

// ResourceRef identifies a Kubernetes API resource for a read.
// Accepts kind, plural, short name, or group-qualified forms such as deployments.apps.
type ResourceRef struct {
	// Raw is the user/LLM supplied identifier before normalization.
	Raw string `json:"raw,omitempty"`
	// Kind is the singular PascalCase kind when known (Pod, Deployment, Node).
	Kind string `json:"kind,omitempty"`
	// Resource is the plural API resource name (pods, deployments, nodes).
	Resource string `json:"resource,omitempty"`
	// Group is the API group ("" for core, apps, batch, or a CRD group).
	Group string `json:"group,omitempty"`
	// Version is optional; discovery (T-049) fills the served version when empty.
	Version string `json:"version,omitempty"`
	// Scope is namespaced, cluster, or unknown until discovery resolves it.
	Scope ResourceScope `json:"scope,omitempty"`
}

// ReadRequest is the generic read-only contract used by planners and readers.
type ReadRequest struct {
	Resource      ResourceRef
	Namespace     string
	Name          string // empty → list
	LabelSelector string
	Limit         int64
	Continue      string
	Timeout       time.Duration
}

// ReadTable is the stable tabular output contract for human and JSON modes.
type ReadTable struct {
	APIVersion string              `json:"apiVersion,omitempty"`
	Kind       string              `json:"kind"`
	Group      string              `json:"group,omitempty"`
	Resource   string              `json:"resource,omitempty"`
	Namespace  string              `json:"namespace,omitempty"`
	Scope      ResourceScope       `json:"scope,omitempty"`
	Headers    []string            `json:"headers"`
	Rows       []map[string]string `json:"rows"`
	Continue   string              `json:"continue,omitempty"`
	Truncated  bool                `json:"truncated,omitempty"`
}

// ErrUnknownResource reports a resource identity that discovery cannot resolve.
var ErrUnknownResource = errors.New("unknown Kubernetes resource")

// ErrAmbiguousResource reports a short name or kind matching multiple API resources.
var ErrAmbiguousResource = errors.New("ambiguous Kubernetes resource")

// UnknownResourceError carries the unresolved identity.
type UnknownResourceError struct {
	Ref ResourceRef
}

func (e UnknownResourceError) Error() string {
	label := e.Ref.Display()
	if label == "" {
		label = e.Ref.Raw
	}
	return fmt.Sprintf(
		"unknown Kubernetes resource %q — use a kind, plural, short name, or group-qualified name (e.g. deployments.apps)",
		label,
	)
}

func (e UnknownResourceError) Unwrap() error { return ErrUnknownResource }

// AmbiguousResourceError lists candidate matches for a short name.
type AmbiguousResourceError struct {
	Query      string
	Candidates []ResourceRef
}

func (e AmbiguousResourceError) Error() string {
	parts := make([]string, 0, len(e.Candidates))
	for _, c := range e.Candidates {
		parts = append(parts, c.Qualified())
	}
	return fmt.Sprintf(
		"ambiguous resource %q matches %s — qualify with group (e.g. deployments.apps)",
		e.Query,
		strings.Join(parts, ", "),
	)
}

func (e AmbiguousResourceError) Unwrap() error { return ErrAmbiguousResource }

// ParseResourceRef parses kind, plural, short name, or group-qualified resource names.
// Examples: pods, po, Pod, deployments.apps, widgets.example.com.
func ParseResourceRef(raw string) (ResourceRef, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ResourceRef{}, fmt.Errorf("resource identity is required")
	}
	ref := ResourceRef{Raw: raw, Scope: ScopeScopeUnknown}

	// Group-qualified: <resource>.<group> (e.g. deployments.apps, mycrds.example.com).
	if i := strings.Index(raw, "."); i > 0 {
		resource := strings.ToLower(raw[:i])
		group := strings.ToLower(raw[i+1:])
		if resource == "" || group == "" {
			return ResourceRef{}, fmt.Errorf("invalid group-qualified resource %q", raw)
		}
		ref.Resource = resource
		ref.Group = group
		ref.Kind = kindFromResource(resource)
		ref.Scope = guessScope(ref)
		return ref, nil
	}

	canonical := NormalizeKind(raw)
	switch canonical {
	case "Pod":
		ref.Kind, ref.Resource, ref.Group, ref.Scope = "Pod", "pods", "", ScopeNamespaced
	case "Deployment":
		ref.Kind, ref.Resource, ref.Group, ref.Scope = "Deployment", "deployments", "apps", ScopeNamespaced
	case "Service":
		ref.Kind, ref.Resource, ref.Group, ref.Scope = "Service", "services", "", ScopeNamespaced
	case "Workflow":
		ref.Kind, ref.Resource, ref.Group, ref.Scope = "Workflow", "workflows", "argoproj.io", ScopeNamespaced
	case "Node":
		ref.Kind, ref.Resource, ref.Group, ref.Scope = "Node", "nodes", "", ScopeCluster
	case "ConfigMap":
		ref.Kind, ref.Resource, ref.Group, ref.Scope = "ConfigMap", "configmaps", "", ScopeNamespaced
	case "Secret":
		// Secrets are normal discoverable resources; no special-case redaction in the contract.
		ref.Kind, ref.Resource, ref.Group, ref.Scope = "Secret", "secrets", "", ScopeNamespaced
	default:
		lower := strings.ToLower(raw)
		ref.Resource = lower
		ref.Kind = kindFromResource(lower)
		ref.Scope = ScopeScopeUnknown
	}
	return ref, nil
}

// Qualified returns resource.group or resource for core.
func (r ResourceRef) Qualified() string {
	if r.Resource == "" && r.Kind != "" {
		return strings.ToLower(r.Kind)
	}
	if r.Group == "" {
		return r.Resource
	}
	return r.Resource + "." + r.Group
}

// Display is a human-facing label preferring Kind when set.
func (r ResourceRef) Display() string {
	if r.Kind != "" {
		if r.Group != "" && r.Group != "apps" && r.Group != "argoproj.io" {
			return r.Kind + " (" + r.Qualified() + ")"
		}
		return r.Kind
	}
	return r.Qualified()
}

// NormalizeReadRequest applies defaults and validates limits/scope rules.
func NormalizeReadRequest(req ReadRequest) (ReadRequest, error) {
	if req.Resource.Raw == "" && req.Resource.Kind == "" && req.Resource.Resource == "" {
		return ReadRequest{}, fmt.Errorf("read request missing resource identity")
	}
	if req.Resource.Raw != "" && req.Resource.Resource == "" && req.Resource.Kind == "" {
		parsed, err := ParseResourceRef(req.Resource.Raw)
		if err != nil {
			return ReadRequest{}, err
		}
		req.Resource = parsed
	}
	req.Namespace = strings.TrimSpace(req.Namespace)
	req.Name = strings.TrimSpace(req.Name)
	req.LabelSelector = strings.TrimSpace(req.LabelSelector)
	req.Continue = strings.TrimSpace(req.Continue)

	if req.Resource.Scope == ScopeCluster {
		if req.Namespace != "" && !strings.EqualFold(req.Namespace, "default") {
			return ReadRequest{}, fmt.Errorf(
				"cluster-scoped resource %s does not take a namespace (got %q)",
				req.Resource.Display(),
				req.Namespace,
			)
		}
		req.Namespace = ""
	} else if req.Resource.Scope == ScopeNamespaced && req.Namespace == "" {
		req.Namespace = "default"
	}

	if req.Limit == 0 {
		req.Limit = DefaultReadLimit
	}
	if req.Limit < 1 || req.Limit > MaxReadLimit {
		return ReadRequest{}, fmt.Errorf("read limit must be between 1 and %d", MaxReadLimit)
	}
	if req.Timeout == 0 {
		req.Timeout = DefaultReadTimeout
	}
	if req.Timeout < time.Second || req.Timeout > MaxReadTimeout {
		return ReadRequest{}, fmt.Errorf(
			"read timeout must be between 1s and %s",
			MaxReadTimeout,
		)
	}
	return req, nil
}

// QueryFromReadRequest adapts the generic contract to the legacy Query shape.
func QueryFromReadRequest(req ReadRequest) Query {
	kind := req.Resource.Kind
	if kind == "" {
		kind = req.Resource.Resource
	}
	return Query{
		Kind:          kind,
		Namespace:     req.Namespace,
		Name:          req.Name,
		LabelSelector: req.LabelSelector,
		Limit:         req.Limit,
		Continue:      req.Continue,
		Timeout:       req.Timeout,
		Group:         req.Resource.Group,
		Resource:      req.Resource.Resource,
	}
}

func kindFromResource(resource string) string {
	switch resource {
	case "pods", "pod":
		return "Pod"
	case "deployments", "deployment":
		return "Deployment"
	case "services", "service":
		return "Service"
	case "nodes", "node":
		return "Node"
	case "configmaps", "configmap", "cm":
		return "ConfigMap"
	case "secrets", "secret":
		return "Secret"
	case "workflows", "workflow":
		return "Workflow"
	default:
		if resource == "" {
			return ""
		}
		// Best-effort singularization for CRDs / unknown plurals.
		if strings.HasSuffix(resource, "ies") && len(resource) > 3 {
			return strings.ToUpper(resource[:1]) + resource[1:len(resource)-3] + "y"
		}
		if strings.HasSuffix(resource, "s") && len(resource) > 1 {
			return strings.ToUpper(resource[:1]) + resource[1:len(resource)-1]
		}
		return strings.ToUpper(resource[:1]) + resource[1:]
	}
}

func guessScope(ref ResourceRef) ResourceScope {
	switch {
	case ref.Resource == "nodes" || ref.Resource == "namespaces" ||
		ref.Resource == "persistentvolumes" || ref.Resource == "storageclasses":
		return ScopeCluster
	case ref.Group == "" || ref.Group == "apps" || ref.Group == "batch" ||
		ref.Group == "argoproj.io":
		return ScopeNamespaced
	default:
		return ScopeScopeUnknown
	}
}
