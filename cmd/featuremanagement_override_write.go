package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Dynatrace/dtctl/pkg/resources/featuremanagement"
	"github.com/Dynatrace/dtctl/pkg/safety"
)

// fmOverrideView is what an override write prints: the stage rows in table
// view, the API's shape otherwise.
func fmOverrideView(cmd *cobra.Command, o *featuremanagement.FeatureOverrides) any {
	if useFMTableView(cmdContext(cmd)) {
		return featuremanagement.FeatureOverrideRows(o)
	}
	return o
}

func newUpdateFMOverridesCmd() *cobra.Command {
	var (
		stage, variant  string
		clearOverride   bool
		setKeys         []string
		addKeys, rmKeys []string
	)
	c := &cobra.Command{
		Use:     "overrides [feature-key-or-name]",
		Aliases: []string{"override"},
		Short:   "Change your own variant overrides or targeting keys",
		Long: `Change your own variant overrides. They apply only to evaluations carrying one
of your targeting keys, so you can test a variant without changing targeting
for anyone else. They are yours alone, so a readwrite-mine context can change
them.

With a feature, --stage and --variant serve you that variant there; --clear
removes your override in the stage instead.

Without a feature, the targeting-key flags change your targeting keys in the
project: the values your SDK evaluations pass, case-sensitive.
--targeting-keys replaces them; --add-targeting-key and
--remove-targeting-key change one at a time.

Examples:
  dtctl update fm override new-checkout --stage staging --variant enabled
  dtctl update fm override new-checkout --stage staging --clear
  dtctl update fm overrides --add-targeting-key my-user-id
  dtctl update fm overrides --targeting-keys my-user-id,my-device-id
`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmdContext(cmd)
			keyFlags := changedAny(cmd, "targeting-keys", "add-targeting-key", "remove-targeting-key")
			overrideFlags := changedAny(cmd, "stage", "variant", "clear")
			if len(args) == 0 {
				if overrideFlags {
					return fmt.Errorf("--stage, --variant and --clear need a feature: dtctl update fm override <feature> --stage <stage> --variant <variant>")
				}
				if !keyFlags {
					return fmt.Errorf("nothing to update: pass a feature with --stage and --variant or --clear, or --targeting-keys, --add-targeting-key or --remove-targeting-key")
				}
			} else {
				if keyFlags {
					return fmt.Errorf("targeting keys belong to the project, not a feature; change them without a feature argument")
				}
				if strings.TrimSpace(stage) == "" {
					return fmt.Errorf("--stage is required with a feature")
				}
				if clearOverride == (variant != "") {
					return fmt.Errorf("pass exactly one of --variant and --clear")
				}
			}
			_, project, h, printer, err := setupFMWriteAs(cmd, safety.OperationUpdate, safety.OwnershipOwn)
			if err != nil {
				return err
			}
			if len(args) == 0 {
				change := featuremanagement.TargetingKeysChange{Add: addKeys, Remove: rmKeys}
				if cmd.Flags().Changed("targeting-keys") {
					change.Set = &setKeys
				}
				keys, changed, err := h.UpdateTargetingKeys(project, change)
				if err != nil {
					return err
				}
				msg := fmt.Sprintf("Updated your targeting keys in project %s", project)
				if !changed {
					msg = fmt.Sprintf("Nothing to change: your targeting keys in project %s already are as requested", project)
				}
				var view any = map[string][]string{"targetingKeys": keys}
				if useFMTableView(ctx) {
					view = featuremanagement.TargetingKeyRows(keys)
				}
				return printFMWrite(ctx, printer, "update", "fm/override", msg, changed, nil, view)
			}
			var v *string
			if !clearOverride {
				v = &variant
			}
			res, err := h.SetOverride(project, args[0], stage, v)
			if err != nil {
				return err
			}
			msg := fmt.Sprintf("Updated your override of feature %s in stage %s", res.Overrides.FeatureKey, stage)
			if !res.Changed {
				msg = fmt.Sprintf("Nothing to change: your override of feature %s in stage %s already is as requested", res.Overrides.FeatureKey, stage)
			}
			return printFMWrite(ctx, printer, "update", "fm/override", msg, res.Changed, nil, fmOverrideView(cmd, res.Overrides))
		},
	}
	c.Flags().SortFlags = false
	c.Flags().StringVarP(&stage, fmStageFlag, "s", "", "release stage of the override")
	c.Flags().StringVar(&variant, "variant", "", "variant to serve you in the stage")
	c.Flags().BoolVar(&clearOverride, "clear", false, "remove your override in the stage")
	c.Flags().StringSliceVar(&setKeys, "targeting-keys", nil, "replace your targeting keys (comma-separated; empty clears them)")
	c.Flags().StringArrayVar(&addKeys, "add-targeting-key", nil, "add a targeting key (repeatable)")
	c.Flags().StringArrayVar(&rmKeys, "remove-targeting-key", nil, "remove a targeting key (repeatable)")
	return c
}

func newDeleteFMOverridesCmd() *cobra.Command {
	var (
		yes, all, keys bool
	)
	c := &cobra.Command{
		Use:     "overrides [feature-key-or-name]",
		Aliases: []string{"override"},
		Short:   "Delete your own variant overrides or targeting keys",
		Long: `Delete your own variant overrides: of one feature in every stage, or with
--all every override you have in the project. --targeting-keys deletes your
targeting keys in the project, after which your overrides match no
evaluation. They are yours alone, so a readwrite-mine context can delete
them.

To remove your override in one stage only, use
'dtctl update fm override <feature> --stage <stage> --clear'. It asks for
confirmation unless -y is given; without a terminal to ask on, it needs -y.

Examples:
  dtctl delete fm override new-checkout
  dtctl delete fm overrides --all
  dtctl delete fm overrides --all --targeting-keys -y
`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmdContext(cmd)
			if len(args) > 0 && (all || keys) {
				return fmt.Errorf("--all and --targeting-keys act on the whole project; drop the feature argument")
			}
			if len(args) == 0 && !all && !keys {
				return fmt.Errorf("pass a feature, --all for every override in the project, or --targeting-keys")
			}
			var what []string
			switch {
			case len(args) > 0:
				what = append(what, fmt.Sprintf("your overrides of feature %s", args[0]))
			case all:
				what = append(what, "every override you have in the project")
			}
			if keys {
				what = append(what, "your targeting keys in the project")
			}
			ok, err := confirmFMDelete(ctx, yes, strings.Join(what, " and "))
			if err != nil || !ok {
				return err
			}
			_, project, h, printer, err := setupFMWriteAs(cmd, safety.OperationDelete, safety.OwnershipOwn)
			if err != nil {
				return err
			}
			deleted := map[string]string{"projectKey": project}
			if len(args) > 0 {
				res, err := h.ClearOverrides(project, args[0])
				if err != nil {
					return err
				}
				deleted["featureKey"] = res.Overrides.FeatureKey
				return printFMDeleted(ctx, printer, "fm/override", fmt.Sprintf("Deleted your overrides of feature %s", res.Overrides.FeatureKey), deleted)
			}
			var done []string
			if all {
				if err := h.DeleteAllOverrides(project); err != nil {
					return err
				}
				deleted["overrides"] = "all"
				done = append(done, "every override you had")
			}
			if keys {
				if err := h.DeleteTargetingKeys(project); err != nil {
					return err
				}
				deleted["targetingKeys"] = "all"
				done = append(done, "your targeting keys")
			}
			return printFMDeleted(ctx, printer, "fm/override", fmt.Sprintf("Deleted %s in project %s", strings.Join(done, " and "), project), deleted)
		},
	}
	c.Flags().BoolVar(&all, "all", false, "delete every override you have in the project")
	c.Flags().BoolVar(&keys, "targeting-keys", false, "delete your targeting keys in the project")
	addFMYesFlag(c, &yes)
	return c
}
