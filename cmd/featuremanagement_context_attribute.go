package cmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Dynatrace/dtctl/pkg/output"
	"github.com/Dynatrace/dtctl/pkg/resources/featuremanagement"
)

func newGetFMContextAttributesCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "context-attributes [context-attribute-key]",
		Aliases: []string{"context-attribute"},
		Short:   "Get context attributes of a Feature Management project",
		Long: `Get the context attributes of a Feature Management project.

A context attribute declares a key of the evaluation context, such as
plan.tier, and its value type; targeting conditions match on it by its
context key. RULES is the number of targeting rules that reference it. An
attribute is identified by its key; a name is accepted too when no attribute
has it as a key.

The project comes from --project, or from the context's default project
(dtctl config set-context <context> --fm-project <project>).

Examples:
  # List the context attributes of a project
  dtctl get fm context-attributes --project checkout

  # Get one context attribute
  dtctl get fm context-attribute plan-tier

  # Wide output (adds description, last modified, version)
  dtctl get fm context-attributes -o wide
`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, c, printer, err := setup(cmdContext(cmd))
			if err != nil {
				return err
			}
			project, err := resolveFMProject(cmd, cfg)
			if err != nil {
				return err
			}
			h := featuremanagement.NewHandler(c)

			if len(args) > 0 {
				a, err := h.GetContextAttribute(project, args[0])
				if err != nil {
					return err
				}
				return printer.Print(a)
			}

			attrs, err := h.ListContextAttributes(project)
			if err != nil {
				return err
			}
			return printer.PrintList(attrs)
		},
	}
}

func newDescribeFMContextAttributeCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "context-attribute <context-attribute-key>",
		Aliases: []string{"context-attributes"},
		Short:   "Show details of a Feature Management context attribute",
		Long: `Show detailed information about a context attribute of a Feature
Management project, including its schema and the features, and the release
stages within them, whose targeting references it.

The argument is the attribute's key; a name is accepted too when no attribute
has it as a key. The project comes from --project, or from the context's
default project.

-o json or -o yaml prints the attribute as the API returns it, without the
usage; the usage is part of the text view only.

Examples:
  # Describe a context attribute
  dtctl describe fm context-attribute plan-tier --project checkout

  # Output as JSON or YAML
  dtctl describe fm context-attribute plan-tier -o yaml
`,
		Args: cobra.ExactArgs(1),
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

			a, err := h.GetContextAttribute(project, args[0])
			if err != nil {
				return err
			}

			if useActionDescribeTextView(ctx) {
				usage, err := h.GetContextAttributeUsage(project, a.Key)
				if err != nil {
					return err
				}
				newFMUserNames(c).modificationInfo(&a.ModificationInfo)
				printFMContextAttributeDescribe(currentStdout(ctx), a, usage)
				return nil
			}

			enrichAgent(printer, "describe", "fm/context-attribute")
			return printer.Print(a)
		},
	}
}

// printFMContextAttributeDescribe renders the vertical describe view of a
// context attribute, ending with the features that reference it.
func printFMContextAttributeDescribe(w io.Writer, a *featuremanagement.ContextAttribute, usage []featuremanagement.ContextAttributeUsage) {
	const kw = 15
	output.FprintDescribeKV(w, "Key:", kw, "%s", a.Key)
	output.FprintDescribeKV(w, "Name:", kw, "%s", a.Name)
	output.FprintDescribeKV(w, "Project:", kw, "%s", a.ProjectKey)
	if a.Description != "" {
		output.FprintDescribeKV(w, "Description:", kw, "%s", a.Description)
	}
	output.FprintDescribeKV(w, "Context Key:", kw, "%s", a.ContextKey)
	output.FprintDescribeKV(w, "Type:", kw, "%s", a.Type)
	output.FprintDescribeKV(w, "Built-in:", kw, "%s", a.BuiltInDisplay)
	if a.Schema != "" {
		output.FprintDescribeKV(w, "Schema:", kw, "%s", a.Schema)
	}
	output.FprintDescribeKV(w, "Rules:", kw, "%s", a.UsagesDisplay)
	output.FprintDescribeKV(w, "Version:", kw, "%d", a.Version)
	printFMModificationInfo(w, kw, a.ModificationInfo)
	if a.ResourceContext != nil && len(a.ResourceContext.Operations) > 0 {
		output.FprintDescribeKV(w, "Operations:", kw, "%s", strings.Join(a.ResourceContext.Operations, ", "))
	}

	fmt.Fprintln(w)
	output.FprintDescribeSection(w, "Used By:")
	if len(usage) == 0 {
		fmt.Fprintln(w, "  (no targeting rule references it)")
		return
	}
	width := 0
	for _, u := range usage {
		width = max(width, len(u.FeatureKey))
	}
	for _, u := range usage {
		stages := make([]string, len(u.ReleaseStages))
		for i, s := range u.ReleaseStages {
			stages[i] = s.ReleaseStageKey
		}
		fmt.Fprintf(w, "  %-*s  %s\n", width, u.FeatureKey, strings.Join(stages, ", "))
	}
}
