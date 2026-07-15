package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/kprompt/kprompt/internal/config"
	"github.com/kprompt/kprompt/internal/pipeline"
)

var (
	version   = "0.0.0-dev"
	approve   bool
	provider  string
	model     string
	kubeCtx   string
	namespace string
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
			return pipeline.Run(cmd.Context(), cfg, cmd.OutOrStdout())
		},
	}

	root.Flags().BoolVar(&approve, "approve", false, "apply the plan without interactive confirmation")
	root.Flags().StringVar(&provider, "provider", "", "LLM provider (openai|anthropic|gemini|groq|mistral|deepseek|openrouter|together|ollama|openai-compatible)")
	root.Flags().StringVar(&model, "model", "", "LLM model id")
	root.Flags().StringVar(&kubeCtx, "context", "", "kubeconfig context")
	root.Flags().StringVarP(&namespace, "namespace", "n", "", "default namespace")

	root.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintln(cmd.OutOrStdout(), version)
		},
	})

	ctx := context.Background()
	if err := root.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
