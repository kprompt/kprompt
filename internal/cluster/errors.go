package cluster

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/tools/clientcmd"
)

const usageGuide = "https://kprompt.ai/#usage"

// Friendlier maps opaque kubeconfig / API errors to short, actionable messages.
// The original error remains available via errors.Unwrap.
func Friendlier(err error) error {
	if err == nil {
		return nil
	}
	var already *friendlyError
	if errors.As(err, &already) {
		return already
	}
	if msg := classify(err); msg != "" {
		return &friendlyError{msg: msg, err: err}
	}
	return err
}

type friendlyError struct {
	msg string
	err error
}

func (e *friendlyError) Error() string { return e.msg }
func (e *friendlyError) Unwrap() error { return e.err }

func classify(err error) string {
	if msg := classifyConfig(err); msg != "" {
		return msg
	}
	if msg := classifyAPI(err); msg != "" {
		return msg
	}
	if msg := classifyNetwork(err); msg != "" {
		return msg
	}
	return ""
}

func classifyConfig(err error) string {
	var pathErr *os.PathError
	if errors.As(err, &pathErr) && errors.Is(pathErr.Err, os.ErrNotExist) {
		return fmt.Sprintf(
			"kubeconfig not found at %s — create one or set KUBECONFIG. See %s",
			pathErr.Path, usageGuide,
		)
	}

	for e := err; e != nil; e = errors.Unwrap(e) {
		if clientcmd.IsEmptyConfig(e) {
			return fmt.Sprintf(
				"no kubeconfig found — set KUBECONFIG or create ~/.kube/config (try: kubectl cluster-info). See %s",
				usageGuide,
			)
		}
	}

	s := err.Error()
	lower := strings.ToLower(s)

	if strings.Contains(lower, "no configuration has been provided") ||
		strings.Contains(lower, "missing or incomplete configuration") {
		return fmt.Sprintf(
			"no kubeconfig found — set KUBECONFIG or create ~/.kube/config (try: kubectl cluster-info). See %s",
			usageGuide,
		)
	}

	if strings.Contains(lower, "context") &&
		(strings.Contains(lower, "does not exist") || strings.Contains(lower, "not found")) {
		name := extractQuoted(s)
		if name != "" {
			return fmt.Sprintf(
				"kube context %q not found — run: kubectl config get-contexts (or: kprompt config set context <name>)",
				name,
			)
		}
		return "kube context not found — run: kubectl config get-contexts"
	}

	if strings.Contains(lower, "invalid configuration") ||
		(strings.Contains(lower, "certificate") && strings.Contains(lower, "verify")) ||
		strings.Contains(lower, "unable to load root certificates") {
		return fmt.Sprintf(
			"kubeconfig looks invalid or TLS verify failed — check the cluster entry in kubeconfig. See %s",
			usageGuide,
		)
	}

	return ""
}

func classifyAPI(err error) string {
	if apierrors.IsUnauthorized(err) {
		return fmt.Sprintf(
			"not authorized for this cluster — refresh credentials (kubectl auth whoami / re-login). See %s",
			usageGuide,
		)
	}
	if apierrors.IsForbidden(err) {
		verb, resource, ns := forbiddenHints(err)
		switch {
		case verb != "" && resource != "" && ns != "":
			return fmt.Sprintf(
				"RBAC denied: cannot %s %s in namespace %q — ask a cluster admin for access (kubectl auth can-i %s %s -n %s)",
				verb, resource, ns, verb, resource, ns,
			)
		case verb != "" && resource != "":
			return fmt.Sprintf(
				"RBAC denied: cannot %s %s — ask a cluster admin for access (kubectl auth can-i %s %s)",
				verb, resource, verb, resource,
			)
		default:
			return "RBAC denied — your user lacks permission for this action (try: kubectl auth can-i --list)"
		}
	}
	if apierrors.IsTimeout(err) || apierrors.IsServerTimeout(err) {
		return fmt.Sprintf(
			"Kubernetes API timed out — is the cluster reachable? (kubectl cluster-info). See %s",
			usageGuide,
		)
	}
	if apierrors.IsServiceUnavailable(err) {
		return fmt.Sprintf(
			"Kubernetes API unavailable — check the control plane (kubectl cluster-info). See %s",
			usageGuide,
		)
	}
	if meta.IsNoMatchError(err) {
		return "Kubernetes API does not recognize this resource type in the current cluster"
	}
	return ""
}

func classifyNetwork(err error) string {
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return fmt.Sprintf(
			"cannot reach the Kubernetes API (timeout) — is the cluster up / VPN connected? See %s",
			usageGuide,
		)
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return fmt.Sprintf(
			"cannot reach the Kubernetes API (%s) — check kubeconfig server URL and network. See %s",
			opErr.Err, usageGuide,
		)
	}

	lower := strings.ToLower(err.Error())
	switch {
	case strings.Contains(lower, "connection refused"):
		return fmt.Sprintf(
			"Kubernetes API connection refused — is the cluster running? (kubectl cluster-info). See %s",
			usageGuide,
		)
	case strings.Contains(lower, "no such host"),
		strings.Contains(lower, "server misbehaving"):
		return fmt.Sprintf(
			"cannot resolve Kubernetes API host — check the server URL in kubeconfig. See %s",
			usageGuide,
		)
	case strings.Contains(lower, "i/o timeout"),
		strings.Contains(lower, "deadline exceeded"),
		strings.Contains(lower, "context deadline exceeded"):
		return fmt.Sprintf(
			"cannot reach the Kubernetes API (timeout) — is the cluster up / VPN connected? See %s",
			usageGuide,
		)
	case strings.Contains(lower, "connection reset"),
		strings.Contains(lower, "broken pipe"):
		return fmt.Sprintf(
			"connection to the Kubernetes API dropped — retry, or check the control plane. See %s",
			usageGuide,
		)
	}
	return ""
}

func forbiddenHints(err error) (verb, resource, namespace string) {
	return parseForbiddenMessage(err.Error())
}

// parseForbiddenMessage extracts "cannot VERB RESOURCE in namespace NS" style hints.
func parseForbiddenMessage(s string) (verb, resource, namespace string) {
	lower := strings.ToLower(s)
	// typical: User "…" cannot list resource "pods" in API group "" in the namespace "default"
	if i := strings.Index(lower, "cannot "); i >= 0 {
		rest := s[i+len("cannot "):]
		fields := strings.Fields(rest)
		if len(fields) >= 1 {
			verb = strings.ToLower(fields[0])
		}
	}
	if i := strings.Index(lower, `resource "`); i >= 0 {
		rest := s[i+len(`resource "`):]
		if j := strings.Index(rest, `"`); j >= 0 {
			resource = rest[:j]
		}
	}
	if i := strings.Index(lower, `namespace "`); i >= 0 {
		rest := s[i+len(`namespace "`):]
		if j := strings.Index(rest, `"`); j >= 0 {
			namespace = rest[:j]
		}
	}
	return verb, resource, namespace
}

func extractQuoted(s string) string {
	start := strings.Index(s, `"`)
	if start < 0 {
		start = strings.Index(s, `'`)
		if start < 0 {
			return ""
		}
		end := strings.Index(s[start+1:], `'`)
		if end < 0 {
			return ""
		}
		return s[start+1 : start+1+end]
	}
	end := strings.Index(s[start+1:], `"`)
	if end < 0 {
		return ""
	}
	return s[start+1 : start+1+end]
}
