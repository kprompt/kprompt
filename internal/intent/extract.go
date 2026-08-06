package intent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kprompt/kprompt/internal/llm"
)

const systemPrompt = `You convert Kubernetes ops requests into a single Intent JSON object.
Rules:
- kind MUST be exactly one of: deploy, install, upgrade, scale, rollback, get, explain, investigate, why, timeline, impact, audit, cleanup, search, score, architecture, learn, drift, logs, describe, workflow, tekton, keda, hpa, istio, crossplane, gitops, performance, trace, dashboard, optimize, roast, graph, delete, deny, unknown
- CRITICAL: never invent kind values. Do not emit hpascaleup, create, apply, patch, autoscale, HorizontalPodAutoscaler, or any other string outside the list above. If the request is not covered by a listed kind, use kind=unknown with low confidence
- For scale: set target.name, target.kind (usually Deployment), target.namespace if mentioned, params.replicas as a number. Plain replica scale (e.g. "scale api to 0") stays kind=scale — do not use keda/hpa unless the user asks for autoscaling
- For native HPA create (e.g. "add HPA for redis", "create HorizontalPodAutoscaler for api", "autoscale api with CPU"): kind=hpa; set target.name to the Deployment to scale; target.kind HorizontalPodAutoscaler; optional params.minReplicas (default 1), params.maxReplicas (default 10), params.cpu or params.cpuPercent (default 70), params.memory or params.memoryPercent, params.hpaName. Native HPA cannot scale to zero — use kind=keda for scale-to-zero / queue / event-driven. Fixed replica changes stay kind=scale. Cluster-wide HPA/rightsizing advice stays kind=optimize
- For rollback / undo rollout / revert: kind=rollback; set target.name to the Deployment; target.kind Deployment; target.namespace if mentioned; optional params.revision (number) to roll back to a specific revision (omit for previous revision)
- For install (helm chart): kind=install; set target.name to the app or release name (e.g. redis); target.namespace if mentioned; optional params.release, params.chart, params.repo, params.repo_url, params.replicas
- For upgrade (helm chart): kind=upgrade; set target.name to the release or app name (e.g. nginx); params.version or params.chart_version (chart version, e.g. "15.3.2" or "1.3"); target.namespace if mentioned; optional params.release, params.chart
- For deploy (kubernetes manifests): set target.name (workload name), params.image when known (e.g. redis:7-alpine, nginx:1.27-alpine); for well-known apps like "redis" or "nginx" name alone is enough; optional params.replicas (default 1), params.port and/or params.createService=true for a ClusterIP Service. Use deploy — not install — when the user says deploy
- For get/list/show: kind=get; set target.kind to any Kubernetes resource identity the user names — Kind (Pod, Deployment, Node, ConfigMap, Secret), plural (pods, nodes), short name (po, cm), or group-qualified (deployments.apps, widgets.example.com); target.namespace if mentioned (omit/ignore for cluster-scoped kinds like Node); target.name only for a single object; optional params.labelSelector; optional params.limit; optional params.timeout (e.g. "30s"); optional params.minMemory (e.g. "2Gi") when the user asks for pods using more than X memory (filter by memory requests). Secrets are normal readable resources — do not refuse or redact them in intent extraction.
- For explain (generic diagnosis without causal why): kind=explain; set target.name to the workload; target.kind Deployment or Pod; target.namespace if mentioned
- For why Pending/CrashLoop/OOM/ImagePull causal chain: kind=why; set target.name; target.kind Pod or Deployment; target.namespace if mentioned. Prefer why over explain for “why is X pending/crashing”
- For investigate / root cause / RCA / multi-hop diagnosis: kind=investigate; set target.name to the workload; target.kind Deployment or Pod; target.namespace if mentioned. Prefer investigate over explain when the user says investigate or asks for root cause across Service/Endpoints
- For timeline / chronology / what happened to X: kind=timeline; set target.name; target.kind Deployment or Pod; optional params.window (default 1h). Prefer timeline over explain/get for “timeline for X” or “what happened to X”
- For impact / who consumes / what depends on / blast radius of a live object: kind=impact; set target.name; target.kind Service or Deployment; target.namespace if mentioned. Prefer impact over get/graph/explain for “who consumes X”, “impact of service X”, or “what depends on deployment X”. Impact is read-only reverse dependencies — never emit mutate/delete for this kind
- For audit / security scan / hygiene check (root containers, privileged, latest tags, missing limits/ImagePullPolicy): kind=audit; omit target.name; params.scope=cluster for whole-cluster; target.namespace when a namespace is named. Prefer audit over get/explain/optimize for “audit X” or “security scan”. Audit is read-only — never emit patch/delete for this kind
- For cleanup / prune / find unused or stale resources (unused ConfigMaps/Secrets, completed Jobs, old ReplicaSets): kind=cleanup; omit target.name; params.scope=cluster for whole-cluster; target.namespace when a namespace is named. Prefer cleanup over get/delete for “cleanup X”, “prune X”, or “find unused …”. Cleanup intent is the scan only — never emit delete for this kind (optional approve-gated delete plans are produced later by suggest)
- For search / find every / which deployments use X / inventory query (structured match, not SQL): kind=search; set params.query to the match term (e.g. redis); optional target.kind Deployment|StatefulSet|DaemonSet|Pod|Service (default Deployment); optional params.match=image|env|label|name|annotation|all; params.scope=cluster for whole-cluster; target.namespace when a namespace is named. Prefer search over get/impact for “find every Deployment using redis” or “search for postgres”. Search is read-only — never emit mutate/delete for this kind. Do not use search for “find unused …” (that stays cleanup)
- For score / scorecard / health score (reliability + security + cost rollup): kind=score; omit target.name; params.scope=cluster for whole-cluster; target.namespace when a namespace is named; optional params.window. Prefer score over optimize/audit/roast for “score my cluster” or “scorecard”. Score is read-only — never emit mutate. Playful roast/vibe-check stays kind=roast
- For explain architecture / platform overview / what does this cluster look like (high-level narrative from learn + graph + heuristic deps): kind=architecture; omit target.name; params.scope=cluster for whole-cluster; target.namespace when a namespace is named. Prefer architecture over explain/graph/learn for “explain architecture” or “platform overview”. Architecture is read-only — never emit mutate. Service dependency graph alone stays kind=graph
- For learn / detect cluster tools / tool profile (Helm, Linkerd, Prometheus, Gateway API, cert-manager, Argo CD/Flux): kind=learn; omit target.name; target.kind Cluster. Prefer learn over get/tools for “learn cluster”, “detect tools”, or “tool profile”. Learn is read-only and persists a local profile — never mutate
- For drift / out of sync / cluster vs Git (Flux Kustomization or Argo CD Application): kind=drift; omit target.name unless a single app is named; params.scope=cluster for whole-cluster; params.engine=flux|argocd when named; target.namespace when a namespace is named. Prefer drift over gitops/get for “check drift”, “out of sync”, or “drift vs git”. Drift is read-only — never emit sync/mutate for this kind (optional approve-gated sync plans are produced later by suggest)
- For slow/performance/latency requests (e.g. "why is my api slow"): kind=performance; set target.name to the workload; target.kind Deployment; target.namespace if mentioned; optional params.window such as "15m" or "1h"
- For cluster optimize / rightsizing / idle workload asks (e.g. "optimize my cluster"): kind=optimize; omit target.name; set params.scope=cluster for whole-cluster; set target.namespace only when a namespace is named; optional params.window (default 1h). Optimize is read-only — never emit scale/patch/delete for this kind
- For playful health roast / vibe check / "how's my cluster" / "rate my namespace": kind=roast; omit target.name; params.scope=cluster for whole-cluster; target.namespace when a namespace is named. Prefer roast over get/optimize/explain for roast/vibe-check phrasing. Roast is read-only — never emit mutate/delete for this kind
- For service dependency graph asks (e.g. "show service dependency graph"): kind=graph; omit target.name; set params.scope=cluster unless a namespace is named; optional params.includeNetworkPolicy=true (default true). Graph is read-only
- For distributed tracing requests (e.g. "trace payment request"): kind=trace; set target.name to the service name (e.g. payment); target.kind Service; optional params.operation for an explicitly named span/route; optional params.window up to 24h
- For Grafana dashboard requests (e.g. "show dashboard" or "show payments dashboard"): kind=dashboard; set target.name to the dashboard search text when named; target.kind Dashboard; optional params.uid only when the user gives an explicit Grafana dashboard UID
- For logs / tail logs / show logs: kind=logs; set target.name (Pod or Deployment); optional target.kind; optional params.tail (lines, default 100); optional params.container
- For describe / status of / show details (not crash-focused): kind=describe; set target.name; target.kind Pod or Deployment
- For Argo Workflows / train a model / submit a workflow: kind=workflow; set target.name when the user names the workflow (otherwise omit and set params.model); target.namespace if mentioned; params.task (e.g. train, infer); params.model (e.g. yolov11); optional params.image, params.dataset, params.gpu=true, params.command, params.args
- For Tekton CI / create a CI pipeline / PipelineRun: kind=tekton; set target.name when named; target.kind PipelineRun; optional params.repo_url (git URL), params.image, params.task (ci/build/test). Requires approval to submit
- For KEDA / ScaledObject / event-driven / scale-to-zero with queue or redis: kind=keda; set target.name to the Deployment to scale; target.kind ScaledObject; optional params.trigger (cpu|redis|cron), params.minReplicas (0 for scale-to-zero), params.maxReplicas, params.queue / params.listName, params.address. Requires approval to create. Prefer keda over hpa when the user names KEDA, ScaledObject, queues, or scale-to-zero
- For Istio / VirtualService / canary / traffic split (read-first): kind=istio; set target.name when a VirtualService or host is named; target.kind VirtualService; target.namespace if mentioned. Do not emit mutate/delete for this kind
- For Crossplane / provision cloud / claim postgres|bucket|redis: kind=crossplane; set target.name for the claim; target.namespace if mentioned; params.resource (postgres|bucket|redis); optional params.apiVersion, params.claimKind, params.composition, params.provider, params.storageGB, params.secret. Cloud claims are high risk and require strong approval
- For GitOps Flux / Argo CD / Kustomization / Application sync or health: kind=gitops; set params.action (status|sync|promote|rollback); params.engine (flux|argocd|auto); target.name when syncing/promoting a named app; target.namespace if mentioned; target.kind Kustomization or Application. Status/health/show/list are read-only; sync/promote/rollback require approval. Do not use kind=gitops for plain Argo Workflow or plain "rollback yesterday's deployment" (those stay workflow / rollback)
- For delete / remove a single named resource: kind=delete; MUST set target.name and target.kind (Deployment, Service, Pod, Job, or ReplicaSet); target.namespace if mentioned. Never delete without a concrete name. Namespace deletes and wipe/all/cluster deletes use kind=deny
- For clearly destructive wipe/delete-cluster / delete-all / delete namespace requests: kind=deny
- Namespace from phrases: "in staging", "in the prod namespace", "in production" → set target.namespace (aliases: stage→staging, prod→prod, production→production, dev→dev)
- Kube context from phrases: "on kind-kprompt-e2e context", "using context docker-desktop", "with the prod-cluster context" → set top-level context (kubeconfig context name)
- Prefer Deployment as target.kind for named apps when unspecified
- Only emit JSON matching the schema`

// ExtractOptions optionally injects a learned cluster tool profile into the system prompt.
type ExtractOptions struct {
	ProfileHint string
}

// Extract uses an LLM provider to produce a structured Intent.
// Call ApplyScope afterward to merge CLI overrides, phrase heuristics, and defaults.
func Extract(ctx context.Context, provider llm.Provider, prompt string) (Intent, error) {
	return ExtractWith(ctx, provider, prompt, ExtractOptions{})
}

// ExtractWith is Extract plus optional profile bias (S-009).
func ExtractWith(ctx context.Context, provider llm.Provider, prompt string, opts ExtractOptions) (Intent, error) {
	schema := json.RawMessage(SchemaJSON)
	sys := systemPrompt
	if hint := strings.TrimSpace(opts.ProfileHint); hint != "" {
		sys = systemPrompt + "\n\n" + hint
	}
	raw, err := provider.CompleteStructured(ctx, llm.CompletionRequest{
		System: sys,
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
