package runworker

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"

	"github.com/kprompt/kprompt/internal/config"
	"github.com/kprompt/kprompt/internal/output"
	"github.com/kprompt/kprompt/internal/pipeline"
	"github.com/kprompt/kprompt/internal/team"
)

// Execute runs the local plan pipeline for a claimed Team job (never auto-applies).
func Execute(ctx context.Context, job team.RunJob) (team.PostRunResultInput, error) {
	return execute(ctx, job, false)
}

// ExecuteApply re-runs the pipeline with approve consent after in-app approve (A-054).
func ExecuteApply(ctx context.Context, job team.RunJob) (team.PostRunResultInput, error) {
	return execute(ctx, job, true)
}

type executeDeps struct {
	loadFile func() (config.File, error)
	pipeline pipeline.Deps
}

func execute(ctx context.Context, job team.RunJob, approve bool) (team.PostRunResultInput, error) {
	return executeWith(ctx, job, approve, executeDeps{loadFile: config.LoadFile})
}

func executeWith(ctx context.Context, job team.RunJob, approve bool, deps executeDeps) (team.PostRunResultInput, error) {
	file, err := deps.loadFile()
	if err != nil {
		return team.PostRunResultInput{}, err
	}
	cfg := config.Merge(file, "", "", job.ContextHint, job.Namespace, approve, job.Prompt)
	cfg.Output = "json"
	if job.Namespace != "" {
		cfg.NamespaceFromCLI = true
	}
	if job.ContextHint != "" {
		cfg.ContextFromCLI = true
	}
	team.ApplyOrgContextPolicy(&cfg)

	var last *output.PlanResult
	var buf bytes.Buffer
	tty := false
	confirm := func(io.Writer) (bool, error) { return false, nil }
	if approve {
		confirm = func(io.Writer) (bool, error) { return true, nil }
	}
	pipelineDeps := deps.pipeline
	pipelineDeps.IsTerminal = &tty
	pipelineDeps.Confirm = confirm
	pipelineDeps.OnResult = func(doc output.PlanResult) {
		cp := doc
		last = &cp
	}
	err = pipeline.RunWith(ctx, cfg, io.MultiWriter(&buf, os.Stderr), pipelineDeps)
	if last == nil {
		msg := "no plan result"
		if err != nil {
			msg = err.Error()
		}
		return team.PostRunResultInput{Status: "failed", Error: msg, Summary: "bridge failed"}, err
	}

	body, mErr := json.Marshal(last)
	if mErr != nil {
		return team.PostRunResultInput{Status: "failed", Error: mErr.Error()}, mErr
	}
	summary := last.Plan.Summary
	if summary == "" {
		summary = "plan"
	}
	if approve {
		status := "succeeded"
		if last.Risk.Denied {
			status = "denied"
		} else if err != nil && !last.Applied {
			status = "failed"
		}
		return team.PostRunResultInput{
			Status:  status,
			Summary: summary,
			Risk:    last.Risk.Level,
			Body:    body,
		}, nil
	}

	mode := strings.ToLower(strings.TrimSpace(job.ApproveMode))
	status := "succeeded"
	switch {
	case last.Risk.Denied:
		status = "denied"
	case mode == "require_approve" && last.Plan.RequiresApproval:
		status = "awaiting_approve"
	case mode == "auto_if_policy_allows" && last.Plan.RequiresApproval:
		status = "awaiting_approve"
	}
	return team.PostRunResultInput{
		Status:  status,
		Summary: summary,
		Risk:    last.Risk.Level,
		Body:    body,
	}, nil
}
