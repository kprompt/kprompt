package team

import (
	"context"
	"fmt"
	"os"
)

// AuditEventInput is the CLI → control-plane audit payload (no manifests/secrets).
type AuditEventInput struct {
	Prompt         string  `json:"prompt"`
	PlanSummary    string  `json:"plan_summary"`
	Risk           string  `json:"risk"`
	Decision       string  `json:"decision"` // planned|approved|denied|applied
	ClusterContext string  `json:"cluster_context"`
	Namespace      string  `json:"namespace"`
	PlanResultRef  *string `json:"plan_result_ref,omitempty"`
}

// AppendAudit POSTs one audit event for the authenticated org.
func (c *Client) AppendAudit(ctx context.Context, in AuditEventInput) error {
	return c.doJSON(ctx, "POST", "/v1/audit/events", in, c.Token, nil)
}

// PushAuditBestEffort sends an audit event when enrolled. Never fails the caller.
// Skipped when not enrolled or KPROMPT_DISABLE_AUDIT=1.
func PushAuditBestEffort(ctx context.Context, in AuditEventInput) {
	if os.Getenv("KPROMPT_DISABLE_AUDIT") == "1" {
		return
	}
	creds, _, err := LoadCredentials()
	if err != nil {
		return
	}
	token := ResolveToken(creds)
	if token == "" {
		return
	}
	client := NewClient(ResolveAPIURL(creds), token)
	if err := client.AppendAudit(ctx, in); err != nil {
		fmt.Fprintf(os.Stderr, "warning: audit push failed: %v\n", err)
	}
}
