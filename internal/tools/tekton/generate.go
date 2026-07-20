package tekton

import (
	"fmt"
	"regexp"
	"strings"

	"sigs.k8s.io/yaml"
)

// PipelineRequest is the structured input for PipelineRun manifest generation.
type PipelineRequest struct {
	Name      string
	Namespace string
	Repo      string // git URL or repo path
	Image     string
	Task      string // ci, build, test
}

var gitURLFromPrompt = regexp.MustCompile(`(?i)\b(https?://\S+|git@\S+)`)

// InferRepoFromPrompt extracts a git URL when params are missing.
func InferRepoFromPrompt(prompt string) string {
	m := gitURLFromPrompt.FindString(prompt)
	return strings.TrimSpace(m)
}

// GeneratePipelineRun builds a Tekton PipelineRun YAML with an embedded pipelineSpec.
func GeneratePipelineRun(req PipelineRequest) (manifest string, summary string, err error) {
	req = normalizeRequest(req)
	if req.Name == "" {
		return "", "", fmt.Errorf("pipeline run name is required")
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	image := req.Image
	if image == "" {
		image = "alpine/git:2.43.0"
	}
	task := req.Task
	if task == "" {
		task = "ci"
	}

	script := "#!/bin/sh\nset -eu\necho \"kprompt Tekton CI task=" + task + "\"\n"
	if req.Repo != "" {
		script += "echo \"repo=" + req.Repo + "\"\n"
		script += "git clone --depth 1 " + shellQuote(req.Repo) + " /workspace/src\n"
		script += "ls -la /workspace/src\n"
	} else {
		script += "echo \"No repo URL provided — placeholder CI step\"\nsleep 2\n"
	}

	doc := map[string]any{
		"apiVersion": PipelineGroup + "/v1",
		"kind":       PipelineRunKind,
		"metadata": map[string]any{
			"name":      req.Name,
			"namespace": req.Namespace,
			"labels": map[string]any{
				"app.kubernetes.io/managed-by": "kprompt",
			},
		},
		"spec": map[string]any{
			"pipelineSpec": map[string]any{
				"tasks": []map[string]any{
					{
						"name": task,
						"taskSpec": map[string]any{
							"steps": []map[string]any{
								{
									"name":   "run",
									"image":  image,
									"script": script,
								},
							},
						},
					},
				},
			},
		},
	}

	raw, err := yaml.Marshal(doc)
	if err != nil {
		return "", "", err
	}
	if req.Repo != "" {
		summary = fmt.Sprintf("Tekton PipelineRun/%s: %s for %s (image=%s)", req.Name, task, req.Repo, image)
	} else {
		summary = fmt.Sprintf("Tekton PipelineRun/%s: %s (image=%s)", req.Name, task, image)
	}
	return string(raw), summary, nil
}

// DefaultPipelineRunName builds a DNS-safe PipelineRun name.
func DefaultPipelineRunName(task, repoHint string) string {
	task = strings.ToLower(strings.TrimSpace(task))
	if task == "" {
		task = "ci"
	}
	hint := strings.ToLower(strings.TrimSpace(repoHint))
	if hint != "" {
		// Prefer last path segment of a URL.
		hint = strings.TrimSuffix(hint, ".git")
		if i := strings.LastIndex(hint, "/"); i >= 0 {
			hint = hint[i+1:]
		}
		return sanitizeName(task + "-" + hint)
	}
	return sanitizeName(task)
}

func normalizeRequest(req PipelineRequest) PipelineRequest {
	req.Name = sanitizeName(req.Name)
	req.Namespace = strings.TrimSpace(req.Namespace)
	req.Repo = strings.TrimSpace(req.Repo)
	req.Image = strings.TrimSpace(req.Image)
	req.Task = strings.ToLower(strings.TrimSpace(req.Task))
	return req
}

func sanitizeName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			return r
		}
		return '-'
	}, name)
	name = strings.Trim(name, "-")
	if name == "" {
		return "kprompt-ci"
	}
	if len(name) > 63 {
		name = name[:63]
	}
	return name
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}
