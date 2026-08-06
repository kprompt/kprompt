package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/kprompt/kprompt/internal/coach"
	"github.com/kprompt/kprompt/internal/config"
	"github.com/kprompt/kprompt/internal/intent"
	"github.com/kprompt/kprompt/internal/pipeline"
	"github.com/kprompt/kprompt/internal/team"
	"github.com/kprompt/kprompt/internal/ui"
)

var (
	version            = "0.0.0-dev"
	approve            bool
	approveEachContext bool
	waitFlag           bool
	timeout            time.Duration
	provider           string
	model              string
	kubeCtx            string
	kubeCtxs           string
	namespace          string
	outputFmt          string
	theme              string
	gitopsPR           bool
	gitopsRepo         string
	gitopsPath         string
	gitopsBaseBranch   string
)

func main() {
	root := &cobra.Command{
		Use:   "kprompt [prompt]",
		Short: "Talk to your Kubernetes cluster with natural language",
		Long: `kprompt plans cluster actions from a prompt, applies safety policy, and mutates only after interactive confirm or --approve.

Day-0: kprompt init · kprompt demo · kprompt doctor · then kprompt "list pods"
Bare kprompt (no args) prints a readiness coach. Full help: kprompt --help · kprompt advanced`,
		Args:          cobra.ArbitraryArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			prompt := strings.TrimSpace(strings.Join(args, " "))
			if prompt == "" {
				return runCoach(cmd)
			}
			file, err := config.LoadFile()
			if err != nil {
				return err
			}
			if cmd.Flags().Changed("context") && cmd.Flags().Changed("contexts") {
				return fmt.Errorf("use either --context or --contexts, not both")
			}
			cfg := config.Merge(file, provider, model, kubeCtx, namespace, approve, prompt)
			cfg.Wait = waitFlag
			cfg.Timeout = timeout
			cfg.Output = outputFmt
			cfg.ApproveEachContext = approveEachContext
			cfg.NamespaceFromCLI = cmd.Flags().Changed("namespace")
			cfg.ContextFromCLI = cmd.Flags().Changed("context") || cmd.Flags().Changed("contexts")
			if cmd.Flags().Changed("theme") {
				cfg.Theme = theme
			}
			if gitopsPR || cmd.Flags().Changed("gitops") {
				cfg.GitOpsPR = gitopsPR
			}
			if cmd.Flags().Changed("gitops-repo") {
				cfg.GitOpsRepo = gitopsRepo
			}
			if cmd.Flags().Changed("gitops-path") {
				cfg.GitOpsPath = gitopsPath
			}
			if cmd.Flags().Changed("gitops-base-branch") {
				cfg.GitOpsBaseBranch = gitopsBaseBranch
			}
			team.ApplyOrgContextPolicy(&cfg)
			if raw := strings.TrimSpace(kubeCtxs); raw != "" {
				names := config.ParseContextsFlag(raw)
				resolved, err := config.ResolveContextList(names, cfg.Aliases)
				if err != nil {
					return err
				}
				cfg.Contexts = resolved
				if len(resolved) == 1 {
					cfg.Context = resolved[0]
				}
			} else if names := intent.ParseMultiContexts(prompt); len(names) > 0 {
				resolved, err := config.ResolveContextList(names, cfg.Aliases)
				if err != nil {
					return err
				}
				cfg.Contexts = resolved
			}
			ui.SetTheme(cfg.Theme)
			return pipeline.Run(cmd.Context(), cfg, cmd.OutOrStdout())
		},
	}

	root.PersistentFlags().BoolVar(&approve, "approve", false, "apply the plan without interactive confirmation")
	root.PersistentFlags().BoolVar(&approveEachContext, "approve-each-context", false, "apply a mutating plan to every --contexts entry (explicit; not implied by --approve)")
	root.PersistentFlags().BoolVar(&waitFlag, "wait", false, "after apply, wait for Deployment/StatefulSet rollout to complete")
	root.PersistentFlags().DurationVar(&timeout, "timeout", 5*time.Minute, "timeout for --wait (default 5m)")
	root.PersistentFlags().StringVar(&provider, "provider", "", "LLM provider (openai|anthropic|gemini|groq|xai|cerebras|mistral|deepseek|moonshot|openrouter|together|ollama|openai-compatible)")
	root.PersistentFlags().StringVar(&model, "model", "", "LLM model id")
	root.PersistentFlags().StringVar(&kubeCtx, "context", "", "kubeconfig context")
	root.PersistentFlags().StringVar(&kubeCtxs, "contexts", "", "comma-separated contexts for read fan-out / per-context mutate (aliases ok)")
	root.PersistentFlags().StringVarP(&namespace, "namespace", "n", "", "default namespace")
	root.PersistentFlags().StringVarP(&outputFmt, "output", "o", "text", "output format: text|json")
	root.PersistentFlags().StringVar(&theme, "theme", "", "color theme: auto|dracula|nord|gruvbox|mono|none")
	root.PersistentFlags().BoolVar(&gitopsPR, "gitops", false, "open/update a GitHub PR instead of applying to the cluster (requires gitops.repo)")
	root.PersistentFlags().StringVar(&gitopsRepo, "gitops-repo", "", "GitHub owner/name for --gitops (or config gitops.repo / KPROMPT_GITOPS_REPO)")
	root.PersistentFlags().StringVar(&gitopsPath, "gitops-path", "", "path prefix inside the repo for PR files (default kprompt)")
	root.PersistentFlags().StringVar(&gitopsBaseBranch, "gitops-base-branch", "", "PR base branch (default main)")

	registerCommandGroups(root)

	root.AddCommand(withGroup(&cobra.Command{
		Use:   "version",
		Short: "Print version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintln(cmd.OutOrStdout(), version)
		},
	}, groupDay0))
	root.AddCommand(newCompletionCmd()) // Hidden
	root.AddCommand(withGroup(newInitCmd(), groupDay0))
	root.AddCommand(withGroup(newDemoCmd(), groupDay0))
	root.AddCommand(withGroup(newDoctorCmd(), groupDay0))
	root.AddCommand(withGroup(newConfigCmd(), groupDay0))
	root.AddCommand(withGroup(newContextsCmd(), groupDay0))
	root.AddCommand(newAdvancedHelpCmd(root))

	root.AddCommand(withGroup(newThemeCmd(), groupAdvanced))
	root.AddCommand(withGroup(newDashCmd(), groupAdvanced))
	root.AddCommand(withGroup(newAgentCmd(), groupAdvanced))
	root.AddCommand(withGroup(newHistoryCmd(), groupAdvanced))
	root.AddCommand(withGroup(newWatchCmd(), groupAdvanced))
	root.AddCommand(withGroup(newRememberCmd(), groupAdvanced))
	root.AddCommand(withGroup(newForgetCmd(), groupAdvanced))
	root.AddCommand(withGroup(newSessionCmd(), groupAdvanced))
	root.AddCommand(withGroup(newToolsCmd(), groupAdvanced))
	root.AddCommand(withGroup(newSetupCmd(), groupAdvanced))
	root.AddCommand(withGroup(newLearnCmd(), groupAdvanced))
	root.AddCommand(withGroup(newRecipeCmd(), groupAdvanced))
	root.AddCommand(withGroup(newLoginCmd(), groupAdvanced))
	root.AddCommand(withGroup(newLogoutCmd(), groupAdvanced))
	root.AddCommand(withGroup(newWhoamiCmd(), groupAdvanced))
	root.AddCommand(withGroup(newPolicyCmd(), groupAdvanced))
	root.AddCommand(withGroup(newSecretsCmd(), groupAdvanced))
	root.AddCommand(withGroup(newRunCmd(), groupAdvanced))

	ctx := context.Background()
	if err := root.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runCoach(cmd *cobra.Command) error {
	out := cmd.OutOrStdout()
	st, err := coach.Gather(cmd.Context(), version, coach.Options{})
	if err != nil {
		return err
	}
	if stdoutIsTerminal() {
		return coach.Format(out, st)
	}
	return coach.FormatBrief(out, st)
}

func stdoutIsTerminal() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}
