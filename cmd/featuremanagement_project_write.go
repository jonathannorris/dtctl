package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Dynatrace/dtctl/pkg/output"
	"github.com/Dynatrace/dtctl/pkg/resources/featuremanagement"
	"github.com/Dynatrace/dtctl/pkg/safety"
)

// setupFMProjectWrite is setupFMWrite for a project write, which names its
// project as its argument rather than taking --project or the context's.
func setupFMProjectWrite(cmd *cobra.Command, op safety.Operation) (*featuremanagement.Handler, output.Printer, error) {
	ctx := cmdContext(cmd)
	cfg, err := loadConfig(ctx)
	if err != nil {
		return nil, nil, err
	}
	if err := checkFMSafety(ctx, cfg, op, safety.OwnershipShared); err != nil {
		return nil, nil, err
	}
	return newFMWriteHandler(cmd, cfg)
}

func newCreateFMProjectCmd() *cobra.Command {
	var (
		file, name, description, owner string
		maintainers                    []string
		changeRequests                 bool
	)
	c := &cobra.Command{
		Use:     "project <project-key> | -f <file>",
		Aliases: []string{"projects"},
		Short:   "Create a project",
		Long: `Create a Feature Management project: the container for features, release
stages, context attributes and SDK keys.

-f reads the API's ProjectCreate fields from YAML or JSON, so a project
exported with 'dtctl get fm project <key> -o yaml' creates a copy.

Examples:
  dtctl create fm project checkout --name "Checkout"
  dtctl create fm project checkout --name "Checkout" --owner checkout-team --change-requests
  dtctl create fm project -f project.yaml
`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmdContext(cmd)
			if file != "" && changedAny(cmd, "name", "description", "owner", "maintainers", "change-requests") {
				return fmt.Errorf("-f cannot be combined with the field flags; put the fields in the file")
			}
			var body featuremanagement.ProjectCreate
			if file != "" {
				data, err := readFileFlag(ctx, "file", file)
				if err != nil {
					return err
				}
				if err := featuremanagement.ProjectFile().Decode(data, &body); err != nil {
					return fmt.Errorf("%s: %w", sourceName(file), err)
				}
				if len(args) > 0 {
					if body.Key != "" && body.Key != args[0] {
						return fmt.Errorf("the file creates project %q, not %q", body.Key, args[0])
					}
					body.Key = args[0]
				}
				if body.Key == "" {
					return fmt.Errorf("the file has no key; add one or pass it as an argument")
				}
			} else {
				if len(args) == 0 {
					return fmt.Errorf("pass a project key, or -f <file>")
				}
				if name == "" {
					return fmt.Errorf("--name is required (or use -f <file>)")
				}
				body = featuremanagement.ProjectCreate{Key: args[0], Name: name, Description: description, Owner: owner, Maintainers: maintainers}
				if cmd.Flags().Changed("change-requests") {
					body.Settings = &featuremanagement.ProjectSettings{ChangeRequests: &featuremanagement.ChangeRequestSettings{Enabled: &changeRequests}}
				}
			}
			h, printer, err := setupFMProjectWrite(cmd, safety.OperationUpdate)
			if err != nil {
				return err
			}
			res, err := h.CreateProject(body)
			if err != nil {
				return err
			}
			return printFMWrite(ctx, printer, "create", "fm/project", fmt.Sprintf("Created project %s", res.Project.Key), true, nil, res.Project)
		},
	}
	c.Flags().SortFlags = false
	c.Flags().StringVarP(&file, "file", "f", "", "file containing the project (YAML or JSON)")
	c.Flags().StringVar(&name, "name", "", "display name (required unless -f is given)")
	c.Flags().StringVar(&description, "description", "", "project description")
	c.Flags().StringVar(&owner, "owner", "", "project owner")
	c.Flags().StringSliceVar(&maintainers, "maintainers", nil, "comma-separated maintainers")
	c.Flags().BoolVar(&changeRequests, "change-requests", false, "require change requests for targeting in guarded release stages")
	markFileFlag(c, "file", fileValuePath)
	return c
}

func newUpdateFMProjectCmd() *cobra.Command {
	var (
		name, description, owner, version string
		maintainers                       []string
		changeRequests                    bool
	)
	c := &cobra.Command{
		Use:     "project <project-key-or-name>",
		Aliases: []string{"projects"},
		Short:   "Change a project's fields or settings",
		Long: `Change a project. The update is a read-modify-write that sends back
everything it read, settings this dtctl doesn't know included, with the
version it read; a change that leaves the project as it was sends nothing.

Examples:
  dtctl update fm project checkout --owner checkout-team --maintainers alice,bob
  dtctl update fm project checkout --change-requests=false
`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmdContext(cmd)
			if !changedAny(cmd, "name", "description", "owner", "maintainers", "change-requests") {
				return fmt.Errorf("nothing to update: pass --name, --description, --owner, --maintainers or --change-requests")
			}
			var change featuremanagement.ProjectChange
			if cmd.Flags().Changed("name") {
				change.Name = &name
			}
			if cmd.Flags().Changed("description") {
				change.Description = &description
			}
			if cmd.Flags().Changed("owner") {
				change.Owner = &owner
			}
			if cmd.Flags().Changed("maintainers") {
				change.Maintainers = &maintainers
			}
			if cmd.Flags().Changed("change-requests") {
				change.ChangeRequests = &changeRequests
			}
			h, printer, err := setupFMProjectWrite(cmd, safety.OperationUpdate)
			if err != nil {
				return err
			}
			res, err := h.UpdateProject(args[0], change, featuremanagement.WriteOptions{Version: version})
			if err != nil {
				return err
			}
			msg := fmt.Sprintf("Updated project %s", res.Project.Key)
			if !res.Changed {
				msg = fmt.Sprintf("Nothing to change: project %s already is as requested", res.Project.Key)
			}
			return printFMWrite(ctx, printer, "update", "fm/project", msg, res.Changed, nil, res.Project)
		},
	}
	c.Flags().SortFlags = false
	c.Flags().StringVar(&name, "name", "", "new display name")
	c.Flags().StringVar(&description, "description", "", "new description")
	c.Flags().StringVar(&owner, "owner", "", "new owner")
	c.Flags().StringSliceVar(&maintainers, "maintainers", nil, "replace the maintainers (comma-separated)")
	c.Flags().BoolVar(&changeRequests, "change-requests", false, "require change requests for targeting in guarded release stages")
	c.Flags().StringVar(&version, "version", "", fmVersionHelp)
	return c
}

func newDeleteFMProjectCmd() *cobra.Command {
	var yes bool
	c := &cobra.Command{
		Use:     "project <project-key-or-name>",
		Aliases: []string{"projects"},
		Short:   "Delete a project and everything in it",
		Long: `Delete a project with every feature, release stage, context attribute and
SDK key in it. Applications using its SDK keys stop receiving flags.

Because it takes everything in the project along, it needs a context at the
dangerously-unrestricted safety level. It asks for confirmation unless -y is
given; without a terminal to ask on, it needs -y.

Examples:
  dtctl delete fm project checkout
`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmdContext(cmd)
			ok, err := confirmFMDelete(ctx, yes, fmt.Sprintf("project %s with every feature, release stage, context attribute and SDK key in it", args[0]))
			if err != nil || !ok {
				return err
			}
			h, printer, err := setupFMProjectWrite(cmd, safety.OperationDeleteBucket)
			if err != nil {
				return err
			}
			key, err := h.DeleteProject(args[0])
			if err != nil {
				return err
			}
			return printFMDeleted(ctx, printer, "fm/project", fmt.Sprintf("Deleted project %s", key), map[string]string{"key": key})
		},
	}
	addFMYesFlag(c, &yes)
	return c
}
