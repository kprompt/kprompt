package argo

import (
	"fmt"
	"regexp"
	"strings"

	"sigs.k8s.io/yaml"
)

// WorkflowRequest is the structured input for manifest generation.
type WorkflowRequest struct {
	Name      string
	Namespace string
	Task      string
	Model     string
	Image     string
	Command   []string
	Args      []string
	GPU       bool
	Dataset   string
}

type modelRecipe struct {
	Image   string
	Command []string
	Args    []string
	GPU     bool
}

var modelRecipes = map[string]modelRecipe{
	"yolov11": {
		Image:   "ultralytics/ultralytics:latest",
		Command: []string{"yolo"},
		Args:    []string{"train", "model=yolo11n.pt", "data=coco8.yaml", "epochs=1"},
		GPU:     true,
	},
	"yolov8": {
		Image:   "ultralytics/ultralytics:latest",
		Command: []string{"yolo"},
		Args:    []string{"train", "model=yolov8n.pt", "data=coco8.yaml", "epochs=1"},
		GPU:     true,
	},
	"yolo": {
		Image:   "ultralytics/ultralytics:latest",
		Command: []string{"yolo"},
		Args:    []string{"train", "model=yolov8n.pt", "data=coco8.yaml", "epochs=1"},
		GPU:     true,
	},
}

var modelFromPrompt = regexp.MustCompile(`(?i)\b(yolo(?:v?\d+)?)\b`)

// InferModelFromPrompt extracts a model name from natural language when params are missing.
func InferModelFromPrompt(prompt string) string {
	m := modelFromPrompt.FindStringSubmatch(prompt)
	if len(m) < 2 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(m[1]))
}

// GenerateWorkflow builds an Argo Workflow manifest YAML from a request.
func GenerateWorkflow(req WorkflowRequest) (manifest string, summary string, err error) {
	req = normalizeRequest(req)
	if req.Name == "" {
		return "", "", fmt.Errorf("workflow name is required")
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}

	image, command, args, gpu, err := resolveContainer(req)
	if err != nil {
		return "", "", err
	}
	if image == "" {
		return "", "", fmt.Errorf("workflow container image is required (set params.image or params.model)")
	}

	container := map[string]any{
		"image": image,
	}
	if len(command) > 0 {
		container["command"] = command
	}
	if len(args) > 0 {
		container["args"] = args
	}
	if gpu {
		container["resources"] = map[string]any{
			"requests": map[string]any{
				"nvidia.com/gpu": "1",
			},
		}
	}

	entrypoint := sanitizeTemplateName(req.Task)
	if entrypoint == "" {
		entrypoint = "main"
	}

	doc := map[string]any{
		"apiVersion": WorkflowGroup + "/v1alpha1",
		"kind":       WorkflowKind,
		"metadata": map[string]any{
			"name":      req.Name,
			"namespace": req.Namespace,
			"labels": map[string]any{
				"app.kubernetes.io/managed-by": "kprompt",
			},
		},
		"spec": map[string]any{
			"entrypoint": entrypoint,
			"templates": []map[string]any{
				{
					"name":      entrypoint,
					"container": container,
				},
			},
		},
	}

	raw, err := yaml.Marshal(doc)
	if err != nil {
		return "", "", err
	}

	taskLabel := req.Task
	if taskLabel == "" {
		taskLabel = "run"
	}
	modelLabel := req.Model
	if modelLabel != "" {
		summary = fmt.Sprintf("Argo Workflow %s/%s: %s %s (image=%s)", WorkflowKind, req.Name, taskLabel, modelLabel, image)
	} else {
		summary = fmt.Sprintf("Argo Workflow %s/%s: %s (image=%s)", WorkflowKind, req.Name, taskLabel, image)
	}
	return string(raw), summary, nil
}

// DefaultWorkflowName builds a DNS-safe workflow name from task and model.
func DefaultWorkflowName(task, model string) string {
	task = strings.ToLower(strings.TrimSpace(task))
	model = strings.ToLower(strings.TrimSpace(model))
	if task == "" {
		task = "run"
	}
	if model != "" {
		return sanitizeWorkflowName(task + "-" + model)
	}
	return sanitizeWorkflowName(task)
}

func normalizeRequest(req WorkflowRequest) WorkflowRequest {
	req.Name = sanitizeWorkflowName(req.Name)
	req.Task = strings.ToLower(strings.TrimSpace(req.Task))
	req.Model = strings.ToLower(strings.TrimSpace(req.Model))
	req.Image = strings.TrimSpace(req.Image)
	req.Dataset = strings.TrimSpace(req.Dataset)
	return req
}

func resolveContainer(req WorkflowRequest) (image string, command, args []string, gpu bool, err error) {
	if req.Image != "" {
		if isShellLauncher(req.Command) {
			return "", nil, nil, false, fmt.Errorf("workflow params.command may not use shell launcher (/bin/sh -c, sh -c, bash -c)")
		}
		image = req.Image
		command = append([]string(nil), req.Command...)
		args = append([]string(nil), req.Args...)
		gpu = req.GPU
		return image, command, args, gpu, nil
	}
	if recipe, ok := modelRecipes[req.Model]; ok {
		image = recipe.Image
		command = append([]string(nil), recipe.Command...)
		args = append([]string(nil), recipe.Args...)
		if req.Dataset != "" {
			args = appendDatasetArg(args, req.Dataset)
		}
		gpu = recipe.GPU || req.GPU
		return image, command, args, gpu, nil
	}
	if req.Model != "" {
		image = "alpine:3.20"
		command = []string{"echo"}
		args = []string{fmt.Sprintf("Training %s (placeholder)", req.Model)}
		gpu = req.GPU
		return image, command, args, gpu, nil
	}
	return "", nil, nil, false, nil
}

func isShellLauncher(command []string) bool {
	if len(command) < 2 {
		return false
	}
	name := strings.ToLower(strings.TrimSpace(command[0]))
	flag := strings.TrimSpace(command[1])
	if flag != "-c" {
		return false
	}
	switch name {
	case "sh", "/bin/sh", "bash", "/bin/bash", "zsh", "/bin/zsh":
		return true
	default:
		return false
	}
}

func appendDatasetArg(args []string, dataset string) []string {
	out := append([]string(nil), args...)
	for i, a := range out {
		if strings.HasPrefix(a, "data=") {
			out[i] = "data=" + dataset
			return out
		}
	}
	return append(out, "data="+dataset)
}

func sanitizeWorkflowName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			return r
		}
		return '-'
	}, name)
	name = strings.Trim(name, "-")
	if name == "" {
		return "kprompt-workflow"
	}
	if len(name) > 63 {
		name = name[:63]
	}
	return name
}

func sanitizeTemplateName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return "main"
	}
	name = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			return r
		}
		return '-'
	}, name)
	name = strings.Trim(name, "-")
	if name == "" {
		return "main"
	}
	return name
}
