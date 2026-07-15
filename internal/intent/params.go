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
