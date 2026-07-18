package cluster

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

// listDynamic performs get/list via the dynamic client for any resolved GVR.
func (r *Reader) listDynamic(ctx context.Context, q Query) (Result, error) {
	if r.Dynamic == nil {
		return Result{}, fmt.Errorf("dynamic client is required for kind %q", q.Kind)
	}
	resource := strings.TrimSpace(q.Resource)
	version := strings.TrimSpace(q.Version)
	if resource == "" || version == "" {
		return Result{}, fmt.Errorf(
			"dynamic read requires resolved group/version/resource (got group=%q version=%q resource=%q kind=%q) — run discovery first",
			q.Group, q.Version, q.Resource, q.Kind,
		)
	}
	gvr := schema.GroupVersionResource{
		Group:    q.Group,
		Version:  version,
		Resource: resource,
	}
	kind := firstNonEmpty(q.Kind, resource)
	namespaced := true
	switch q.Scope {
	case ScopeCluster:
		namespaced = false
	case ScopeNamespaced:
		namespaced = true
	default:
		namespaced = !isLikelyClusterResource(resource)
	}

	res := Result{
		Kind:     kind,
		Group:    q.Group,
		Resource: resource,
	}
	if namespaced {
		res.Headers = []string{"NAMESPACE", "NAME", "STATUS", "AGE"}
	} else {
		res.Headers = []string{"NAME", "STATUS", "AGE"}
	}

	ri := r.dynamicResource(gvr, q.Namespace, namespaced)
	opts := metav1.ListOptions{
		LabelSelector: q.LabelSelector,
		Continue:      q.Continue,
	}
	if q.Limit > 0 {
		opts.Limit = q.Limit
	}

	if q.Name != "" {
		obj, err := ri.Get(ctx, q.Name, metav1.GetOptions{})
		if err != nil {
			return res, err
		}
		res.Rows = append(res.Rows, unstructuredRow(obj, namespaced))
		return res, nil
	}

	list, err := ri.List(ctx, opts)
	if err != nil {
		return res, err
	}
	for i := range list.Items {
		res.Rows = append(res.Rows, unstructuredRow(&list.Items[i], namespaced))
	}
	res.Continue = list.GetContinue()
	if q.Limit > 0 && int64(len(res.Rows)) > q.Limit {
		res.Rows = res.Rows[:q.Limit]
		res.Truncated = true
	}
	if res.Continue != "" {
		res.Truncated = true
	}
	return res, nil
}

func (r *Reader) dynamicResource(gvr schema.GroupVersionResource, ns string, namespaced bool) dynamic.ResourceInterface {
	base := r.Dynamic.Resource(gvr)
	if !namespaced {
		return base
	}
	if ns == "" {
		ns = "default"
	}
	return base.Namespace(ns)
}

func unstructuredRow(obj *unstructured.Unstructured, namespaced bool) Row {
	status := genericStatus(obj)
	age := "-"
	if ts := obj.GetCreationTimestamp(); !ts.Time.IsZero() {
		age = formatAge(ts.Time)
	}
	row := Row{
		Name:   obj.GetName(),
		Status: status,
		Ready:  status,
		Extra:  age,
	}
	if namespaced {
		row.Namespace = obj.GetNamespace()
	}
	return row
}

func genericStatus(obj *unstructured.Unstructured) string {
	if phase, ok, _ := unstructured.NestedString(obj.Object, "status", "phase"); ok && phase != "" {
		return phase
	}
	if conds, ok, _ := unstructured.NestedSlice(obj.Object, "status", "conditions"); ok {
		for _, c := range conds {
			m, ok := c.(map[string]any)
			if !ok {
				continue
			}
			t, _ := m["type"].(string)
			st, _ := m["status"].(string)
			if strings.EqualFold(t, "Ready") && st != "" {
				if st == "True" {
					return "Ready"
				}
				return "NotReady"
			}
		}
	}
	if ready, ok, _ := unstructured.NestedInt64(obj.Object, "status", "readyReplicas"); ok {
		desired, _, _ := unstructured.NestedInt64(obj.Object, "status", "replicas")
		if desired == 0 {
			desired, _, _ = unstructured.NestedInt64(obj.Object, "spec", "replicas")
		}
		return fmt.Sprintf("%d/%d", ready, desired)
	}
	switch strings.ToLower(obj.GetKind()) {
	case "secret":
		typ, _, _ := unstructured.NestedString(obj.Object, "type")
		data, _, _ := unstructured.NestedMap(obj.Object, "data")
		if typ == "" {
			typ = "Opaque"
		}
		return fmt.Sprintf("%s %d", typ, len(data))
	case "configmap":
		data, _, _ := unstructured.NestedMap(obj.Object, "data")
		return strconv.Itoa(len(data))
	case "service":
		typ, _, _ := unstructured.NestedString(obj.Object, "spec", "type")
		if typ != "" {
			return typ
		}
	case "node":
		return nodeReadyStatus(obj)
	}
	if len(obj.GetOwnerReferences()) > 0 {
		return obj.GetOwnerReferences()[0].Kind
	}
	return "-"
}

func nodeReadyStatus(obj *unstructured.Unstructured) string {
	conds, ok, _ := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if !ok {
		return "-"
	}
	for _, c := range conds {
		m, ok := c.(map[string]any)
		if !ok {
			continue
		}
		if t, _ := m["type"].(string); t == "Ready" {
			if st, _ := m["status"].(string); st == "True" {
				return "Ready"
			}
			return "NotReady"
		}
	}
	return "-"
}

func isLikelyClusterResource(resource string) bool {
	switch strings.ToLower(resource) {
	case "nodes", "namespaces", "persistentvolumes", "storageclasses",
		"clusterroles", "clusterrolebindings", "csinodes", "csidrivers",
		"priorityclasses", "runtimeclasses", "ingressclasses":
		return true
	default:
		return false
	}
}
