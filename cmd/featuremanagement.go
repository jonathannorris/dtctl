package cmd

import (
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Dynatrace/dtctl/pkg/output"
	"github.com/Dynatrace/dtctl/pkg/resources/featuremanagement"
)

// featureManagementDevelopmentFeature is the opt-in key for the Feature
// Management command surface (docs/dev/FEATURE_MANAGEMENT_DESIGN.md). Enable
// it with `dtctl config set development.feature-management on` or
// DTCTL_DEVELOPMENT=feature-management. One key gates the `fm` group on every
// verb, so a single opt-in grants the whole surface.
const featureManagementDevelopmentFeature = "feature-management"

// The fm group's name and alias, shared by every verb's group.
const (
	fmGroupName  = "fm"
	fmGroupAlias = "feature-management"
)

// newFMGroupCmd builds one verb's `fm` group. The group only routes to its
// resources; its stability tier comes from addDevelopmentCommand, which
// covers every resource under it.
func newFMGroupCmd(short string) *cobra.Command {
	c := &cobra.Command{
		Use:     fmGroupName,
		Aliases: []string{fmGroupAlias},
		Short:   short,
		RunE:    requireSubcommand,
	}
	addFMProjectFlag(c)
	return c
}

var getFMCmd = newGetFMCmd()

func newGetFMCmd() *cobra.Command {
	c := newFMGroupCmd("Get Feature Management resources")
	c.AddCommand(newGetFMProjectsCmd())
	c.AddCommand(newGetFMReleaseStagesCmd())
	c.AddCommand(newGetFMFeaturesCmd())
	c.AddCommand(newGetFMFeatureTargetingCmd())
	c.AddCommand(newGetFMContextAttributesCmd())
	c.AddCommand(newGetFMSDKKeysCmd())
	c.AddCommand(newGetFMOverridesCmd())
	return c
}

var describeFMCmd = newDescribeFMCmd()

func newDescribeFMCmd() *cobra.Command {
	c := newFMGroupCmd("Show details of Feature Management resources")
	c.AddCommand(newDescribeFMProjectCmd())
	c.AddCommand(newDescribeFMReleaseStageCmd())
	c.AddCommand(newDescribeFMFeatureCmd())
	c.AddCommand(newDescribeFMFeatureTargetingCmd())
	c.AddCommand(newDescribeFMContextAttributeCmd())
	c.AddCommand(newDescribeFMOverridesCmd())
	return c
}

var createFMCmd = newCreateFMCmd()

func newCreateFMCmd() *cobra.Command {
	c := newFMGroupCmd("Create Feature Management resources")
	c.AddCommand(newCreateFMFeatureCmd())
	c.AddCommand(newCreateFMFeatureTargetingCmd())
	return c
}

var updateFMCmd = newUpdateFMCmd()

func newUpdateFMCmd() *cobra.Command {
	c := newFMGroupCmd("Update Feature Management resources")
	c.AddCommand(newUpdateFMFeatureCmd())
	c.AddCommand(newUpdateFMFeatureTargetingCmd())
	return c
}

var deleteFMCmd = newDeleteFMCmd()

func newDeleteFMCmd() *cobra.Command {
	c := newFMGroupCmd("Delete Feature Management resources")
	c.AddCommand(newDeleteFMFeatureCmd())
	c.AddCommand(newDeleteFMFeatureTargetingCmd())
	return c
}

var enableFMCmd = newEnableFMCmd()

func newEnableFMCmd() *cobra.Command {
	c := newFMGroupCmd("Turn Feature Management targeting on")
	c.AddCommand(newFMFeatureToggleCmd(true))
	return c
}

var disableFMCmd = newDisableFMCmd()

func newDisableFMCmd() *cobra.Command {
	c := newFMGroupCmd("Turn Feature Management targeting off")
	c.AddCommand(newFMFeatureToggleCmd(false))
	return c
}

func init() {
	// Feature Management is unfinished, so the whole `fm` group of each verb is
	// a development-tier feature: declared here, attached only when the opt-in
	// is on. The groups are not in newCommandTree, which omits every
	// development command (see its doc comment).
	addDevelopmentCommand(getCmd, getFMCmd, featureManagementDevelopmentFeature)
	addDevelopmentCommand(describeCmd, describeFMCmd, featureManagementDevelopmentFeature)
	addDevelopmentCommand(createCmd, createFMCmd, featureManagementDevelopmentFeature)
	addDevelopmentCommand(updateCmd, updateFMCmd, featureManagementDevelopmentFeature)
	addDevelopmentCommand(deleteCmd, deleteFMCmd, featureManagementDevelopmentFeature)
	addDevelopmentCommand(enableCmd, enableFMCmd, featureManagementDevelopmentFeature)
	addDevelopmentCommand(disableCmd, disableFMCmd, featureManagementDevelopmentFeature)
}

func newGetFMProjectsCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "projects [project-key]",
		Aliases: []string{"project"},
		Short:   "Get Feature Management projects",
		Long: `Get Feature Management projects.

A project is the container for features, release stages, context attributes
and SDK keys. Projects are identified by their key; a name is accepted too
when no project has it as a key.

Examples:
  # List all projects
  dtctl get fm projects

  # Get one project
  dtctl get fm project checkout

  # Wide output (adds description, maintainers, last modified, version)
  dtctl get fm projects -o wide

  # Output as JSON or YAML
  dtctl get fm projects -o json
  dtctl get fm projects -o yaml
`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, c, printer, err := setup(cmdContext(cmd))
			if err != nil {
				return err
			}
			h := featuremanagement.NewHandler(c)

			if len(args) > 0 {
				p, err := h.GetProject(args[0])
				if err != nil {
					return err
				}
				return printer.Print(p)
			}

			projects, err := h.ListProjects()
			if err != nil {
				return err
			}
			return printer.PrintList(projects)
		},
	}
}

func newDescribeFMProjectCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "project <project-key>",
		Aliases: []string{"projects"},
		Short:   "Show details of a Feature Management project",
		Long: `Show detailed information about a Feature Management project.

The argument is the project's key; a name is accepted too when no project
has it as a key.

Examples:
  # Describe a project
  dtctl describe fm project checkout

  # Output as JSON or YAML
  dtctl describe fm project checkout -o json
  dtctl describe fm project checkout -o yaml
`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, c, printer, err := setup(cmdContext(cmd))
			if err != nil {
				return err
			}

			p, err := featuremanagement.NewHandler(c).GetProject(args[0])
			if err != nil {
				return err
			}

			if useActionDescribeTextView(cmdContext(cmd)) {
				printFMProjectDescribe(currentStdout(cmdContext(cmd)), p)
				return nil
			}

			enrichAgent(printer, "describe", "fm/project")
			return printer.Print(p)
		},
	}
}

// printFMProjectDescribe renders the vertical describe view of a project.
func printFMProjectDescribe(w io.Writer, p *featuremanagement.Project) {
	const kw = 17
	output.FprintDescribeKV(w, "Key:", kw, "%s", p.Key)
	output.FprintDescribeKV(w, "Name:", kw, "%s", p.Name)
	if p.Description != "" {
		output.FprintDescribeKV(w, "Description:", kw, "%s", p.Description)
	}
	if p.Owner != "" {
		output.FprintDescribeKV(w, "Owner:", kw, "%s", p.Owner)
	}
	if len(p.Maintainers) > 0 {
		output.FprintDescribeKV(w, "Maintainers:", kw, "%s", strings.Join(p.Maintainers, ", "))
	}
	output.FprintDescribeKV(w, "Change Requests:", kw, "%s", p.ChangeRequestsDisplay)
	output.FprintDescribeKV(w, "Version:", kw, "%d", p.Version)

	printFMModificationInfo(w, kw, p.ModificationInfo)
	if p.ResourceContext != nil && len(p.ResourceContext.Operations) > 0 {
		output.FprintDescribeKV(w, "Operations:", kw, "%s", strings.Join(p.ResourceContext.Operations, ", "))
	}
}
