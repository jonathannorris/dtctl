package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Dynatrace/dtctl/pkg/resources/featuremanagement"
	"github.com/Dynatrace/dtctl/pkg/safety"
)

var (
	fmFeatureTypes  = []string{"RELEASE", "CONFIGURATION", "ENTITLEMENTS", "EXPERIMENT", "OPS"}
	fmFlagTypes     = []string{"BOOLEAN", "STRING", "NUMBER", "JSON"}
	fmFeatureStates = []string{"ACTIVE", "ARCHIVED"}
)

// fmEnum checks a value against an enum, case-insensitively, and returns it
// in the API's spelling.
func fmEnum(flag, value string, allowed []string) (string, error) {
	for _, a := range allowed {
		if strings.EqualFold(value, a) {
			return a, nil
		}
	}
	return "", fmt.Errorf("invalid --%s %q: must be one of %s", flag, value, strings.Join(allowed, ", "))
}

func newCreateFMFeatureCmd() *cobra.Command {
	var (
		file, name, description, owner, featureType, flagKey, flagType string
		maintainers, variants, flagValues                              []string
	)
	c := &cobra.Command{
		Use:     "feature <feature-key> | -f <file>",
		Aliases: []string{"features"},
		Short:   "Create a feature",
		Long: `Create a feature from flags, with one flag, or from a file with several.

From flags, the feature gets the variants --variants names (default
disabled,enabled), each named after its key, and one flag: a BOOLEAN keyed
like the feature unless --flag-key and --flag-type say otherwise. Give the
flag a value for every variant with --flag-value <variant>=<value>, or none
for the type's defaults: the last variant gets true, "Enabled", 100 or
{"enabled": true}, every other variant the "off" value.

-f reads the API's FeatureCreate fields from YAML or JSON, so a feature
exported with 'dtctl get fm feature <key> -o yaml' creates a copy. A flags
list there can carry names, descriptions and schemas. The file's projectKey,
if present, must be the target project.

Examples:
  # A release toggle with the default variants and a BOOLEAN flag
  dtctl create fm feature new-checkout --name "New checkout"

  # A STRING flag with a value per variant
  dtctl create fm feature new-checkout --name "New checkout" --variants baseline,variation-a \
    --flag-key checkout-layout --flag-type STRING \
    --flag-value baseline=classic --flag-value variation-a=one-page

  # From a file
  dtctl create fm feature -f feature.yaml
`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmdContext(cmd)
			fieldFlags := []string{"name", "description", "owner", "maintainers", "type", "variants", "flag-key", "flag-type", "flag-value"}
			if file != "" && changedAny(cmd, fieldFlags...) {
				return fmt.Errorf("-f cannot be combined with the field flags; put the fields in the file")
			}
			if file == "" {
				if len(args) == 0 {
					return fmt.Errorf("pass a feature key, or -f <file>")
				}
				if name == "" {
					return fmt.Errorf("--name is required (or use -f <file>)")
				}
			}
			// The file is read before the safety check and the client, so a
			// bad path fails without a round trip.
			var data []byte
			if file != "" {
				var err error
				if data, err = readFileFlag(ctx, "file", file); err != nil {
					return err
				}
			}
			_, project, h, printer, err := setupFMWrite(cmd, safety.OperationUpdate)
			if err != nil {
				return err
			}
			var body featuremanagement.FeatureCreate
			if file != "" {
				if err := featuremanagement.FeatureFile(project).Decode(data, &body); err != nil {
					return fmt.Errorf("%s: %w", sourceName(file), err)
				}
				if len(args) > 0 {
					if body.Key != "" && body.Key != args[0] {
						return fmt.Errorf("the file creates feature %q, not %q", body.Key, args[0])
					}
					body.Key = args[0]
				}
				if body.Key == "" {
					return fmt.Errorf("the file has no key; add one or pass it as an argument")
				}
			} else {
				spec := featuremanagement.FeatureCreateSpec{
					Key: args[0], Name: name, Description: description, Owner: owner, Maintainers: maintainers,
					Variants: variants, FlagKey: flagKey, FlagValues: flagValues,
				}
				if featureType != "" {
					t, err := fmEnum("type", featureType, fmFeatureTypes)
					if err != nil {
						return err
					}
					spec.Type = featuremanagement.FeatureType(t)
				}
				if flagType != "" {
					t, err := fmEnum("flag-type", flagType, fmFlagTypes)
					if err != nil {
						return err
					}
					spec.FlagType = featuremanagement.FlagType(t)
				}
				if body, err = spec.Body(); err != nil {
					return err
				}
			}
			res, err := h.CreateFeature(project, body)
			if err != nil {
				return err
			}
			return printFMWrite(ctx, printer, "create", "fm/feature",
				fmt.Sprintf("Created feature %s in project %s", res.Feature.Key, project), true, nil, res.Feature)
		},
	}
	c.Flags().SortFlags = false
	c.Flags().StringVarP(&file, "file", "f", "", "file containing the feature, including variants and flags (YAML or JSON)")
	c.Flags().StringVar(&name, "name", "", "display name (required unless -f is given)")
	c.Flags().StringVar(&description, "description", "", "feature description")
	c.Flags().StringVar(&featureType, "type", "", "RELEASE|CONFIGURATION|ENTITLEMENTS|EXPERIMENT|OPS (default RELEASE)")
	c.Flags().StringSliceVar(&variants, "variants", nil, "comma-separated variant keys, in display order (default disabled,enabled)")
	c.Flags().StringVar(&flagKey, "flag-key", "", "key of the initial flag (default: the feature key)")
	c.Flags().StringVar(&flagType, "flag-type", "", "type of the initial flag: BOOLEAN|STRING|NUMBER|JSON (default BOOLEAN)")
	c.Flags().StringArrayVar(&flagValues, "flag-value", nil, "the initial flag's value for one variant, as <variant>=<value> (repeatable; one per variant, or none)")
	c.Flags().StringVar(&owner, "owner", "", "feature owner")
	c.Flags().StringSliceVar(&maintainers, "maintainers", nil, "comma-separated maintainers")
	markFileFlag(c, "file", fileValuePath)
	_ = c.RegisterFlagCompletionFunc("type", cobra.FixedCompletions(fmFeatureTypes, cobra.ShellCompDirectiveNoFileComp))
	_ = c.RegisterFlagCompletionFunc("flag-type", cobra.FixedCompletions(fmFlagTypes, cobra.ShellCompDirectiveNoFileComp))
	return c
}

func newUpdateFMFeatureCmd() *cobra.Command {
	var (
		name, description, owner, featureType, status, version   string
		maintainers                                              []string
		addVariant, variantName, removeVariant                   string
		displayOrder                                             int
		values, flagValues                                       []string
		addFlag, flagType, flagName, flagDescription, removeFlag string
	)
	c := &cobra.Command{
		Use:     "feature <feature-key-or-name>",
		Aliases: []string{"features"},
		Short:   "Change a feature's status, descriptive fields, variants or flags",
		Long: `Change a feature. Each command does one of three kinds of change, since each
uses its own endpoint and scope:

  descriptive  --name, --description, --type, --owner, --maintainers
               the metadata endpoint (features.metadata:write), which can't
               change what the feature serves
  status       --status ACTIVE|ARCHIVED (an archived feature leaves the
               flag configuration SDKs fetch)
  structure    --add-variant, --remove-variant, --add-flag, --remove-flag,
               combinable in one write: the full feature update
               (features:write)

Every change is a read-modify-write that sends back everything it read,
flag schemas and fields this dtctl doesn't know included, with the version
it read; a change that leaves the feature as it was sends nothing.

--add-variant needs every flag's value for the new variant, one
--variant-value <flag-key>=<value> per flag, parsed by the flag's type. Adding a
variant serves it to nobody until a targeting rule does.

--add-flag takes --flag-value <variant>=<value> for every variant, or none
for the type's defaults (the last variant on, the rest off). A flag with a
schema needs 'dtctl create fm feature -f' or apply.

--remove-variant is refused while an enabled stage serves the variant.
Stages whose rules still name it are listed as a warning afterwards.

Examples:
  dtctl update fm feature new-checkout --status ARCHIVED
  dtctl update fm feature new-checkout --owner checkout-team --maintainers alice,bob
  dtctl update fm feature new-checkout --add-variant variation-b \
    --variant-value new-checkout=true --variant-value checkout-layout=express
  dtctl update fm feature new-checkout --add-flag checkout-theme --flag-type STRING \
    --flag-value baseline=light --flag-value variation-a=dark
  dtctl update fm feature new-checkout --remove-flag checkout-theme
`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmdContext(cmd)
			descriptive := changedAny(cmd, "name", "description", "type", "owner", "maintainers")
			statusChange := cmd.Flags().Changed("status")
			structure := changedAny(cmd, "add-variant", "remove-variant", "add-flag", "remove-flag",
				"variant-name", "variant-display-order", "variant-value", "flag-type", "flag-name", "flag-description", "flag-value")
			kinds := 0
			for _, k := range []bool{descriptive, statusChange, structure} {
				if k {
					kinds++
				}
			}
			switch {
			case kinds == 0:
				return fmt.Errorf("nothing to update: pass descriptive flags, --status, or --add-variant/--remove-variant/--add-flag/--remove-flag")
			case kinds > 1:
				return fmt.Errorf("descriptive flags, --status and the variant and flag options each use their own endpoint; run them as separate commands")
			}
			_, project, h, printer, err := setupFMWrite(cmd, safety.OperationUpdate)
			if err != nil {
				return err
			}
			opts := featuremanagement.WriteOptions{Version: version}
			var res *featuremanagement.FeatureWriteResult
			switch {
			case statusChange:
				s, err := fmEnum("status", status, fmFeatureStates)
				if err != nil {
					return err
				}
				res, err = h.SetFeatureStatus(project, args[0], featuremanagement.FeatureStatus(s), opts)
				if err != nil {
					return err
				}
			case descriptive:
				var change featuremanagement.FeatureMetadataChange
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
				if cmd.Flags().Changed("type") {
					t, err := fmEnum("type", featureType, fmFeatureTypes)
					if err != nil {
						return err
					}
					ft := featuremanagement.FeatureType(t)
					change.Type = &ft
				}
				res, err = h.UpdateFeatureMetadata(project, args[0], change, opts)
				if err != nil {
					return err
				}
			default:
				change := featuremanagement.VariantFlagChange{
					AddVariant: addVariant, VariantName: variantName, Values: values, RemoveVariant: removeVariant,
					AddFlag: addFlag, FlagName: flagName, FlagDescription: flagDescription, FlagValues: flagValues, RemoveFlag: removeFlag,
				}
				if cmd.Flags().Changed("variant-display-order") {
					change.DisplayOrder = &displayOrder
				}
				if flagType != "" {
					t, err := fmEnum("flag-type", flagType, fmFlagTypes)
					if err != nil {
						return err
					}
					change.FlagType = featuremanagement.FlagType(t)
				}
				res, err = h.ChangeFeatureVariantsAndFlags(project, args[0], change, opts)
				if err != nil {
					return err
				}
			}
			msg := fmt.Sprintf("Updated feature %s", res.Feature.Key)
			if !res.Changed {
				msg = fmt.Sprintf("Nothing to change: feature %s already is as requested", res.Feature.Key)
			}
			return printFMWrite(ctx, printer, "update", "fm/feature", msg, res.Changed, res.Warnings, res.Feature)
		},
	}
	c.Flags().SortFlags = false
	c.Flags().StringVar(&name, "name", "", "new display name")
	c.Flags().StringVar(&description, "description", "", "new description")
	c.Flags().StringVar(&featureType, "type", "", "RELEASE|CONFIGURATION|ENTITLEMENTS|EXPERIMENT|OPS")
	c.Flags().StringVar(&owner, "owner", "", "new owner")
	c.Flags().StringSliceVar(&maintainers, "maintainers", nil, "replace the maintainers (comma-separated)")
	c.Flags().StringVar(&status, "status", "", "ACTIVE|ARCHIVED")
	c.Flags().StringVar(&addVariant, "add-variant", "", "add a variant with this key")
	c.Flags().StringVar(&variantName, "variant-name", "", "display name for --add-variant (default: the key)")
	c.Flags().IntVar(&displayOrder, "variant-display-order", 0, "display order for --add-variant (default: last)")
	c.Flags().StringArrayVar(&values, "variant-value", nil, "each flag's value for the new variant, as <flag-key>=<value>; one per flag (repeatable)")
	c.Flags().StringVar(&removeVariant, "remove-variant", "", "remove a variant and its value on every flag")
	c.Flags().StringVar(&addFlag, "add-flag", "", "add a flag with this key")
	c.Flags().StringVar(&flagType, "flag-type", "", "type for --add-flag: BOOLEAN|STRING|NUMBER|JSON (default BOOLEAN)")
	c.Flags().StringVar(&flagName, "flag-name", "", "display name for --add-flag (default: the key)")
	c.Flags().StringVar(&flagDescription, "flag-description", "", "description for --add-flag")
	c.Flags().StringArrayVar(&flagValues, "flag-value", nil, "the new flag's value for one variant, as <variant>=<value> (repeatable; one per variant, or none)")
	c.Flags().StringVar(&removeFlag, "remove-flag", "", "remove a flag; SDKs that evaluate it get the default in their code")
	c.Flags().StringVar(&version, "version", "", fmVersionHelp)
	_ = c.RegisterFlagCompletionFunc("status", cobra.FixedCompletions(fmFeatureStates, cobra.ShellCompDirectiveNoFileComp))
	_ = c.RegisterFlagCompletionFunc("type", cobra.FixedCompletions(fmFeatureTypes, cobra.ShellCompDirectiveNoFileComp))
	_ = c.RegisterFlagCompletionFunc("flag-type", cobra.FixedCompletions(fmFlagTypes, cobra.ShellCompDirectiveNoFileComp))
	return c
}

func newDeleteFMFeatureCmd() *cobra.Command {
	var yes bool
	c := &cobra.Command{
		Use:     "feature <feature-key-or-name>",
		Aliases: []string{"features"},
		Short:   "Delete a feature",
		Long: `Delete a feature with its flags and its targeting in every release stage.
SDKs that still evaluate its flags get the defaults in their code. It asks for
confirmation unless -y is given; without a terminal to ask on, it needs -y.

Examples:
  dtctl delete fm feature new-checkout
  dtctl delete fm feature new-checkout -y
`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmdContext(cmd)
			ok, err := confirmFMDelete(ctx, yes, fmt.Sprintf("feature %s with its flags and targeting", args[0]))
			if err != nil || !ok {
				return err
			}
			_, project, h, printer, err := setupFMWrite(cmd, safety.OperationDelete)
			if err != nil {
				return err
			}
			key, err := h.DeleteFeature(project, args[0])
			if err != nil {
				return err
			}
			return printFMDeleted(ctx, printer, "fm/feature", fmt.Sprintf("Deleted feature %s from project %s", key, project), map[string]string{"projectKey": project, "key": key})
		},
	}
	addFMYesFlag(c, &yes)
	return c
}
