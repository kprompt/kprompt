package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/kprompt/kprompt/internal/agent/analyze"
	"github.com/kprompt/kprompt/internal/agent/autopilot"
	"github.com/kprompt/kprompt/internal/agent/brief"
	"github.com/kprompt/kprompt/internal/agent/coordinator"
	"github.com/kprompt/kprompt/internal/agent/correlate"
	"github.com/kprompt/kprompt/internal/agent/crdstatus"
	"github.com/kprompt/kprompt/internal/agent/ctxbuild"
	"github.com/kprompt/kprompt/internal/agent/fleet"
	"github.com/kprompt/kprompt/internal/agent/handoff"
	"github.com/kprompt/kprompt/internal/agent/health"
	agentlogs "github.com/kprompt/kprompt/internal/agent/logs"
	"github.com/kprompt/kprompt/internal/agent/memory"
	agentdiscord "github.com/kprompt/kprompt/internal/agent/notify/discord"
	agentslack "github.com/kprompt/kprompt/internal/agent/notify/slack"
	"github.com/kprompt/kprompt/internal/agent/notify/slack/ask"
	agentwebhook "github.com/kprompt/kprompt/internal/agent/notify/webhook"
	"github.com/kprompt/kprompt/internal/agent/operator"
	"github.com/kprompt/kprompt/internal/agent/patterns"
	agentwatch "github.com/kprompt/kprompt/internal/agent/watch"
	"github.com/kprompt/kprompt/internal/cluster"
	"github.com/kprompt/kprompt/internal/config"
	"github.com/kprompt/kprompt/internal/graph"
	"github.com/kprompt/kprompt/internal/incident"
	"github.com/kprompt/kprompt/internal/llm"
	"github.com/kprompt/kprompt/internal/tools"
	"github.com/kprompt/kprompt/internal/ui"
	"k8s.io/client-go/kubernetes"
)

func newAgentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Observe agent and operator",
		Long:  "Namespace-scoped Observe Mode (`agent run`) and optional Operator that reconciles KpromptAgent CRs into agent Deployments. Observe agents never mutate workloads; the operator only manages agent lifecycle objects.",
	}
	cmd.AddCommand(newAgentRunCmd())
	cmd.AddCommand(newAgentListCmd())
	cmd.AddCommand(newAgentStatusCmd())
	cmd.AddCommand(newAgentOperatorCmd())
	cmd.AddCommand(newAgentCoordinatorCmd())
	cmd.AddCommand(newAgentAutopilotCmd())
	cmd.AddCommand(newAgentMemoryCmd())
	cmd.AddCommand(newAgentPatternsCmd())
	cmd.AddCommand(newAgentGraphCmd())
	return cmd
}

func newAgentStatusCmd() *cobra.Command {
	var (
		ns               string
		kubeCtx          string
		inCluster        bool
		incidentsBackend string
		incidentsDir     string
		patternsBackend  string
		patternsDir      string
		memoryBackend    string
		memoryDir        string
		asJSON           bool
	)
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Namespace Agent intelligence brief",
		Long: `Read-only rollup of health score, open incidents, learned patterns, and
memory deps for one namespace. Composes Incident Memory stores — does not mutate.

Richer continuous reasoning beyond this brief remains building.`,
		Example: `  kprompt agent status -n payments
  kprompt agent status -n payments --incidents-backend configmap --patterns-backend configmap --memory-backend configmap --in-cluster
  kprompt agent status -n payments --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ns = strings.TrimSpace(ns)
			if ns == "" {
				return fmt.Errorf("--namespace is required")
			}
			needKube := inCluster ||
				strings.EqualFold(incidentsBackend, "configmap") ||
				strings.EqualFold(patternsBackend, "configmap") ||
				strings.EqualFold(memoryBackend, "configmap")
			clients, err := connectOptional(kubeCtx, inCluster, needKube)
			if err != nil {
				return err
			}
			incBe := strings.TrimSpace(incidentsBackend)
			if incBe == "" {
				incBe = "file"
			}
			incStore, err := openIncidentsStore(incBe, incidentsDir, ns, inCluster, clients)
			if err != nil {
				return err
			}
			patBe := strings.TrimSpace(patternsBackend)
			if patBe == "" {
				patBe = "file"
			}
			patStore, err := openPatternsStore(patBe, patternsDir, ns, inCluster, clients)
			if err != nil {
				return err
			}
			memBe := strings.TrimSpace(memoryBackend)
			if memBe == "" {
				memBe = "file"
			}
			memStore, err := openMemoryStore(memBe, memoryDir, ns, inCluster, clients)
			if err != nil {
				return err
			}
			var cs kubernetes.Interface
			if clients != nil {
				cs = clients.Clientset
			}
			b, err := brief.Build(cmd.Context(), ns, brief.Inputs{
				Client:    cs,
				Incidents: incStore,
				Patterns:  patStore,
				Memory:    memStore,
			})
			if err != nil {
				return err
			}
			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(b)
			}
			fmt.Fprintln(cmd.OutOrStdout(), brief.Format(b))
			return nil
		},
	}
	cmd.Flags().StringVarP(&ns, "namespace", "n", "", "namespace (required)")
	cmd.Flags().StringVar(&kubeCtx, "context", "", "kubeconfig context")
	cmd.Flags().BoolVar(&inCluster, "in-cluster", false, "use in-cluster config")
	cmd.Flags().StringVar(&incidentsBackend, "incidents-backend", "file", "incidents store: file|configmap")
	cmd.Flags().StringVar(&incidentsDir, "incidents-dir", "", "file backend directory for incidents")
	cmd.Flags().StringVar(&patternsBackend, "patterns-backend", "file", "patterns store: file|configmap")
	cmd.Flags().StringVar(&patternsDir, "patterns-dir", "", "file backend directory for patterns")
	cmd.Flags().StringVar(&memoryBackend, "memory-backend", "file", "memory store: file|configmap")
	cmd.Flags().StringVar(&memoryDir, "memory-dir", "", "file backend directory for memory")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print NamespaceAgentBrief JSON")
	_ = cmd.MarkFlagRequired("namespace")
	return cmd
}

func newAgentListCmd() *cobra.Command {
	var (
		ns        string
		allNS     bool
		kubeCtx   string
		inCluster bool
		asJSON    bool
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List Namespace Agent / Observe surfaces",
		Long: `Read-only inventory of KpromptAgent CRs and labeled kprompt-agent Deployments.

Shows mode, watch namespace, Ready condition, health score / trend, and open
incidents when status is synced. Helm-only Deployments appear when no matching CR
covers them. Never mutates.`,
		Example: `  kprompt agent list -A
  kprompt agent list -n payments
  kprompt agent list -A --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			var clients *cluster.Clients
			var err error
			if inCluster {
				clients, err = cluster.ConnectInCluster()
			} else {
				clients, err = cluster.Connect(kubeCtx)
			}
			if err != nil {
				return err
			}
			dyn, err := cluster.DynamicForConfig(clients.Config)
			if err != nil {
				return err
			}
			scope := strings.TrimSpace(ns)
			if allNS {
				scope = ""
			} else if scope == "" {
				scope = "default"
			}
			inv, err := fleet.List(cmd.Context(), dyn, clients.Clientset, fleet.ListOptions{Namespace: scope})
			if err != nil {
				return err
			}
			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(inv)
			}
			fmt.Fprintln(cmd.OutOrStdout(), fleet.Format(inv))
			return nil
		},
	}
	cmd.Flags().StringVarP(&ns, "namespace", "n", "", "namespace scope (default: default; use -A for all)")
	cmd.Flags().BoolVarP(&allNS, "all-namespaces", "A", false, "list across all namespaces")
	cmd.Flags().StringVar(&kubeCtx, "context", "", "kubeconfig context")
	cmd.Flags().BoolVar(&inCluster, "in-cluster", false, "use in-cluster config")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print AgentFleet JSON")
	return cmd
}

func newAgentAutopilotCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "autopilot",
		Short: "Autopilot proposal apply bridge (gated)",
		Long:  "Human approve bridge for AutopilotProposal JSON. Apply requires --approve plus RemediationPolicy mode=policyAuto apply=true.",
	}
	cmd.AddCommand(newAgentAutopilotApplyCmd())
	return cmd
}

func newAgentAutopilotApplyCmd() *cobra.Command {
	var (
		file       string
		policyFile string
		kubeCtx    string
		inCluster  bool
		approve    bool
	)
	cmd := &cobra.Command{
		Use:   "apply-proposal",
		Short: "Apply an AutopilotProposal JSON under policyAuto",
		Long: `Load a saved AutopilotProposal, re-check RemediationPolicy + Safety, and mutate only when:
  --approve is set AND policy mode=policyAuto AND policy.apply=true.

Propose-only policies always deny. Never invents allowlist entries.`,
		Example: `  kprompt agent autopilot apply-proposal --file proposal.json --approve --policy ./policy-auto.json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !approve {
				return fmt.Errorf("refusing apply without --approve")
			}
			path := strings.TrimSpace(file)
			if path == "" {
				return fmt.Errorf("--file is required")
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			var prop autopilot.Proposal
			if err := json.Unmarshal(raw, &prop); err != nil {
				return fmt.Errorf("proposal JSON: %w", err)
			}
			var clients *cluster.Clients
			if inCluster {
				clients, err = cluster.ConnectInCluster()
			} else {
				clients, err = cluster.Connect(kubeCtx)
			}
			if err != nil {
				return err
			}
			pol, src, err := autopilot.LoadPolicy(cmd.Context(), policyFile, prop.Namespace, clients.Clientset)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "remediation policy source=%s mode=%s apply=%v\n", src, pol.Mode, pol.Apply)
			eng := &autopilot.Engine{
				Policy: pol,
				Audit:  autopilot.FileAudit{Dir: autopilot.DefaultAuditDir()},
			}
			out, err := eng.ApplyProposal(cmd.Context(), clients.Clientset, prop)
			if out != nil {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				_ = enc.Encode(out)
			}
			return err
		},
	}
	cmd.Flags().StringVar(&file, "file", "", "path to AutopilotProposal JSON")
	cmd.Flags().StringVar(&policyFile, "policy", "", "RemediationPolicy JSON (default: ConfigMap or built-in proposeOnly)")
	cmd.Flags().StringVar(&kubeCtx, "context", "", "kubeconfig context")
	cmd.Flags().BoolVar(&inCluster, "in-cluster", false, "use InClusterConfig")
	cmd.Flags().BoolVar(&approve, "approve", false, "required explicit approval to mutate")
	_ = cmd.MarkFlagRequired("file")
	return cmd
}

func newAgentCoordinatorCmd() *cobra.Command {
	var (
		addr               string
		probeKube          bool
		kubeCtx            string
		inCluster          bool
		knowledgeBackend   string // "" | file | configmap
		knowledgeDir       string
		knowledgeNamespace string
	)
	cmd := &cobra.Command{
		Use:   "coordinator",
		Short: "Thin Coordinator HTTP fan-in (no mutate)",
		Long: `Listen for Namespace Agent CoordinatorHandoff POSTs, merge InvestigationReports,
and reply with CoordinatorReply.

Never applies/patches/deletes workloads. Optional --probe-kube enables a
read-only Events/Pods probe of suspectNamespace. Default probe is a no-op.

Shared Knowledge: GET /v1/knowledge summarizes handoff edges.
Blast-radius MVP: GET /v1/blast-radius turns those edges into risk-ranked hops
(not a continuous mesh/OTel product graph). Optional --knowledge-backend file|configmap
persists the recent ring across restarts.

Endpoints:
  GET  /healthz
  POST /v1/handoff       — accept handoff.Envelope → CoordinatorReply
  GET  /v1/recent        — recent handoffs
  GET  /v1/knowledge     — Shared Knowledge summary (namespace edges)
  GET  /v1/blast-radius  — blast-radius hops (?namespace= filter)

Pair with: kprompt agent run … --coordinator-url http://<addr>/v1/handoff`,
		Example: `  kprompt agent coordinator --addr :9090
  kprompt agent coordinator --addr :9090 --probe-kube
  kprompt agent coordinator --addr :9090 --knowledge-backend configmap --in-cluster --knowledge-namespace kprompt-system
  kprompt agent coordinator knowledge --url http://127.0.0.1:9090
  kprompt agent coordinator blast-radius --url http://127.0.0.1:9090 -n payments`,
		RunE: func(cmd *cobra.Command, args []string) error {
			runCtx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			svc := coordinator.New()
			backend := strings.ToLower(strings.TrimSpace(knowledgeBackend))
			needKube := probeKube || backend == "configmap"
			var clients *cluster.Clients
			if needKube {
				var err error
				if inCluster {
					clients, err = cluster.ConnectInCluster()
				} else {
					clients, err = cluster.Connect(kubeCtx)
				}
				if err != nil {
					return fmt.Errorf("coordinator kube: %w", err)
				}
			}
			if probeKube {
				svc.Probe = &coordinator.KubeProbe{Client: clients.Clientset}
				fmt.Fprintf(cmd.ErrOrStderr(), "kprompt agent coordinator: kube probe enabled (read-only)\n")
			}
			switch backend {
			case "":
				// in-memory only
			case "file":
				dir := strings.TrimSpace(knowledgeDir)
				if dir == "" {
					home, _ := os.UserHomeDir()
					dir = filepath.Join(home, ".config", "kprompt", "coordinator")
				}
				path := filepath.Join(dir, "handoffs.json")
				svc.Store = coordinator.FileStore{Path: path}
				fmt.Fprintf(cmd.ErrOrStderr(), "kprompt agent coordinator: knowledge store file %s\n", path)
			case "configmap":
				ns := strings.TrimSpace(knowledgeNamespace)
				if ns == "" {
					ns = strings.TrimSpace(os.Getenv("POD_NAMESPACE"))
				}
				if ns == "" {
					return fmt.Errorf("coordinator --knowledge-backend configmap requires --knowledge-namespace or POD_NAMESPACE")
				}
				svc.Store = coordinator.ConfigMapStore{Client: clients.Clientset, Namespace: ns}
				fmt.Fprintf(cmd.ErrOrStderr(), "kprompt agent coordinator: knowledge store ConfigMap %s/%s\n", ns, coordinator.ConfigMapName)
			default:
				return fmt.Errorf("coordinator --knowledge-backend: want file|configmap, got %q", knowledgeBackend)
			}
			if svc.Store != nil {
				svc.PersistErrLog = func(err error) {
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: persist knowledge: %v\n", err)
				}
				if err := svc.Restore(); err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: restore knowledge: %v\n", err)
				} else if n := len(svc.Recent()); n > 0 {
					fmt.Fprintf(cmd.ErrOrStderr(), "kprompt agent coordinator: restored %d handoff(s)\n", n)
				}
			}
			h := &coordinator.Handler{
				Service: svc,
				Logf: func(format string, a ...any) {
					fmt.Fprintf(cmd.ErrOrStderr(), format+"\n", a...)
				},
			}
			listen := strings.TrimSpace(addr)
			if listen == "" {
				listen = ":9090"
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "kprompt agent coordinator listening on %s (mutate=off)…\n", listen)
			err := coordinator.ListenAndServe(runCtx, listen, h)
			if err == context.Canceled || err == context.DeadlineExceeded {
				return nil
			}
			return err
		},
	}
	cmd.Flags().StringVar(&addr, "addr", ":9090", "listen address for Coordinator HTTP API")
	cmd.Flags().BoolVar(&probeKube, "probe-kube", false, "read-only Pods/Events probe of suspectNamespace")
	cmd.Flags().StringVar(&kubeCtx, "context", "", "kubeconfig context for --probe-kube / configmap knowledge")
	cmd.Flags().BoolVar(&inCluster, "in-cluster", false, "use in-cluster config for --probe-kube / configmap knowledge")
	cmd.Flags().StringVar(&knowledgeBackend, "knowledge-backend", "", "persist Shared Knowledge: file|configmap")
	cmd.Flags().StringVar(&knowledgeDir, "knowledge-dir", "", "file backend directory (default ~/.config/kprompt/coordinator)")
	cmd.Flags().StringVar(&knowledgeNamespace, "knowledge-namespace", "", "ConfigMap namespace (default POD_NAMESPACE)")
	cmd.AddCommand(newAgentCoordinatorKnowledgeCmd())
	cmd.AddCommand(newAgentCoordinatorBlastRadiusCmd())
	cmd.AddCommand(newAgentCoordinatorRecentCmd())
	return cmd
}

func newAgentCoordinatorKnowledgeCmd() *cobra.Command {
	var (
		baseURL string
		asJSON  bool
	)
	cmd := &cobra.Command{
		Use:   "knowledge",
		Short: "Fetch Coordinator Shared Knowledge MVP",
		Long: `GET /v1/knowledge from a running Coordinator — namespace edges derived from
recent handoffs. Pair with blast-radius for risk-ranked hops.`,
		Example: `  kprompt agent coordinator knowledge --url http://127.0.0.1:9090`,
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := fetchCoordinatorJSON(cmd.Context(), baseURL, "/v1/knowledge")
			if err != nil {
				return err
			}
			if asJSON {
				_, err = cmd.OutOrStdout().Write(append(body, '\n'))
				return err
			}
			var sum coordinator.KnowledgeSummary
			if err := json.Unmarshal(body, &sum); err != nil {
				return fmt.Errorf("decode knowledge: %w", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), coordinator.FormatKnowledge(sum))
			return nil
		},
	}
	cmd.Flags().StringVar(&baseURL, "url", "http://127.0.0.1:9090", "Coordinator base URL (no path)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print raw JSON")
	return cmd
}

func newAgentCoordinatorBlastRadiusCmd() *cobra.Command {
	var (
		baseURL string
		ns      string
		asJSON  bool
	)
	cmd := &cobra.Command{
		Use:   "blast-radius",
		Short: "Fetch Coordinator blast-radius MVP",
		Long: `GET /v1/blast-radius — risk-ranked cross-namespace hops from Shared Knowledge
handoffs. Not a continuous mesh/OTel product graph.`,
		Example: `  kprompt agent coordinator blast-radius --url http://127.0.0.1:9090
  kprompt agent coordinator blast-radius --url http://127.0.0.1:9090 -n payments`,
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "/v1/blast-radius"
			if ns = strings.TrimSpace(ns); ns != "" {
				path += "?namespace=" + ns
			}
			body, err := fetchCoordinatorJSON(cmd.Context(), baseURL, path)
			if err != nil {
				return err
			}
			if asJSON {
				_, err = cmd.OutOrStdout().Write(append(body, '\n'))
				return err
			}
			var rep coordinator.BlastRadiusReport
			if err := json.Unmarshal(body, &rep); err != nil {
				return fmt.Errorf("decode blast-radius: %w", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), coordinator.FormatBlastRadius(rep))
			return nil
		},
	}
	cmd.Flags().StringVar(&baseURL, "url", "http://127.0.0.1:9090", "Coordinator base URL (no path)")
	cmd.Flags().StringVarP(&ns, "namespace", "n", "", "focus namespace (optional filter)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print raw JSON")
	return cmd
}

func newAgentCoordinatorRecentCmd() *cobra.Command {
	var baseURL string
	cmd := &cobra.Command{
		Use:     "recent",
		Short:   "Fetch recent Coordinator handoffs",
		Long:    `GET /v1/recent from a running Coordinator — raw in-memory handoff ring.`,
		Example: `  kprompt agent coordinator recent --url http://127.0.0.1:9090`,
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := fetchCoordinatorJSON(cmd.Context(), baseURL, "/v1/recent")
			if err != nil {
				return err
			}
			_, err = cmd.OutOrStdout().Write(append(body, '\n'))
			return err
		},
	}
	cmd.Flags().StringVar(&baseURL, "url", "http://127.0.0.1:9090", "Coordinator base URL (no path)")
	return cmd
}

func fetchCoordinatorJSON(ctx context.Context, baseURL, path string) ([]byte, error) {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		return nil, fmt.Errorf("coordinator url is required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("coordinator %s: HTTP %d: %s", path, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return body, nil
}

func newAgentOperatorCmd() *cobra.Command {
	var (
		kubeCtx   string
		inCluster bool
		ns        string
		once      bool
		image     string
	)
	cmd := &cobra.Command{
		Use:   "operator",
		Short: "Reconcile KpromptAgent CRs into Observe agent Deployments",
		Long: `Watch KpromptAgent custom resources and ensure ServiceAccount, Role,
RoleBinding, and Deployment exist for Observe Mode.

Never enables Autopilot. Rejects non-Observe modes. V1 requires the CR
namespace to equal the watch namespace (spec.namespace empty or same).`,
		Example: `  kprompt agent operator --once -n payments
  kprompt agent operator --in-cluster`,
		RunE: func(cmd *cobra.Command, args []string) error {
			var clients *cluster.Clients
			var err error
			if inCluster {
				clients, err = cluster.ConnectInCluster()
			} else {
				clients, err = cluster.Connect(kubeCtx)
			}
			if err != nil {
				return err
			}
			dyn, err := cluster.DynamicForConfig(clients.Config)
			if err != nil {
				return err
			}
			rec := &operator.Reconciler{
				Kube:    clients.Clientset,
				Dynamic: dyn,
				Options: operator.Options{DefaultImage: strings.TrimSpace(image)},
			}
			ctrl := &operator.Controller{
				Reconciler: rec,
				Namespace:  strings.TrimSpace(ns),
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "kprompt agent operator watching KpromptAgent (ns=%q)…\n", ns)
			if once {
				return ctrl.ReconcileAll(cmd.Context())
			}
			runCtx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			if err := ctrl.ReconcileAll(runCtx); err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "initial reconcile: %v\n", err)
			}
			return ctrl.Run(runCtx)
		},
	}
	cmd.Flags().StringVar(&kubeCtx, "context", "", "kubeconfig context (ignored with --in-cluster)")
	cmd.Flags().BoolVar(&inCluster, "in-cluster", false, "use InClusterConfig (ServiceAccount)")
	cmd.Flags().StringVarP(&ns, "namespace", "n", "", "limit to one namespace (empty = all)")
	cmd.Flags().BoolVar(&once, "once", false, "reconcile current CRs once and exit")
	cmd.Flags().StringVar(&image, "default-image", "", "default agent image repo:tag (default ghcr.io/kprompt/kprompt:latest)")
	return cmd
}

func newAgentRunCmd() *cobra.Command {
	var (
		ns               string
		kubeCtx          string
		inCluster        bool
		emitJSON         bool
		emitInitial      bool
		incidents        bool
		fetchLogs        bool
		buildContext     bool
		doAnalyze        bool
		heuristic        bool
		notifyDiscord    bool
		discordURL       string
		notifySlack      bool
		notifyWebhook    bool
		webhookURL       string
		trackHealth      bool
		providerName     string
		modelName        string
		minSeverity      string
		minConfidence    float64
		duration         time.Duration
		agentCR          string
		agentCRNS        string
		watchList        []string
		useMemory        bool
		memoryBackend    string // file | configmap
		memoryDir        string
		usePatterns      bool
		patternsDir      string
		patternsBackend  string // file | configmap
		autopilotProp    bool
		autopilotDir     string
		autopilotPolicy  string
		autopilotApply   bool
		incidentsBackend string // file | configmap | "" (off)
		incidentsDir     string
		slackAsk         bool
		slackAskAddr     string
		coordinatorURL   string
		gitopsEvidence   bool
	)
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Watch Pods and Events in a namespace (Observe Mode)",
		Long: `Start the Observe watch engine for one namespace.

Watched resources (read-only):
  --watch          comma-separated: pods,events (default) plus deployments,
                   replicasets,statefulsets,jobs,cronjobs,pvc,configmaps,secrets.
                   secrets are opt-in and metadata-only (never values).

Pipeline flags (read-only — never mutate workload objects):
  --incidents      correlate problem signals into Incidents
  --fetch-logs     on-demand log tail on CrashLoop/Failed/OOM
  --build-context  assemble AgentContext
  --analyze        LLM/heuristic → gated AgentAlert
  --discord        post gated alerts to Discord webhook
  --slack          post gated alerts to Slack threads
  --webhook        POST gated AgentAlert JSON to a URL
  --health         emit namespace health score / risk_increasing
  --agent-cr       patch KpromptAgent.status (health + lastAlert)
  --memory         discover/load namespace deps+facts into analyzer context
  --patterns       learn incident signatures; boost confidence on “seen before”
  --patterns-backend file|configmap   pattern store (default file)
  --autopilot-propose  emit PlanResult-shaped AutopilotProposal (propose-only by default)
  --autopilot-policy   RemediationPolicy JSON file; else ConfigMap / defaults
  --autopilot-apply    with policyAuto+apply=true, apply approved proposals in-loop (off by default)
  --slack-ask          listen for Slack Events ask (status/why/what broke/false positive) — read-only
  --coordinator-url    POST CoordinatorHandoff when cross-ns suspicion (opt-in)
  --gitops-evidence    attach Argo/Flux sync + deploy history as EvidenceRefs (opt-in)

Durable incidents (local / in-cluster only):
  --incidents-backend file|configmap   persist open incidents across restarts
  --incidents-dir     file backend directory (default: ~/.config/kprompt/incidents)

Namespace memory (local / in-cluster only — never uploaded to api.kprompt.ai):
  --memory-backend file|configmap   (default: file; configmap uses kprompt-namespace-memory)
  --memory-dir     file backend directory (default: ~/.config/kprompt/memory)

Pattern learning (local only — never mutates from a match):
  --patterns-dir   pattern store directory (default: ~/.config/kprompt/patterns)

Autopilot (ADR-0015 MVP — propose-only by default):
  --autopilot-audit-dir  audit JSONL directory (default: ~/.config/kprompt/autopilot)

Slack credentials from env / mounted Secret:
  KPROMPT_SLACK_BOT_TOKEN + KPROMPT_SLACK_CHANNEL  (preferred, threaded)
  KPROMPT_SLACK_WEBHOOK_URL                        (fallback)

Generic webhook:
  KPROMPT_WEBHOOK_URL  or  --webhook-url

Discord:
  KPROMPT_DISCORD_WEBHOOK_URL  or  --discord-webhook-url

KpromptAgent status sync:
  --agent-cr / KPROMPT_AGENT_CR  (+ optional --agent-cr-namespace)`,
		Example: `  kprompt agent run -n payments --health --heuristic
  kprompt agent run -n payments --analyze --fetch-logs --health --heuristic
  kprompt agent run -n payments --coordinator-url http://127.0.0.1:9090/v1/handoff --health --heuristic`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ns = strings.TrimSpace(ns)
			if ns == "" {
				return fmt.Errorf("--namespace is required")
			}
			if slackAsk {
				notifySlack = true
				incidents = true
				trackHealth = true
				doAnalyze = true
			}
			if notifyDiscord || notifySlack || notifyWebhook {
				doAnalyze = true
			}
			if trackHealth && !incidents {
				incidents = true
			}
			if doAnalyze {
				buildContext = true
			}
			if (fetchLogs || buildContext || doAnalyze) && !incidents {
				incidents = true
			}
			if useMemory && !buildContext && !doAnalyze {
				buildContext = true
				incidents = true
			}
			if usePatterns {
				doAnalyze = true
				buildContext = true
				incidents = true
			}
			if autopilotProp {
				doAnalyze = true
				buildContext = true
				incidents = true
			}

			var clients *cluster.Clients
			var err error
			if inCluster {
				clients, err = cluster.ConnectInCluster()
			} else {
				clients, err = cluster.Connect(kubeCtx)
			}
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			var builder *correlate.Builder
			var fetcher *agentlogs.Fetcher
			var ctxBuilder *ctxbuild.Builder
			var analyzer *analyze.Analyzer
			var discordClient *agentdiscord.Client
			var slackClient *agentslack.Client
			var webhookClient *agentwebhook.Client
			var healthTracker *health.Tracker
			var statusSync *crdstatus.Syncer
			var nsMemory *memory.Memory
			var memoryFacts []memory.Fact
			var apEngine *autopilot.Engine
			var handoffClient handoff.Client
			threads := map[string]string{}

			if u := strings.TrimSpace(coordinatorURL); u != "" {
				handoffClient = &handoff.HTTPClient{URL: u}
			}

			if useMemory {
				store, serr := openMemoryStore(memoryBackend, memoryDir, ns, inCluster, clients)
				if serr != nil {
					return serr
				}
				nsMemory = memory.New(store)
				facts, derr := memory.Discover(cmd.Context(), clients.Clientset, ns)
				if derr != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: memory discover: %v\n", derr)
				} else if len(facts) > 0 {
					if _, uerr := nsMemory.Upsert(ns, facts...); uerr != nil {
						fmt.Fprintf(cmd.ErrOrStderr(), "warning: memory upsert: %v\n", uerr)
					}
				}
				if snap, lerr := nsMemory.List(ns); lerr == nil {
					memoryFacts = snap.Facts
				}
			}
			if autopilotProp {
				dir := strings.TrimSpace(autopilotDir)
				if dir == "" {
					dir = autopilot.DefaultAuditDir()
				}
				pol, src, perr := autopilot.LoadPolicy(cmd.Context(), autopilotPolicy, ns, clients.Clientset)
				if perr != nil {
					return fmt.Errorf("autopilot policy: %w", perr)
				}
				fmt.Fprintf(cmd.ErrOrStderr(), "autopilot policy source=%s mode=%s apply=%v\n", src, pol.Mode, pol.Apply)
				if autopilotApply && !pol.PolicyAuto() {
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: --autopilot-apply ignored unless RemediationPolicy mode=policyAuto apply=true\n")
				}
				apEngine = &autopilot.Engine{
					Policy: pol,
					Audit:  autopilot.FileAudit{Dir: dir},
				}
			}
			crCfg := crdstatus.FromEnv()
			if n := strings.TrimSpace(agentCR); n != "" {
				crCfg.Name = n
			}
			if n := strings.TrimSpace(agentCRNS); n != "" {
				crCfg.Namespace = n
			}
			if crCfg.Name != "" {
				dyn, derr := cluster.DynamicForConfig(clients.Config)
				if derr != nil {
					return fmt.Errorf("dynamic client for KpromptAgent status: %w", derr)
				}
				statusSync = crdstatus.New(dyn, crCfg)
			}

			if incidents {
				builder = correlate.NewBuilder(correlate.Options{Namespace: ns})
				if be := strings.TrimSpace(incidentsBackend); be != "" {
					store, serr := openIncidentsStore(be, incidentsDir, ns, inCluster, clients)
					if serr != nil {
						return serr
					}
					builder.SetStore(store)
					if snap, lerr := store.Load(ns); lerr == nil {
						if rerr := builder.Restore(snap); rerr != nil {
							fmt.Fprintf(cmd.ErrOrStderr(), "warning: restore incidents: %v\n", rerr)
						} else {
							fmt.Fprintf(cmd.ErrOrStderr(), "restored %d open incident(s) from %s store\n", len(builder.OpenIncidents()), be)
						}
					}
				}
			}
			if fetchLogs {
				fetcher = agentlogs.New(clients.Clientset)
			}
			if buildContext || doAnalyze {
				ctxBuilder = &ctxbuild.Builder{Client: clients.Clientset}
				file, ferr := config.LoadFile()
				if ferr == nil {
					settings := tools.LoadSettings(file)
					if prom, perr := tools.NewPrometheusClient(settings); perr == nil {
						ctxBuilder.Metrics = prom
					}
					if otel, oerr := tools.NewOTelClient(settings); oerr == nil {
						ctxBuilder.Traces = otel
					}
				}
				if gitopsEvidence {
					dyn, derr := cluster.DynamicForConfig(clients.Config)
					if derr != nil {
						fmt.Fprintf(cmd.ErrOrStderr(), "warning: gitops evidence: dynamic client: %v\n", derr)
					} else {
						ctxBuilder.GitOps = &ctxbuild.ClusterGitOps{Config: clients.Config, Dynamic: dyn}
					}
				}
			}
			if trackHealth {
				healthTracker = health.NewTracker(ns, clients.Clientset)
			}
			if doAnalyze {
				opts := analyze.Options{
					MinSeverity:   minSeverity,
					MinConfidence: minConfidence,
					HeuristicOnly: heuristic,
				}
				var provider llm.Provider
				if !heuristic {
					file, err := config.LoadFile()
					if err != nil {
						return err
					}
					cfg := config.Merge(file, providerName, modelName, "", ns, false, "")
					provider, err = llm.New(cfg.Provider, config.APIKeyFor(cfg.Provider), cfg.BaseURL, cfg.Model)
					if err != nil {
						fmt.Fprintf(cmd.ErrOrStderr(), "warning: LLM unavailable (%v); using heuristic analyzer\n", err)
						opts.HeuristicOnly = true
					}
				}
				analyzer = analyze.New(provider, opts)
				if usePatterns {
					pstore, perr := openPatternsStore(patternsBackend, patternsDir, ns, inCluster, clients)
					if perr != nil {
						return perr
					}
					analyzer.Patterns = patterns.New(pstore)
				}
			}
			if notifyDiscord {
				dcfg := agentdiscord.ConfigFromEnv()
				if u := strings.TrimSpace(discordURL); u != "" {
					dcfg.URL = u
				}
				if !dcfg.Enabled() {
					return fmt.Errorf("--discord requires %s or --discord-webhook-url", agentdiscord.EnvWebhookURL)
				}
				discordClient = agentdiscord.New(dcfg)
			}
			if notifySlack {
				scfg := agentslack.ConfigFromEnv()
				if !scfg.Enabled() {
					return fmt.Errorf("--slack requires %s or %s+%s in the environment (mount from Secret)",
						agentslack.EnvWebhookURL, agentslack.EnvBotToken, agentslack.EnvChannel)
				}
				slackClient = agentslack.New(scfg)
				if !scfg.Threaded() {
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: Slack webhook mode cannot reliably thread; prefer bot token + channel\n")
				}
			}
			if notifyWebhook {
				wcfg := agentwebhook.ConfigFromEnv()
				if u := strings.TrimSpace(webhookURL); u != "" {
					wcfg.URL = u
				}
				if !wcfg.Enabled() {
					return fmt.Errorf("--webhook requires %s or --webhook-url", agentwebhook.EnvURL)
				}
				webhookClient = agentwebhook.New(wcfg)
			}

			currentMemoryFacts := func() []memory.Fact {
				if nsMemory == nil {
					return memoryFacts
				}
				snap, err := nsMemory.List(ns)
				if err != nil {
					return memoryFacts
				}
				memoryFacts = snap.Facts
				return memoryFacts
			}

			emitHealth := func() {
				if healthTracker == nil || builder == nil {
					return
				}
				snap := healthTracker.Evaluate(cmd.Context(), builder.OpenIncidents())
				if statusSync != nil {
					if err := statusSync.PatchHealth(cmd.Context(), snap); err != nil {
						fmt.Fprintf(cmd.ErrOrStderr(), "kpromptagent status health: %v\n", err)
					}
				}
				if emitJSON {
					_ = json.NewEncoder(out).Encode(snap)
					return
				}
				fmt.Fprintf(out, "health score=%d/100 trend=%s open=%d ready=%s restarts=%d %s\n",
					snap.Score, snap.Trend, snap.OpenIncidents, snap.PodReady, snap.Restarts, snap.Message)
			}

			emitChange := func(ch correlate.Change) {
				if analyzer != nil && ctxBuilder != nil {
					switch ch.Kind {
					case correlate.ChangeOpened, correlate.ChangeUpdated, correlate.ChangeReopened, correlate.ChangeClosed:
						agentCtx := ctxBuilder.Build(cmd.Context(), ch.Incident, ctxbuild.Options{Memory: currentMemoryFacts()})
						outcome, err := analyzer.Analyze(cmd.Context(), agentCtx, analyze.AlertStatusFor(ch.Kind))
						if err != nil {
							fmt.Fprintf(cmd.ErrOrStderr(), "analyze error: %v\n", err)
							return
						}
						if outcome.Skipped {
							return
						}
						if slackClient != nil && outcome.PassedGate {
							thread := threads[outcome.Alert.IncidentID]
							if thread == "" && builder != nil {
								thread = builder.NotifierThread(outcome.Alert.IncidentID)
							}
							if thread == "" {
								thread = ch.Incident.NotifierThread
							}
							res, err := slackClient.Notify(cmd.Context(), outcome.Alert, thread)
							if err != nil {
								fmt.Fprintf(cmd.ErrOrStderr(), "slack notify error: %v\n", err)
							} else if res.ThreadTS != "" {
								threads[outcome.Alert.IncidentID] = res.ThreadTS
								if builder != nil {
									_ = builder.SetNotifierThread(outcome.Alert.IncidentID, res.ThreadTS)
									_ = builder.Persist()
								}
								ch.Incident.NotifierThread = res.ThreadTS
							}
						}
						if webhookClient != nil && outcome.PassedGate {
							if err := webhookClient.Notify(cmd.Context(), outcome.Alert); err != nil {
								fmt.Fprintf(cmd.ErrOrStderr(), "webhook notify error: %v\n", err)
							}
						}
						if discordClient != nil && outcome.PassedGate {
							if err := discordClient.Notify(cmd.Context(), outcome.Alert); err != nil {
								fmt.Fprintf(cmd.ErrOrStderr(), "discord notify error: %v\n", err)
							}
						}
						if statusSync != nil && outcome.PassedGate {
							if err := statusSync.PatchAlert(cmd.Context(), outcome.Alert); err != nil {
								fmt.Fprintf(cmd.ErrOrStderr(), "kpromptagent status alert: %v\n", err)
							}
						}
						if apEngine != nil && !outcome.Skipped {
							agentCtx.Incident.Confidence = outcome.Alert.Confidence
							agentCtx.Incident.RootCause = outcome.Alert.RootCause
							agentCtx.Incident.Summary = outcome.Alert.Summary
							prop, perr := apEngine.ProposeFromContext(agentCtx, outcome.Alert.Confidence)
							if perr != nil {
								fmt.Fprintf(cmd.ErrOrStderr(), "autopilot propose error: %v\n", perr)
							} else if prop != nil {
								if autopilotApply && apEngine.Policy.PolicyAuto() &&
									(prop.Decision == autopilot.DecisionProposed || prop.Decision == autopilot.DecisionApproved) {
									applied, aerr := apEngine.ApplyProposal(cmd.Context(), clients.Clientset, *prop)
									if aerr != nil {
										fmt.Fprintf(cmd.ErrOrStderr(), "autopilot apply error: %v\n", aerr)
									}
									if applied != nil {
										prop = applied
									}
								}
								if emitJSON {
									_ = json.NewEncoder(out).Encode(prop)
								} else {
									fmt.Fprintf(out, "autopilot %s action=%s target=%s/%s risk=%s applied=%v — %s\n",
										prop.Decision, prop.ActionID, prop.TargetKind, prop.TargetName, prop.Risk, prop.Applied, prop.Reason)
								}
							}
						}
						if handoffClient != nil && !outcome.Skipped {
							if suspect, reason, ok := handoff.NeedsHandoff(ns, outcome.Report); ok {
								env := handoff.New(ns, suspect, reason, outcome.Report)
								reply, herr := handoffClient.Handoff(cmd.Context(), env)
								if herr != nil {
									fmt.Fprintf(cmd.ErrOrStderr(), "coordinator handoff: %v\n", herr)
								} else {
									if !emitJSON {
										if reply != nil && reply.Merged.Summary != "" {
											fmt.Fprintf(out, "handoff from=%s suspect=%s conf=%.2f routing=%v summary=%s\n",
												ns, reply.SuspectNamespace, reply.Merged.Confidence, reply.Routing, reply.Merged.Summary)
										} else {
											fmt.Fprintf(out, "handoff from=%s suspect=%s reason=%q\n", ns, suspect, reason)
										}
									}
									// AG-053: surface CoordinatorReply on Slack thread / webhook.
									if reply != nil {
										if slackClient != nil && slackClient.Threaded() {
											thread := threads[outcome.Alert.IncidentID]
											if thread == "" && builder != nil {
												thread = builder.NotifierThread(outcome.Alert.IncidentID)
											}
											if thread == "" {
												thread = ch.Incident.NotifierThread
											}
											if text := handoff.FormatReply(reply); text != "" {
												if _, serr := slackClient.PostText(cmd.Context(), text, thread); serr != nil {
													fmt.Fprintf(cmd.ErrOrStderr(), "slack coordinator reply: %v\n", serr)
												}
											}
										}
										if webhookClient != nil {
											if werr := webhookClient.NotifyJSON(cmd.Context(), reply); werr != nil {
												fmt.Fprintf(cmd.ErrOrStderr(), "webhook coordinator reply: %v\n", werr)
											}
										}
									}
								}
							}
						}
						if emitJSON {
							_ = json.NewEncoder(out).Encode(outcome)
							emitHealth()
							return
						}
						gate := "held"
						if outcome.PassedGate {
							gate = "alert"
						}
						extra := ""
						if ts := threads[outcome.Alert.IncidentID]; ts != "" {
							extra = " thread=" + ts
						}
						if outcome.SeenBefore != "" {
							extra += " seenBefore=" + strconv.Itoa(outcome.PatternHits)
						}
						fmt.Fprintf(out, "%s [%s/%s] id=%s severity=%s conf=%.2f summary=%s rootCause=%s%s\n",
							gate, outcome.Source, outcome.Alert.Status, outcome.Alert.IncidentID,
							outcome.Alert.Severity, outcome.Alert.Confidence, outcome.Alert.Summary, outcome.Alert.RootCause, extra)
						emitHealth()
						return
					}
				}
				if ctxBuilder != nil && analyzer == nil {
					switch ch.Kind {
					case correlate.ChangeOpened, correlate.ChangeUpdated, correlate.ChangeReopened:
						agentCtx := ctxBuilder.Build(cmd.Context(), ch.Incident, ctxbuild.Options{Memory: currentMemoryFacts()})
						if emitJSON {
							_ = json.NewEncoder(out).Encode(agentCtx)
							emitHealth()
							return
						}
						fmt.Fprintf(out, "context id=%s target=%v degraded=%v\n",
							agentCtx.Incident.ID, agentCtx.Target, agentCtx.Degraded)
						for _, line := range agentCtx.PromptBlocks() {
							fmt.Fprintf(out, "  %s\n", line)
						}
						emitHealth()
						return
					}
				}
				if emitJSON {
					_ = json.NewEncoder(out).Encode(ch)
					emitHealth()
					return
				}
				fmt.Fprintf(out, "incident %s id=%s severity=%s status=%s summary=%s evidence=%d\n",
					ch.Kind, ch.Incident.ID, ch.Incident.Severity, ch.Incident.Status,
					ch.Incident.Summary, len(ch.Incident.Evidence))
				emitHealth()
			}

			handler := func(ev agentwatch.Event) {
				if builder != nil {
					if ch, ok := builder.Ingest(ev); ok {
						if fetcher != nil {
							switch ch.Kind {
							case correlate.ChangeOpened, correlate.ChangeUpdated, correlate.ChangeReopened:
								inc := ch.Incident
								fetcher.Attach(cmd.Context(), &inc, ev)
								if snap, synced := builder.SyncIncident(inc); synced {
									ch.Incident = snap
								} else {
									ch.Incident = inc
								}
							}
						}
						if err := builder.Persist(); err != nil {
							fmt.Fprintf(cmd.ErrOrStderr(), "warning: persist incidents: %v\n", err)
						}
						emitChange(ch)
					}
					return
				}
				if emitJSON {
					_ = json.NewEncoder(out).Encode(ev)
					return
				}
				switch ev.Resource {
				case agentwatch.ResourceEvent:
					fmt.Fprintf(out, "%s Event %s/%s reason=%s involved=%s/%s %s\n",
						ev.Type, ev.Namespace, ev.Name, ev.Reason, ev.InvolvedKind, ev.InvolvedName, ev.Message)
				case agentwatch.ResourcePod:
					fmt.Fprintf(out, "%s %s %s/%s phase=%s\n",
						ev.Type, ev.Resource, ev.Namespace, ev.Name, ev.PodPhase)
				default:
					extra := ev.Detail
					if ev.PodPhase != "" {
						extra = "phase=" + ev.PodPhase + " " + extra
					}
					fmt.Fprintf(out, "%s %s %s/%s %s\n",
						ev.Type, ev.Resource, ev.Namespace, ev.Name, strings.TrimSpace(extra))
				}
			}

			resources := agentwatch.NormalizeResources(watchList)
			eng := &agentwatch.Engine{
				Client: clients.Clientset,
				Options: agentwatch.Options{
					Namespace:   ns,
					Resources:   resources,
					EmitInitial: emitInitial,
				},
				Handler: handler,
			}

			runCtx := cmd.Context()
			if duration > 0 {
				var cancel context.CancelFunc
				runCtx, cancel = context.WithTimeout(runCtx, duration)
				defer cancel()
			} else {
				var stop context.CancelFunc
				runCtx, stop = signal.NotifyContext(runCtx, os.Interrupt, syscall.SIGTERM)
				defer stop()
			}

			mode := "Pods+Events"
			switch {
			case notifyDiscord || notifySlack || notifyWebhook:
				mode = "watch → analyze → notify"
			case doAnalyze:
				mode = "watch → incidents → context → analyze"
			case buildContext:
				mode = "watch → incidents → context"
			case trackHealth:
				mode = "watch → incidents → health"
			case incidents && fetchLogs:
				mode = "Pods+Events → Incidents + on-demand logs"
			case incidents:
				mode = "Pods+Events → Incidents"
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "kprompt agent watching namespace %q resources=%s (%s, read-only)…\n",
				ns, strings.Join(resources, ","), mode)

			if slackAsk && slackClient != nil && builder != nil {
				askHandler := &ask.Handler{
					OpenIncidents: builder.OpenIncidents,
					Health: func(ctx context.Context) *health.Snapshot {
						if healthTracker == nil {
							return nil
						}
						snap := healthTracker.Evaluate(ctx, builder.OpenIncidents())
						return &snap
					},
				}
				if ctxBuilder != nil {
					askHandler.Why = ask.WhyFromHeuristic(ctxBuilder)
				}
				if analyzer != nil && analyzer.Patterns != nil && ctxBuilder != nil {
					askHandler.MarkFalsePositive = func(ctx context.Context, inc incident.Incident) error {
						agentCtx := ctxBuilder.Build(ctx, inc, ctxbuild.Options{Memory: currentMemoryFacts()})
						_, err := analyzer.Patterns.RecordOutcome(ns, agentCtx, patterns.OutcomeFalsePositive)
						return err
					}
				}
				addr := strings.TrimSpace(slackAskAddr)
				if addr == "" {
					addr = ":8080"
				}
				go func() {
					if err := ask.ListenAndServe(runCtx, ask.ListenConfig{
						Addr:   addr,
						Client: slackClient,
						Ask:    askHandler,
						Logf: func(format string, args ...any) {
							fmt.Fprintf(cmd.ErrOrStderr(), format+"\n", args...)
						},
					}); err != nil {
						fmt.Fprintf(cmd.ErrOrStderr(), "slack ask listener: %v\n", err)
					}
				}()
			}

			if builder != nil {
				go func() {
					t := time.NewTicker(30 * time.Second)
					defer t.Stop()
					for {
						select {
						case <-runCtx.Done():
							return
						case <-t.C:
							for _, ch := range builder.Sweep() {
								emitChange(ch)
							}
							_ = builder.Persist()
							emitHealth()
						}
					}
				}()
			}

			err = eng.Run(runCtx)
			if err == context.Canceled || err == context.DeadlineExceeded {
				return nil
			}
			return err
		},
	}
	cmd.Flags().StringVarP(&ns, "namespace", "n", "", "namespace to watch (required)")
	cmd.Flags().StringVar(&kubeCtx, "context", "", "kubeconfig context (ignored with --in-cluster)")
	cmd.Flags().BoolVar(&inCluster, "in-cluster", false, "use InClusterConfig (ServiceAccount)")
	cmd.Flags().BoolVar(&emitJSON, "json", false, "emit one JSON object per line")
	cmd.Flags().BoolVar(&emitInitial, "emit-initial", false, "emit current Pods/Events as Added before live watch")
	cmd.Flags().StringSliceVar(&watchList, "watch", nil, "resources to watch (default pods,events; deployments,replicasets,statefulsets,jobs,cronjobs,pvc,configmaps,secrets). secrets are opt-in and metadata-only")
	cmd.Flags().BoolVar(&incidents, "incidents", false, "correlate problem signals into Incident changes")
	cmd.Flags().BoolVar(&fetchLogs, "fetch-logs", false, "on CrashLoop/Failed/OOM attach a short log tail (enables --incidents)")
	cmd.Flags().BoolVar(&buildContext, "build-context", false, "assemble AgentContext for LLM (enables --incidents)")
	cmd.Flags().BoolVar(&doAnalyze, "analyze", false, "run LLM/heuristic analyzer → gated AgentAlert")
	cmd.Flags().BoolVar(&notifyDiscord, "discord", false, "post gated alerts to Discord webhook (enables --analyze)")
	cmd.Flags().StringVar(&discordURL, "discord-webhook-url", "", "override KPROMPT_DISCORD_WEBHOOK_URL for --discord")
	cmd.Flags().BoolVar(&notifySlack, "slack", false, "post gated alerts to Slack (enables --analyze)")
	cmd.Flags().BoolVar(&notifyWebhook, "webhook", false, "POST gated AgentAlert JSON to webhook URL (enables --analyze)")
	cmd.Flags().StringVar(&webhookURL, "webhook-url", "", "override KPROMPT_WEBHOOK_URL for --webhook")
	cmd.Flags().BoolVar(&trackHealth, "health", false, "emit namespace health score and risk_increasing trends (enables --incidents)")
	cmd.Flags().StringVar(&agentCR, "agent-cr", "", "KpromptAgent name to patch status (or KPROMPT_AGENT_CR)")
	cmd.Flags().StringVar(&agentCRNS, "agent-cr-namespace", "", "namespace of --agent-cr (default: POD_NAMESPACE / default)")
	cmd.Flags().BoolVar(&useMemory, "memory", false, "discover/load namespace dependency facts into analyzer context")
	cmd.Flags().StringVar(&memoryBackend, "memory-backend", "file", "memory store: file|configmap")
	cmd.Flags().StringVar(&memoryDir, "memory-dir", "", "file backend directory (default ~/.config/kprompt/memory)")
	cmd.Flags().BoolVar(&usePatterns, "patterns", false, "learn incident signatures; boost confidence on seen-before (never mutates)")
	cmd.Flags().StringVar(&patternsBackend, "patterns-backend", "file", "pattern store: file|configmap")
	cmd.Flags().StringVar(&patternsDir, "patterns-dir", "", "file backend directory (default ~/.config/kprompt/patterns)")
	cmd.Flags().BoolVar(&autopilotProp, "autopilot-propose", false, "emit AutopilotProposal for allowlisted actions (propose-only by default)")
	cmd.Flags().StringVar(&autopilotDir, "autopilot-audit-dir", "", "autopilot audit directory (default ~/.config/kprompt/autopilot)")
	cmd.Flags().StringVar(&autopilotPolicy, "autopilot-policy", "", "RemediationPolicy JSON file")
	cmd.Flags().BoolVar(&autopilotApply, "autopilot-apply", false, "apply proposals when policy mode=policyAuto apply=true (off by default)")
	cmd.Flags().StringVar(&incidentsBackend, "incidents-backend", "", "persist incidents across restarts: file|configmap")
	cmd.Flags().StringVar(&incidentsDir, "incidents-dir", "", "file backend directory (default ~/.config/kprompt/incidents)")
	cmd.Flags().BoolVar(&slackAsk, "slack-ask", false, "Slack Events ask listener for status/why/what broke/false positive (read-only)")
	cmd.Flags().StringVar(&slackAskAddr, "slack-ask-addr", ":8080", "listen address for --slack-ask Events API")
	cmd.Flags().StringVar(&coordinatorURL, "coordinator-url", "", "POST CoordinatorHandoff when cross-ns suspicion (opt-in)")
	cmd.Flags().BoolVar(&gitopsEvidence, "gitops-evidence", false, "attach Argo/Flux sync + deploy history EvidenceRefs (opt-in)")
	cmd.Flags().BoolVar(&heuristic, "heuristic", false, "with --analyze, skip LLM and use local heuristics only")
	cmd.Flags().StringVar(&providerName, "provider", "", "LLM provider for --analyze (default from config)")
	cmd.Flags().StringVar(&modelName, "model", "", "LLM model for --analyze (default from config)")
	cmd.Flags().StringVar(&minSeverity, "min-severity", "", "alert gate minimum severity (default medium)")
	cmd.Flags().Float64Var(&minConfidence, "min-confidence", 0, "alert gate minimum confidence 0..1 (default 0.7)")
	cmd.Flags().DurationVar(&duration, "duration", 0, "stop after duration (0 = until signal); useful for e2e")
	_ = cmd.MarkFlagRequired("namespace")
	return cmd
}

func openMemoryStore(backend, dir, ns string, inCluster bool, clients *cluster.Clients) (memory.Store, error) {
	b := strings.ToLower(strings.TrimSpace(backend))
	if b == "" {
		b = "file"
	}
	switch b {
	case "configmap":
		if clients == nil || clients.Clientset == nil {
			return nil, fmt.Errorf("memory: configmap backend requires kubernetes")
		}
		return memory.ConfigMapStore{Client: clients.Clientset, Namespace: ns}, nil
	case "file":
		if dir == "" {
			dir = memory.DefaultDir()
		}
		return memory.FileStore{Dir: dir}, nil
	default:
		return nil, fmt.Errorf("memory: unknown backend %q (want file|configmap)", backend)
	}
}

func openIncidentsStore(backend, dir, ns string, inCluster bool, clients *cluster.Clients) (correlate.Store, error) {
	b := strings.ToLower(strings.TrimSpace(backend))
	switch b {
	case "configmap":
		if clients == nil || clients.Clientset == nil {
			return nil, fmt.Errorf("incidents: configmap backend requires kubernetes")
		}
		return correlate.ConfigMapStore{Client: clients.Clientset, Namespace: ns}, nil
	case "file":
		if dir == "" {
			dir = correlate.DefaultIncidentsDir()
		}
		return correlate.FileStore{Dir: dir}, nil
	default:
		return nil, fmt.Errorf("incidents: unknown backend %q (want file|configmap)", backend)
	}
}

func openPatternsStore(backend, dir, ns string, inCluster bool, clients *cluster.Clients) (patterns.Store, error) {
	b := strings.ToLower(strings.TrimSpace(backend))
	if b == "" {
		b = "file"
	}
	switch b {
	case "configmap":
		if clients == nil || clients.Clientset == nil {
			return nil, fmt.Errorf("patterns: configmap backend requires kubernetes")
		}
		return patterns.ConfigMapStore{Client: clients.Clientset, Namespace: ns}, nil
	case "file":
		if dir == "" {
			dir = patterns.DefaultDir()
		}
		return patterns.FileStore{Dir: dir}, nil
	default:
		return nil, fmt.Errorf("patterns: unknown backend %q (want file|configmap)", backend)
	}
}

func newAgentPatternsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "patterns",
		Short: "Incident Memory signatures (local/in-cluster only)",
	}
	cmd.AddCommand(newAgentPatternsListCmd())
	return cmd
}

func newAgentGraphCmd() *cobra.Command {
	var (
		ns             string
		kubeCtx        string
		inCluster      bool
		includeNP      bool
		includeIngress bool
		includePVC     bool
		includeRefs    bool
		output         string
	)
	cmd := &cobra.Command{
		Use:   "graph",
		Short: "Dump Knowledge Graph service dependency graph (read-only)",
		Long: `Build a read-only service-graph for one namespace (Services, EndpointSlices,
Ingress→Service, Pod→PVC, Pod→Secret/ConfigMap name-only refs, optional NetworkPolicies).

Does not mutate the cluster. Never reads Secret.data. External APIs / topology UI
remain out of scope — see docs/graph.md.`,
		Example: `  kprompt agent graph -n payments
  kprompt agent graph -n payments --output json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ns = strings.TrimSpace(ns)
			if ns == "" {
				return fmt.Errorf("--namespace is required")
			}
			var clients *cluster.Clients
			var err error
			if inCluster {
				clients, err = cluster.ConnectInCluster()
			} else {
				clients, err = cluster.Connect(kubeCtx)
			}
			if err != nil {
				return err
			}
			report, err := graph.Build(cmd.Context(), clients.Clientset, graph.Request{
				Namespace:            ns,
				IncludeNetworkPolicy: includeNP,
				IncludeIngress:       includeIngress,
				IncludePVC:           includePVC,
				IncludeVolumeRefs:    includeRefs,
			})
			if err != nil {
				return err
			}
			settings := tools.LoadSettings(config.File{})
			if otelClient, oerr := tools.NewOTelClient(settings); oerr == nil {
				graph.EnrichFromOTel(cmd.Context(), otelClient, &report, time.Hour)
			}
			if strings.EqualFold(strings.TrimSpace(output), "json") {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(report)
			}
			ui.PrintGraphReport(cmd.OutOrStdout(), report)
			return nil
		},
	}
	cmd.Flags().StringVarP(&ns, "namespace", "n", "", "namespace to graph (required)")
	cmd.Flags().StringVar(&kubeCtx, "context", "", "kubeconfig context")
	cmd.Flags().BoolVar(&inCluster, "in-cluster", false, "use in-cluster config")
	cmd.Flags().BoolVar(&includeNP, "network-policy", true, "include NetworkPolicy selects edges")
	cmd.Flags().BoolVar(&includeIngress, "ingress", true, "include Ingress→Service exposes edges")
	cmd.Flags().BoolVar(&includePVC, "pvc", true, "include Pod→PVC mounts edges")
	cmd.Flags().BoolVar(&includeRefs, "volume-refs", true, "include Pod→Secret/ConfigMap name-only refs")
	cmd.Flags().StringVarP(&output, "output", "o", "text", "text|json")
	_ = cmd.MarkFlagRequired("namespace")
	return cmd
}

func newAgentPatternsListCmd() *cobra.Command {
	var ns, backend, dir, kubeCtx string
	var inCluster bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List learned incident signatures for a namespace",
		RunE: func(cmd *cobra.Command, args []string) error {
			ns = strings.TrimSpace(ns)
			if ns == "" {
				return fmt.Errorf("--namespace is required")
			}
			clients, err := connectOptional(kubeCtx, inCluster, backend == "configmap")
			if err != nil {
				return err
			}
			store, err := openPatternsStore(backend, dir, ns, inCluster, clients)
			if err != nil {
				return err
			}
			snap, err := patterns.New(store).List(ns)
			if err != nil {
				return err
			}
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(snap)
		},
	}
	cmd.Flags().StringVarP(&ns, "namespace", "n", "", "namespace (required)")
	cmd.Flags().StringVar(&backend, "patterns-backend", "file", "file|configmap")
	cmd.Flags().StringVar(&dir, "patterns-dir", "", "file backend directory")
	cmd.Flags().StringVar(&kubeCtx, "context", "", "kubeconfig context")
	cmd.Flags().BoolVar(&inCluster, "in-cluster", false, "use InClusterConfig")
	_ = cmd.MarkFlagRequired("namespace")
	return cmd
}

func newAgentMemoryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "memory",
		Short: "Namespace dependency facts (local/in-cluster only)",
	}
	cmd.AddCommand(newAgentMemoryListCmd())
	cmd.AddCommand(newAgentMemorySetCmd())
	cmd.AddCommand(newAgentMemoryDiscoverCmd())
	return cmd
}

func newAgentMemoryListCmd() *cobra.Command {
	var ns, backend, dir, kubeCtx string
	var inCluster bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List remembered facts for a namespace",
		RunE: func(cmd *cobra.Command, args []string) error {
			ns = strings.TrimSpace(ns)
			if ns == "" {
				return fmt.Errorf("--namespace is required")
			}
			clients, err := connectOptional(kubeCtx, inCluster, backend == "configmap")
			if err != nil {
				return err
			}
			store, err := openMemoryStore(backend, dir, ns, inCluster, clients)
			if err != nil {
				return err
			}
			snap, err := memory.New(store).List(ns)
			if err != nil {
				return err
			}
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(snap)
		},
	}
	cmd.Flags().StringVarP(&ns, "namespace", "n", "", "namespace (required)")
	cmd.Flags().StringVar(&backend, "memory-backend", "file", "file|configmap")
	cmd.Flags().StringVar(&dir, "memory-dir", "", "file backend directory")
	cmd.Flags().StringVar(&kubeCtx, "context", "", "kubeconfig context")
	cmd.Flags().BoolVar(&inCluster, "in-cluster", false, "use InClusterConfig")
	_ = cmd.MarkFlagRequired("namespace")
	return cmd
}

func newAgentMemorySetCmd() *cobra.Command {
	var ns, backend, dir, kubeCtx, key, value, kind string
	var inCluster bool
	cmd := &cobra.Command{
		Use:   "set",
		Short: "Upsert a manual fact (never uploaded to control plane)",
		RunE: func(cmd *cobra.Command, args []string) error {
			ns = strings.TrimSpace(ns)
			key = strings.TrimSpace(key)
			if ns == "" || key == "" {
				return fmt.Errorf("--namespace and --key are required")
			}
			if kind == "" {
				kind = memory.KindNote
			}
			clients, err := connectOptional(kubeCtx, inCluster, backend == "configmap")
			if err != nil {
				return err
			}
			store, err := openMemoryStore(backend, dir, ns, inCluster, clients)
			if err != nil {
				return err
			}
			snap, err := memory.New(store).Upsert(ns, memory.Fact{
				Kind:   kind,
				Key:    key,
				Value:  value,
				Source: "manual",
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "saved %d facts for namespace %q\n", len(snap.Facts), ns)
			return nil
		},
	}
	cmd.Flags().StringVarP(&ns, "namespace", "n", "", "namespace (required)")
	cmd.Flags().StringVar(&key, "key", "", "fact key (e.g. redis, team)")
	cmd.Flags().StringVar(&value, "value", "", "fact value")
	cmd.Flags().StringVar(&kind, "kind", memory.KindNote, "dependency|note")
	cmd.Flags().StringVar(&backend, "memory-backend", "file", "file|configmap")
	cmd.Flags().StringVar(&dir, "memory-dir", "", "file backend directory")
	cmd.Flags().StringVar(&kubeCtx, "context", "", "kubeconfig context")
	cmd.Flags().BoolVar(&inCluster, "in-cluster", false, "use InClusterConfig")
	_ = cmd.MarkFlagRequired("namespace")
	_ = cmd.MarkFlagRequired("key")
	return cmd
}

func newAgentMemoryDiscoverCmd() *cobra.Command {
	var ns, backend, dir, kubeCtx string
	var inCluster bool
	cmd := &cobra.Command{
		Use:   "discover",
		Short: "Scan Services/Deployments for dependency hints and persist them",
		RunE: func(cmd *cobra.Command, args []string) error {
			ns = strings.TrimSpace(ns)
			if ns == "" {
				return fmt.Errorf("--namespace is required")
			}
			var clients *cluster.Clients
			var err error
			if inCluster {
				clients, err = cluster.ConnectInCluster()
			} else {
				clients, err = cluster.Connect(kubeCtx)
			}
			if err != nil {
				return err
			}
			facts, err := memory.Discover(cmd.Context(), clients.Clientset, ns)
			if err != nil {
				return err
			}
			store, err := openMemoryStore(backend, dir, ns, inCluster, clients)
			if err != nil {
				return err
			}
			snap, err := memory.New(store).Upsert(ns, facts...)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "discovered %d deps; namespace %q now has %d facts\n", len(facts), ns, len(snap.Facts))
			return nil
		},
	}
	cmd.Flags().StringVarP(&ns, "namespace", "n", "", "namespace (required)")
	cmd.Flags().StringVar(&backend, "memory-backend", "file", "file|configmap")
	cmd.Flags().StringVar(&dir, "memory-dir", "", "file backend directory")
	cmd.Flags().StringVar(&kubeCtx, "context", "", "kubeconfig context")
	cmd.Flags().BoolVar(&inCluster, "in-cluster", false, "use InClusterConfig")
	_ = cmd.MarkFlagRequired("namespace")
	return cmd
}

func connectOptional(kubeCtx string, inCluster, needClient bool) (*cluster.Clients, error) {
	if !needClient {
		return nil, nil
	}
	if inCluster {
		return cluster.ConnectInCluster()
	}
	return cluster.Connect(kubeCtx)
}
