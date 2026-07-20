package intent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/kprompt/kprompt/internal/llm"
)

const systemPrompt = `You convert Kubernetes ops requests into a single Intent JSON object.
Rules:
- kind must be one of: deploy, install, scale, rollback, get, explain, logs, describe, workflow, tekton, keda, istio, performance, trace, dashboard, optimize, graph, delete, deny, unknown
- For scale: set target.name, target.kind (usually Deployment), target.namespace if mentioned, params.replicas as a number. Plain replica scale (e.g. "scale api to 0") stays kind=scale — do not use keda unless the user mentions KEDA, ScaledObject, scale-to-zero with an event/queue trigger, or event-driven autoscaling
- For rollback / undo rollout / revert: kind=rollback; set target.name to the Deployment; target.kind Deployment; target.namespace if mentioned; optional params.revision (number) to roll back to a specific revision (omit for previous revision)
- For install (helm chart): kind=install; set target.name to the app or release name (e.g. redis); target.namespace if mentioned; optional params.release, params.chart, params.repo, params.repo_url, params.replicas
- For upgrade (helm chart): kind=upgrade; set target.name to the release or app name (e.g. nginx); params.version or params.chart_version (chart version, e.g. "15.3.2" or "1.3"); target.namespace if mentioned; optional params.release, params.chart
- For deploy (kubernetes manifests): set target.name (workload name), params.image when known (e.g. redis:7-alpine, nginx:1.27-alpine); for well-known apps like "redis" or "nginx" name alone is enough; optional params.replicas (default 1), params.port and/or params.createService=true for a ClusterIP Service. Use deploy — not install — when the user says deploy
- For get/list/show: kind=get; set target.kind to any Kubernetes resource identity the user names — Kind (Pod, Deployment, Node, ConfigMap, Secret), plural (pods, nodes), short name (po, cm), or group-qualified (deployments.apps, widgets.example.com); target.namespace if mentioned (omit/ignore for cluster-scoped kinds like Node); target.name only for a single object; optional params.labelSelector; optional params.limit; optional params.timeout (e.g. "30s"); optional params.minMemory (e.g. "2Gi") when the user asks for pods using more than X memory (filter by memory requests). Secrets are normal readable resources — do not refuse or redact them in intent extraction.
- For explain/why crashing/failing: kind=explain; set target.name to the workload; target.kind Deployment or Pod; target.namespace if mentioned
- For slow/performance/latency requests (e.g. "why is my api slow"): kind=performance; set target.name to the workload; target.kind Deployment; target.namespace if mentioned; optional params.window such as "15m" or "1h"
- For cluster optimize / rightsizing / idle workload asks (e.g. "optimize my cluster"): kind=optimize; omit target.name; set params.scope=cluster for whole-cluster; set target.namespace only when a namespace is named; optional params.window (default 1h). Optimize is read-only — never emit scale/patch/delete for this kind
- For service dependency graph asks (e.g. "show service dependency graph"): kind=graph; omit target.name; set params.scope=cluster unless a namespace is named; optional params.includeNetworkPolicy=true (default true). Graph is read-only
- For distributed tracing requests (e.g. "trace payment request"): kind=trace; set target.name to the service name (e.g. payment); target.kind Service; optional params.operation for an explicitly named span/route; optional params.window up to 24h
- For Grafana dashboard requests (e.g. "show dashboard" or "show payments dashboard"): kind=dashboard; set target.name to the dashboard search text when named; target.kind Dashboard; optional params.uid only when the user gives an explicit Grafana dashboard UID
- For logs / tail logs / show logs: kind=logs; set target.name (Pod or Deployment); optional target.kind; optional params.tail (lines, default 100); optional params.container
- For describe / status of / show details (not crash-focused): kind=describe; set target.name; target.kind Pod or Deployment
- For Argo Workflows / train a model / submit a workflow: kind=workflow; set target.name when the user names the workflow (otherwise omit and set params.model); target.namespace if mentioned; params.task (e.g. train, infer); params.model (e.g. yolov11); optional params.image, params.dataset, params.gpu=true, params.command, params.args
- For Tekton CI / create a CI pipeline / PipelineRun: kind=tekton; set target.name when named; target.kind PipelineRun; optional params.repo_url (git URL), params.image, params.task (ci/build/test). Requires approval to submit
- For KEDA / ScaledObject / event-driven / scale-to-zero with queue or redis: kind=keda; set target.name to the Deployment to scale; target.kind ScaledObject; optional params.trigger (cpu|redis|cron), params.minReplicas (0 for scale-to-zero), params.maxReplicas, params.queue / params.listName, params.address. Requires approval to create
- For Istio / VirtualService / canary / traffic split (read-first): kind=istio; set target.name when a VirtualService or host is named; target.kind VirtualService; target.namespace if mentioned. Do not emit mutate/delete for this kind
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
