package intent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/kprompt/kprompt/internal/llm"
)

const systemPrompt = `You convert Kubernetes ops requests into a single Intent JSON object.
Rules:
- kind must be one of: deploy, install, scale, rollback, get, explain, logs, describe, workflow, performance, delete, deny, unknown
- For scale: set target.name, target.kind (usually Deployment), target.namespace if mentioned, params.replicas as a number
- For rollback / undo rollout / revert: kind=rollback; set target.name to the Deployment; target.kind Deployment; target.namespace if mentioned; optional params.revision (number) to roll back to a specific revision (omit for previous revision)
- For install (helm chart): kind=install; set target.name to the app or release name (e.g. redis); target.namespace if mentioned; optional params.release, params.chart, params.repo, params.repo_url, params.replicas
- For upgrade (helm chart): kind=upgrade; set target.name to the release or app name (e.g. nginx); params.version or params.chart_version (chart version, e.g. "15.3.2" or "1.3"); target.namespace if mentioned; optional params.release, params.chart
- For deploy (kubernetes manifests): set target.name (workload name), params.image when known (e.g. redis:7-alpine, nginx:1.27-alpine); for well-known apps like "redis" or "nginx" name alone is enough; optional params.replicas (default 1), params.port and/or params.createService=true for a ClusterIP Service. Use deploy — not install — when the user says deploy
- For get/list/show: kind=get; set target.kind to Pod, Deployment, Service, or Workflow; target.namespace if mentioned; target.name only for a single object; optional params.labelSelector; optional params.minMemory (e.g. "2Gi") when the user asks for pods using more than X memory (filter by memory requests)
- For explain/why crashing/failing: kind=explain; set target.name to the workload; target.kind Deployment or Pod; target.namespace if mentioned
- For slow/performance/latency requests (e.g. "why is my api slow"): kind=performance; set target.name to the workload; target.kind Deployment; target.namespace if mentioned; optional params.window such as "15m" or "1h"
- For logs / tail logs / show logs: kind=logs; set target.name (Pod or Deployment); optional target.kind; optional params.tail (lines, default 100); optional params.container
- For describe / status of / show details (not crash-focused): kind=describe; set target.name; target.kind Pod or Deployment
- For Argo Workflows / train a model / submit a workflow: kind=workflow; set target.name when the user names the workflow (otherwise omit and set params.model); target.namespace if mentioned; params.task (e.g. train, infer); params.model (e.g. yolov11); optional params.image, params.dataset, params.gpu=true, params.command, params.args
- For delete / remove a single named resource: kind=delete; MUST set target.name and target.kind (Deployment, Service, or Pod); target.namespace if mentioned. Never delete without a concrete name. Namespace deletes and wipe/all/cluster deletes use kind=deny
- For clearly destructive wipe/delete-cluster / delete-all / delete namespace requests: kind=deny
- Namespace from phrases: "in staging", "in the prod namespace", "in production" → set target.namespace (aliases: stage→staging, prod→prod, production→production, dev→dev)
- Kube context from phrases: "on kind-kprompt-e2e context", "using context docker-desktop", "with the prod-cluster context" → set top-level context (kubeconfig context name)
- Prefer Deployment as target.kind for named apps when unspecified
- Only emit JSON matching the schema`

// Extract uses an LLM provider to produce a structured Intent.
// Call ApplyScope afterward to merge CLI overrides, phrase heuristics, and defaults.
func Extract(ctx context.Context, provider llm.Provider, prompt string) (Intent, error) {
	schema := json.RawMessage(SchemaJSON)
	raw, err := provider.CompleteStructured(ctx, llm.CompletionRequest{
		System: systemPrompt,
		User:   prompt,
	}, schema)
	if err != nil {
		return Intent{}, fmt.Errorf("intent extract: %w", err)
	}
	in, err := ParseStructured(raw)
	if err != nil {
		return Intent{}, fmt.Errorf("intent parse: %w", err)
	}
	in.Raw = prompt
	return in, nil
}
