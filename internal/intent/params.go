package intent

import (
	"encoding/json"
	"fmt"
	"strings"
)

// StringParam returns a string params value when present.
func (i Intent) StringParam(key string) (string, bool) {
	v, ok := i.Params[key]
	if !ok || v == nil {
		return "", false
	}
	switch s := v.(type) {
	case string:
		return s, s != ""
	default:
		return fmt.Sprint(s), true
	}
}

// Chart returns params.chart (e.g. bitnami/redis) when set.
func (i Intent) Chart() (string, bool) {
	return i.StringParam("chart")
}

// Release returns params.release (Helm release name) when set.
func (i Intent) Release() (string, bool) {
	return i.StringParam("release")
}

// Repo returns params.repo (Helm repo name) when set.
func (i Intent) Repo() (string, bool) {
	return i.StringParam("repo")
}

// RepoURL returns params.repo_url when set.
func (i Intent) RepoURL() (string, bool) {
	return i.StringParam("repo_url")
}

// Image returns params.image when set.
func (i Intent) Image() (string, bool) {
	return i.StringParam("image")
}

// Port returns params.port when set (container/service port).
func (i Intent) Port() (int32, bool) {
	v, ok := i.Params["port"]
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		return int32(n), true
	case int:
		return int32(n), true
	case int32:
		return n, true
	case json.Number:
		i64, err := n.Int64()
		if err != nil {
			return 0, false
		}
		return int32(i64), true
	default:
		return 0, false
	}
}

// MinMemory returns a memory quantity filter from params.minMemory when set
// (e.g. "2Gi", "2048Mi").
func (i Intent) MinMemory() (string, bool) {
	return i.StringParam("minMemory")
}

// LabelSelector returns params.labelSelector when set.
func (i Intent) LabelSelector() (string, bool) {
	return i.StringParam("labelSelector")
}

// WantService is true when params.createService is true, or a port is set.
func (i Intent) WantService() bool {
	if _, ok := i.Port(); ok {
		return true
	}
	v, ok := i.Params["createService"]
	if !ok {
		return false
	}
	switch b := v.(type) {
	case bool:
		return b
	case string:
		return strings.EqualFold(b, "true") || b == "1"
	default:
		return false
	}
}

// Revision returns params.revision when set (Deployment rollout target revision).
func (i Intent) Revision() (int64, bool) {
	v, ok := i.Params["revision"]
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		return int64(n), true
	case int:
		return int64(n), true
	case int64:
		return n, true
	case json.Number:
		i64, err := n.Int64()
		if err != nil {
			return 0, false
		}
		return i64, true
	default:
		return 0, false
	}
}

// TailLines returns params.tail when set (log line count).
func (i Intent) TailLines() (int64, bool) {
	v, ok := i.Params["tail"]
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		return int64(n), true
	case int:
		return int64(n), true
	case int64:
		return n, true
	case json.Number:
		i64, err := n.Int64()
		if err != nil {
			return 0, false
		}
		return i64, true
	default:
		return 0, false
	}
}

// Container returns params.container when set.
func (i Intent) Container() (string, bool) {
	return i.StringParam("container")
}
