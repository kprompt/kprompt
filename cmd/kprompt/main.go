package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/kprompt/kprompt/internal/config"
	"github.com/kprompt/kprompt/internal/pipeline"
	"github.com/kprompt/kprompt/internal/ui"
)

var (
	version   = "0.0.0-dev"
	approve   bool
	waitFlag  bool
	timeout   time.Duration
	provider  string
	model     string
	kubeCtx   string
	namespace string
	outputFmt string
	theme     string
)

func main() {
	root := &cobra.Command{
		Use:           "kprompt [prompt]",
		Short:         "Talk to your Kubernetes cluster with natural language",
		Long:          "kprompt plans cluster actions from a prompt, applies safety policy, and mutates only after interactive confirm or --approve.",
		Args:          cobra.ArbitraryArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			prompt := strings.TrimSpace(strings.Join(args, " "))
			if prompt == "" {
				return cmd.Help()
			}
			file, err := config.LoadFile()
			if err != nil {
				return err
			}
			cfg := config.Merge(file, provider, model, kubeCtx, namespace, approve, prompt)
			cfg.Wait = waitFlag
			cfg.Timeout = timeout
			cfg.Output = outputFmt
			cfg.NamespaceFromCLI = cmd.Flags().Changed("namespace")
			cfg.ContextFromCLI = cmd.Flags().Changed("context")
			if cmd.Flags().Changed("theme") {
				cfg.Theme = theme
			}
			ui.SetTheme(cfg.Theme)
			return pipeline.Run(cmd.Context(), cfg, cmd.OutOrStdout())
		},
	}

	root.PersistentFlags().BoolVar(&approve, "approve", false, "apply the plan without interactive confirmation")
	root.PersistentFlags().BoolVar(&waitFlag, "wait", false, "after apply, wait for Deployment rollout to complete")
	root.PersistentFlags().DurationVar(&timeout, "timeout", 5*time.Minute, "timeout for --wait (default 5m)")
	root.PersistentFlags().StringVar(&provider, "provider", "", "LLM provider (openai|anthropic|gemini|groq|mistral|deepseek|openrouter|together|ollama|openai-compatible)")
	root.PersistentFlags().StringVar(&model, "model", "", "LLM model id")
	root.PersistentFlags().StringVar(&kubeCtx, "context", "", "kubeconfig context")
	root.PersistentFlags().StringVarP(&namespace, "namespace", "n", "", "default namespace")
	root.PersistentFlags().StringVarP(&outputFmt, "output", "o", "text", "output format: text|json")
	root.PersistentFlags().StringVar(&theme, "theme", "", "color theme: auto|dracula|nord|gruvbox|mono|none")

	root.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintln(cmd.OutOrStdout(), version)
		},
	})
	root.AddCommand(newConfigCmd())
	root.AddCommand(newHistoryCmd())
	root.AddCommand(newToolsCmd())
	root.AddCommand(newLoginCmd())
	root.AddCommand(newLogoutCmd())
	root.AddCommand(newWhoamiCmd())
	root.AddCommand(newPolicyCmd())

	ctx := context.Background()
	if err := root.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
