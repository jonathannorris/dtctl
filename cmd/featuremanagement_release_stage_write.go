package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Dynatrace/dtctl/pkg/resources/featuremanagement"
	"github.com/Dynatrace/dtctl/pkg/safety"
)

var fmReleaseStageTypes = []string{"DEVELOPMENT", "STAGING", "PRODUCTION"}

func newCreateFMReleaseStageCmd() *cobra.Command {
	var (
		file, name, description, stageType string
		guarded                            bool
		displayOrder                       int
	)
	c := &cobra.Command{
		Use:     "release-stage <stage-key> | -f <file>",
		Aliases: []string{"release-stages", "stage", "stages"},
		Short:   "Create a release stage",
		Long: `Create a release stage: an environment features are released through, such
as development, staging or production. Every feature starts off in it, with
no targeting.

A guarded stage needs an approved change request for every targeting change
when the project has change requests on.

-f reads the API's ReleaseStageCreate fields from YAML or JSON, so a stage
exported with 'dtctl get fm release-stage <key> -o yaml' creates a copy. The
file's projectKey, if present, must be the target project.

Examples:
  dtctl create fm release-stage qa --name QA --type STAGING
  dtctl create fm release-stage production-eu --name "Production EU" --type PRODUCTION --guarded
  dtctl create fm release-stage -f stage.yaml
`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmdContext(cmd)
			if file != "" && changedAny(cmd, "name", "description", "type", "guarded", "display-order") {
				return fmt.Errorf("-f cannot be combined with the field flags; put the fields in the file")
			}
			var data []byte
			if file != "" {
				var err error
				if data, err = readFileFlag(ctx, "file", file); err != nil {
					return err
				}
			} else {
				if len(args) == 0 {
					return fmt.Errorf("pass a release stage key, or -f <file>")
				}
				if name == "" || stageType == "" {
					return fmt.Errorf("--name and --type are required (or use -f <file>)")
				}
			}
			_, project, h, printer, err := setupFMWrite(cmd, safety.OperationUpdate)
			if err != nil {
				return err
			}
			var body featuremanagement.ReleaseStageCreate
			if file != "" {
				if err := featuremanagement.ReleaseStageFile(project).Decode(data, &body); err != nil {
					return fmt.Errorf("%s: %w", sourceName(file), err)
				}
				if len(args) > 0 {
					if body.Key != "" && body.Key != args[0] {
						return fmt.Errorf("the file creates release stage %q, not %q", body.Key, args[0])
					}
					body.Key = args[0]
				}
				if body.Key == "" {
					return fmt.Errorf("the file has no key; add one or pass it as an argument")
				}
			} else {
				t, err := fmEnum("type", stageType, fmReleaseStageTypes)
				if err != nil {
					return err
				}
				body = featuremanagement.ReleaseStageCreate{Key: args[0], Name: name, Description: description, Type: featuremanagement.ReleaseStageType(t)}
				if cmd.Flags().Changed("guarded") {
					body.Guarded = &guarded
				}
				if cmd.Flags().Changed("display-order") {
					body.DisplayOrder = &displayOrder
				}
			}
			res, err := h.CreateReleaseStage(project, body)
			if err != nil {
				return err
			}
			return printFMWrite(ctx, printer, "create", "fm/release-stage",
				fmt.Sprintf("Created release stage %s in project %s", res.Stage.Key, project), true, nil, res.Stage)
		},
	}
	c.Flags().SortFlags = false
	c.Flags().StringVarP(&file, "file", "f", "", "file containing the release stage (YAML or JSON)")
	c.Flags().StringVar(&name, "name", "", "display name (required unless -f is given)")
	c.Flags().StringVar(&stageType, "type", "", "DEVELOPMENT|STAGING|PRODUCTION (required unless -f is given)")
	c.Flags().StringVar(&description, "description", "", "release stage description")
	c.Flags().BoolVar(&guarded, "guarded", false, "require approved change requests for targeting changes")
	c.Flags().IntVar(&displayOrder, "display-order", 0, "order among the stages of its type, 0-100 (default first)")
	markFileFlag(c, "file", fileValuePath)
	_ = c.RegisterFlagCompletionFunc("type", cobra.FixedCompletions(fmReleaseStageTypes, cobra.ShellCompDirectiveNoFileComp))
	return c
}

func newUpdateFMReleaseStageCmd() *cobra.Command {
	var (
		name, description, stageType, version string
		guarded                               bool
		displayOrder                          int
		order                                 []string
	)
	c := &cobra.Command{
		Use:     "release-stage <stage-key-or-name> | --order <stage>,...",
		Aliases: []string{"release-stages", "stage", "stages"},
		Short:   "Change a release stage, or the order of all of them",
		Long: `Change a release stage's fields, or with --order and no stage, the order of
the project's release stages.

A field update is a read-modify-write that sends back everything it read,
with the version it read; a change that leaves the stage as it was sends
nothing.

--order names every stage of the project, and each gets its position as its
display order. Display order ranks stages within their environment type, so
the list orders the stages of each type among themselves.

Examples:
  dtctl update fm release-stage production --guarded
  dtctl update fm release-stage qa --name "Quality assurance"
  dtctl update fm release-stages --order development,qa,staging,production
`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmdContext(cmd)
			fields := changedAny(cmd, "name", "description", "type", "guarded", "display-order")
			reorder := cmd.Flags().Changed("order")
			switch {
			case reorder && (fields || len(args) > 0 || version != ""):
				return fmt.Errorf("--order changes every stage's order and takes no stage, field flags or --version; each stage's version is the one it is read at")
			case !reorder && len(args) == 0:
				return fmt.Errorf("pass a release stage, or --order <stage>,... to reorder them all")
			case !reorder && !fields:
				return fmt.Errorf("nothing to update: pass --name, --description, --type, --guarded or --display-order")
			}
			var change featuremanagement.ReleaseStageChange
			if cmd.Flags().Changed("name") {
				change.Name = &name
			}
			if cmd.Flags().Changed("description") {
				change.Description = &description
			}
			if cmd.Flags().Changed("type") {
				t, err := fmEnum("type", stageType, fmReleaseStageTypes)
				if err != nil {
					return err
				}
				rt := featuremanagement.ReleaseStageType(t)
				change.Type = &rt
			}
			if cmd.Flags().Changed("guarded") {
				change.Guarded = &guarded
			}
			if cmd.Flags().Changed("display-order") {
				change.DisplayOrder = &displayOrder
			}
			_, project, h, printer, err := setupFMWrite(cmd, safety.OperationUpdate)
			if err != nil {
				return err
			}
			if reorder {
				stages, changed, err := h.ReorderReleaseStages(project, order)
				if err != nil {
					return err
				}
				msg := fmt.Sprintf("Reordered the release stages of project %s", project)
				if !changed {
					msg = fmt.Sprintf("Nothing to change: the release stages of project %s already are in that order", project)
				}
				return printFMWrite(ctx, printer, "update", "fm/release-stage", msg, changed, nil, stages)
			}
			res, err := h.UpdateReleaseStage(project, args[0], change, featuremanagement.WriteOptions{Version: version})
			if err != nil {
				return err
			}
			msg := fmt.Sprintf("Updated release stage %s", res.Stage.Key)
			if !res.Changed {
				msg = fmt.Sprintf("Nothing to change: release stage %s already is as requested", res.Stage.Key)
			}
			return printFMWrite(ctx, printer, "update", "fm/release-stage", msg, res.Changed, nil, res.Stage)
		},
	}
	c.Flags().SortFlags = false
	c.Flags().StringVar(&name, "name", "", "new display name")
	c.Flags().StringVar(&description, "description", "", "new description")
	c.Flags().StringVar(&stageType, "type", "", "DEVELOPMENT|STAGING|PRODUCTION")
	c.Flags().BoolVar(&guarded, "guarded", false, "require approved change requests for targeting changes")
	c.Flags().IntVar(&displayOrder, "display-order", 0, "order among the stages of its type, 0-100")
	c.Flags().StringSliceVar(&order, "order", nil, "every stage key, comma-separated, in the new order")
	c.Flags().StringVar(&version, "version", "", fmVersionHelp)
	_ = c.RegisterFlagCompletionFunc("type", cobra.FixedCompletions(fmReleaseStageTypes, cobra.ShellCompDirectiveNoFileComp))
	return c
}

func newDeleteFMReleaseStageCmd() *cobra.Command {
	var yes bool
	c := &cobra.Command{
		Use:     "release-stage <stage-key-or-name>",
		Aliases: []string{"release-stages", "stage", "stages"},
		Short:   "Delete a release stage",
		Long: `Delete a release stage with its SDK keys and every feature's targeting in
it. Applications using its SDK keys stop receiving flags.

Because it takes the stage's SDK keys and targeting along, it needs a context
at the dangerously-unrestricted safety level. It asks for confirmation
unless -y is given; without a terminal to ask on, it needs -y.

Examples:
  dtctl delete fm release-stage qa
`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmdContext(cmd)
			ok, err := confirmFMDelete(ctx, yes, fmt.Sprintf("release stage %s with its SDK keys and every feature's targeting in it", args[0]))
			if err != nil || !ok {
				return err
			}
			_, project, h, printer, err := setupFMWrite(cmd, safety.OperationDeleteBucket)
			if err != nil {
				return err
			}
			key, err := h.DeleteReleaseStage(project, args[0])
			if err != nil {
				return err
			}
			return printFMDeleted(ctx, printer, "fm/release-stage", fmt.Sprintf("Deleted release stage %s from project %s", key, project),
				map[string]string{"projectKey": project, "key": key})
		},
	}
	addFMYesFlag(c, &yes)
	return c
}
