package cmd

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/Dynatrace/dtctl/pkg/output"
	"github.com/Dynatrace/dtctl/pkg/resources/featuremanagement"
)

func newGetFMOverridesCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "overrides [feature-key]",
		Aliases: []string{"override"},
		Short:   "Get your own variant overrides",
		Long: `Get your own variant overrides in a Feature Management project.

An override serves you a chosen variant of a feature in a release stage,
applied only to evaluations that carry one of your targeting keys, so you can
test a variant without changing targeting for anyone else. Overrides belong to
the signed-in user; nobody else's are visible.

Without an argument, lists every override you have in the project, by feature
and stage. With a feature, shows your override in each release stage of the
project, "-" where you have none. The feature is a key; a name is accepted too
when no feature has it as a key.

The project comes from --project, or from the context's default project.

Examples:
  # Your overrides across the project
  dtctl get fm overrides

  # Your override per release stage for one feature
  dtctl get fm override new-checkout

  # As the API returns them
  dtctl get fm overrides -o yaml
`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmdContext(cmd)
			cfg, c, printer, err := setup(ctx)
			if err != nil {
				return err
			}
			project, err := resolveFMProject(cmd, cfg)
			if err != nil {
				return err
			}
			h := featuremanagement.NewHandler(c)

			if len(args) > 0 {
				fo, err := h.GetFeatureOverrides(project, args[0])
				if err != nil {
					return err
				}
				if useFMTableView(ctx) {
					return printer.PrintList(featuremanagement.FeatureOverrideRows(fo))
				}
				return printer.Print(fo)
			}

			items, err := h.ListOverrides(project)
			if err != nil {
				return err
			}
			if useFMTableView(ctx) {
				return printer.PrintList(featuremanagement.ProjectOverrideRows(items))
			}
			return printer.PrintList(items)
		},
	}
}

func newDescribeFMOverridesCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "overrides",
		Aliases: []string{"override"},
		Short:   "Show your targeting keys and every override",
		Long: `Show your own override setup in a Feature Management project: the
targeting keys your overrides apply to (your overrides profile), and every
variant override you have, by feature and release stage.

The project comes from --project, or from the context's default project.

Examples:
  # Your targeting keys and overrides
  dtctl describe fm overrides

  # Output as JSON or YAML
  dtctl describe fm overrides -o yaml
`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmdContext(cmd)
			cfg, c, printer, err := setup(ctx)
			if err != nil {
				return err
			}
			project, err := resolveFMProject(cmd, cfg)
			if err != nil {
				return err
			}

			sum, err := featuremanagement.NewHandler(c).GetOverridesSummary(project)
			if err != nil {
				return err
			}

			if useActionDescribeTextView(ctx) {
				printFMOverridesDescribe(currentStdout(ctx), project, sum)
				return nil
			}

			enrichAgent(printer, "describe", "fm/override")
			return printer.Print(sum)
		},
	}
}

// printFMOverridesDescribe renders the caller's targeting keys, then their
// overrides grouped by feature.
func printFMOverridesDescribe(w io.Writer, project string, s *featuremanagement.OverridesSummary) {
	output.FprintDescribeKV(w, "Project:", 15, "%s", project)

	fmt.Fprintln(w)
	output.FprintDescribeSection(w, "Targeting Keys:")
	if len(s.TargetingKeys) == 0 {
		fmt.Fprintln(w, "  (none; your overrides apply to no evaluation until you add one)")
	}
	for _, k := range s.TargetingKeys {
		fmt.Fprintf(w, "  %s\n", k)
	}

	fmt.Fprintln(w)
	output.FprintDescribeSection(w, "Overrides:")
	if len(s.Items) == 0 {
		fmt.Fprintln(w, "  (none)")
		return
	}
	width := 0
	for _, f := range s.Items {
		for _, o := range f.Overrides {
			width = max(width, len(o.ReleaseStageKey))
		}
	}
	for i, f := range s.Items {
		if i > 0 {
			fmt.Fprintln(w)
		}
		fmt.Fprintf(w, "  %s\n", f.FeatureKey)
		for _, o := range f.Overrides {
			fmt.Fprintf(w, "    %-*s  %s\n", width, o.ReleaseStageKey, o.VariantKey)
		}
	}
}
