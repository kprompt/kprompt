package main

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/kprompt/kprompt/internal/config"
	"github.com/kprompt/kprompt/internal/pipeline"
	"github.com/kprompt/kprompt/internal/recipe"
	"github.com/kprompt/kprompt/internal/ui"
)

func newRecipeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "recipe",
		Short: "Curated workflow packs (harden, peak prep, Ingress→Gateway discover, RCA chains)",
		Long: `Recipes expand into multi-step prompts run through the same route + approval
loop as a manual "A then B" chain. Never mutates silently (S-013 · T-088).`,
		Example: `  # List all built-in recipe packs
  kprompt recipe list

  # Show steps and triggers for a specific recipe
  kprompt recipe show harden-production

  # Execute a recipe in a specific namespace
  kprompt recipe run harden-production -n payments

  # Run an RCA chain for a crashlooping workload
  kprompt recipe run crashloop-rca --workload api -n payments

  # Run via natural language trigger directly
  kprompt "prepare for black friday"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newRecipeListCmd())
	cmd.AddCommand(newRecipeShowCmd())
	cmd.AddCommand(newRecipeRunCmd())
	return cmd
}

func newRecipeListCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List built-in recipe packs",
		Example: `  # List all recipe packs as a formatted table
  kprompt recipe list

  # Output the catalog in JSON format
  kprompt recipe list --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if jsonOut {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(recipe.Catalog())
			}
			fmt.Fprint(cmd.OutOrStdout(), recipe.FormatList())
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON")
	return cmd
}

func newRecipeShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <id>",
		Short: "Show one recipe (steps + notes)",
		Args:  cobra.ExactArgs(1),
		Example: `  # Show details of the harden-production recipe
  kprompt recipe show harden-production`,
		RunE: func(cmd *cobra.Command, args []string) error {
			r, ok := recipe.Lookup(args[0])
			if !ok {
				return fmt.Errorf("unknown recipe %q — kprompt recipe list", args[0])
			}
			fmt.Fprint(cmd.OutOrStdout(), recipe.FormatShow(r))
			return nil
		},
	}
}

func newRecipeRunCmd() *cobra.Command {
	var workload string
	var kubeCtx string
	var ns string
	var approve bool
	var outputFmt string

	cmd := &cobra.Command{
		Use:   "run <id>",
		Short: "Expand and execute a recipe as a multi-step route",
		Args:  cobra.ExactArgs(1),
		Example: `  # Run a recipe in a specific namespace
  kprompt recipe run harden-production -n payments

  # Run a recipe with workload and namespace overrides
  kprompt recipe run crashloop-rca --workload api -n payments

  # Run a recipe and automatically approve mutating steps
  kprompt recipe run oom-rca --workload backend --approve`,
		RunE: func(cmd *cobra.Command, args []string) error {
			r, ok := recipe.Lookup(args[0])
			if !ok {
				return fmt.Errorf("unknown recipe %q — kprompt recipe list", args[0])
			}
			file, err := config.LoadFile()
			if err != nil {
				return err
			}
			namespace := ns
			if namespace == "" {
				namespace = file.Namespace
			}
			if namespace == "" {
				namespace = "default"
			}
			steps, err := r.Expand(namespace, workload)
			if err != nil {
				return err
			}
			cfg := config.Merge(file, "", "", kubeCtx, namespace, approve, "recipe:"+r.ID)
			cfg.Output = outputFmt
			cfg.NamespaceFromCLI = cmd.Flags().Changed("namespace")
			cfg.ContextFromCLI = cmd.Flags().Changed("context")
			ui.SetTheme(cfg.Theme)
			fmt.Fprint(cmd.OutOrStdout(), recipe.FormatShow(r))
			fmt.Fprintln(cmd.OutOrStdout())
			return pipeline.RunWith(cmd.Context(), cfg, cmd.OutOrStdout(), pipeline.Deps{
				RouteSteps: steps,
			})
		},
	}
	cmd.Flags().StringVar(&workload, "workload", "", "workload name for RCA recipes ({{workload}})")
	cmd.Flags().StringVar(&kubeCtx, "context", "", "kubeconfig context")
	cmd.Flags().StringVarP(&ns, "namespace", "n", "", "namespace for {{namespace}} steps")
	cmd.Flags().BoolVar(&approve, "approve", false, "approve mutating steps in the recipe route")
	cmd.Flags().StringVarP(&outputFmt, "output", "o", "text", "output format: text|json")
	return cmd
}
