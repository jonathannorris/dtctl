package cmd

import (
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Dynatrace/dtctl/pkg/output"
	"github.com/Dynatrace/dtctl/pkg/resources/featuremanagement"
)

func newGetFMReleaseStagesCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "release-stages [release-stage-key]",
		Aliases: []string{"release-stage", "stages", "stage"},
		Short:   "Get release stages of a Feature Management project",
		Long: `Get the release stages of a Feature Management project.

A release stage is one environment a feature is rolled out through, such as
development, staging or production. Stages are listed in their display order,
the order a feature is promoted through them. A stage is identified by its
key; a name is accepted too when no stage has it as a key.

The project comes from --project, or from the context's default project
(dtctl config set-context <context> --fm-project <project>).

Examples:
  # List the release stages of a project
  dtctl get fm release-stages --project checkout

  # Get one release stage
  dtctl get fm release-stage production --project checkout

  # Wide output (adds description, display order, last modified, version)
  dtctl get fm release-stages -o wide
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
				rs, err := h.GetReleaseStage(project, args[0])
				if err != nil {
					return err
				}
				return printer.Print(rs)
			}

			stages, err := h.ListReleaseStages(project)
			if err != nil {
				return err
			}
			return printer.PrintList(stages)
		},
	}
}

func newDescribeFMReleaseStageCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "release-stage <release-stage-key>",
		Aliases: []string{"release-stages", "stage", "stages"},
		Short:   "Show details of a Feature Management release stage",
		Long: `Show detailed information about a release stage of a Feature Management
project.

The argument is the stage's key; a name is accepted too when no stage has it
as a key. The project comes from --project, or from the context's default
project.

Examples:
  # Describe a release stage
  dtctl describe fm release-stage production --project checkout

  # Output as JSON or YAML
  dtctl describe fm release-stage production -o json
  dtctl describe fm release-stage production -o yaml
`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, c, printer, err := setup(cmdContext(cmd))
			if err != nil {
				return err
			}
			project, err := resolveFMProject(cmd, cfg)
			if err != nil {
				return err
			}

			rs, err := featuremanagement.NewHandler(c).GetReleaseStage(project, args[0])
			if err != nil {
				return err
			}

			if useActionDescribeTextView(cmdContext(cmd)) {
				newFMUserNames(c).modificationInfo(&rs.ModificationInfo)
				printFMReleaseStageDescribe(currentStdout(cmdContext(cmd)), project, rs)
				return nil
			}

			enrichAgent(printer, "describe", "fm/release-stage")
			return printer.Print(rs)
		},
	}
}

// printFMReleaseStageDescribe renders the vertical describe view of a release
// stage.
func printFMReleaseStageDescribe(w io.Writer, project string, rs *featuremanagement.ReleaseStage) {
	const kw = 15
	output.FprintDescribeKV(w, "Key:", kw, "%s", rs.Key)
	output.FprintDescribeKV(w, "Name:", kw, "%s", rs.Name)
	output.FprintDescribeKV(w, "Project:", kw, "%s", project)
	if rs.Description != "" {
		output.FprintDescribeKV(w, "Description:", kw, "%s", rs.Description)
	}
	output.FprintDescribeKV(w, "Type:", kw, "%s", rs.Type)
	output.FprintDescribeKV(w, "Guarded:", kw, "%s", rs.GuardedDisplay)
	output.FprintDescribeKV(w, "Display Order:", kw, "%d", rs.DisplayOrder)
	output.FprintDescribeKV(w, "Version:", kw, "%d", rs.Version)
	printFMModificationInfo(w, kw, rs.ModificationInfo)
	if rs.ResourceContext != nil && len(rs.ResourceContext.Operations) > 0 {
		output.FprintDescribeKV(w, "Operations:", kw, "%s", strings.Join(rs.ResourceContext.Operations, ", "))
	}
}

// printFMModificationInfo prints the created and last-modified lines every
// Feature Management describe view ends with.
func printFMModificationInfo(w io.Writer, kw int, mi featuremanagement.ModificationInfo) {
	if !mi.CreatedTime.IsZero() {
		output.FprintDescribeKV(w, "Created:", kw, "%s by %s", mi.CreatedTime.UTC().Format("2006-01-02 15:04:05"), mi.CreatedBy)
	}
	if !mi.LastModifiedTime.IsZero() {
		output.FprintDescribeKV(w, "Last Modified:", kw, "%s by %s", mi.LastModifiedTime.UTC().Format("2006-01-02 15:04:05"), mi.LastModifiedBy)
	}
}
