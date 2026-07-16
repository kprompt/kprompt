package planner

import "github.com/kprompt/kprompt/internal/intent"

// Op is a planned Kubernetes operation.
type Op string

const (
	OpCreate   Op = "create"
	OpUpdate   Op = "update"
	OpScale    Op = "scale"
	OpRollback Op = "rollback"
	OpDelete   Op = "delete"
	OpGet      Op = "get"
	OpHelmRepo Op = "helm-repo"
	OpHelmRepoUpdate Op = "helm-repo-update"
	OpHelmInstall Op = "helm-install"
	OpHelmUpgrade Op = "helm-upgrade"
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
	// Revision is the Deployment rollout target (nil = previous revision).
	Revision *int64
	// Backend is the integration owner (kubernetes, helm).
	Backend string
	// Command is the argv shown and executed for CLI backends (includes binary).
	Command []string
}

// ExecutionPlan is the reviewable output of planning.
type ExecutionPlan struct {
	Intent           intent.Intent
	Actions          []Action
	Summary          string
	RequiresApproval bool
}
