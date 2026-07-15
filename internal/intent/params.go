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
