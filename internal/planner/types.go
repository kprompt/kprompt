package planner

import "github.com/kprompt/kprompt/internal/intent"

// Op is a planned Kubernetes operation.
type Op string

const (
	OpCreate Op = "create"
	OpUpdate Op = "update"
	OpScale  Op = "scale"
	OpDelete Op = "delete"
	OpGet    Op = "get"
)

// ObjectRef is a Kubernetes object identity.
type ObjectRef struct {
	APIVersion string
	Kind       string
	Name       string
	Namespace  string
}

// Action is one step in an execution plan.
type Action struct {
	Op       Op
	Object   ObjectRef
	Manifest string
	Diff     string
	Replicas *int32
}

// ExecutionPlan is the reviewable output of planning.
type ExecutionPlan struct {
	Intent           intent.Intent
	Actions          []Action
	Summary          string
	RequiresApproval bool
}
