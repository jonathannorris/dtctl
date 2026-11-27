package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Dynatrace/dtctl/pkg/output"
	"github.com/Dynatrace/dtctl/pkg/resources/featuremanagement"
	"github.com/Dynatrace/dtctl/pkg/safety"
)

func newCreateFMSDKKeyCmd() *cobra.Command {
	var keyType string
	c := &cobra.Command{
		Use:     "sdk-key <release-stage-key>",
		Aliases: []string{"sdk-keys"},
		Short:   "Create an SDK key for a release stage",
		Long: `Create an SDK key: the credential an SDK of the given type uses to fetch one
release stage's flag configuration. The new key is printed in full, in every
output format, since it is created to be used; treat it as a live
credential.

A stage can hold several keys of a type, so an application can move to a new
key before the old one is revoked. To reuse an existing key instead, see
'dtctl get fm sdk-keys <stage> --reveal'.

Examples:
  dtctl create fm sdk-key production --type server
  dtctl create fm sdk-key staging --type web -o json
`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmdContext(cmd)
			if keyType == "" {
				return fmt.Errorf("--type is required: server, web or mobile")
			}
			if !fmValidSDKKeyType(keyType) {
				return fmt.Errorf("invalid --type %q: must be one of server, web, mobile", keyType)
			}
			_, project, h, printer, err := setupFMWrite(cmd, safety.OperationUpdate)
			if err != nil {
				return err
			}
			res, err := h.CreateSDKKey(project, args[0], featuremanagement.SDKKeyType(keyType))
			if err != nil {
				return err
			}
			var warnings []string
			if res.Existing > 0 {
				warnings = append(warnings, fmt.Sprintf("release stage %s already had %d %s key(s); 'dtctl get fm sdk-keys %s --type %s --reveal' shows them for reuse",
					res.Key.ReleaseStageKey, res.Existing, keyType, res.Key.ReleaseStageKey, keyType))
			}
			if useFMTableView(ctx) {
				output.FprintWarning(currentStderr(ctx), "%s", fmSDKKeyNotice)
			}
			return printFMWrite(ctx, printer, "create", "fm/sdk-key",
				fmt.Sprintf("Created %s SDK key in release stage %s of project %s", keyType, res.Key.ReleaseStageKey, project), true, warnings, res.Key)
		},
	}
	c.Flags().StringVar(&keyType, "type", "", "the SDK the key is for: server|web|mobile (required)")
	_ = c.RegisterFlagCompletionFunc("type", cobra.FixedCompletions(fmSDKKeyTypes, cobra.ShellCompDirectiveNoFileComp))
	return c
}

func newDeleteFMSDKKeyCmd() *cobra.Command {
	var yes bool
	c := &cobra.Command{
		Use:     "sdk-key <release-stage-key> <sdk-key>",
		Aliases: []string{"sdk-keys"},
		Short:   "Revoke an SDK key",
		Long: `Revoke an SDK key of a release stage. Applications using it stop receiving
flags: their requests fail with 404 from then on.

The key can be given masked, as 'dtctl get fm sdk-keys' prints it
(dt01.server_us_AbCd123456.********), as its public part alone, or in full.

Because applications using the key break, it needs a context at the
dangerously-unrestricted safety level. It asks for confirmation unless -y is
given; without a terminal to ask on, it needs -y.

Examples:
  dtctl delete fm sdk-key production dt01.server_us_AbCd123456.********
  dtctl delete fm sdk-key production dt01.server_us_AbCd123456 -y
`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmdContext(cmd)
			ok, err := confirmFMDelete(ctx, yes, fmt.Sprintf("SDK key %s of release stage %s; applications using it stop receiving flags", featuremanagement.MaskSDKKeyArg(args[1]), args[0]))
			if err != nil || !ok {
				return err
			}
			_, project, h, printer, err := setupFMWrite(cmd, safety.OperationDeleteBucket)
			if err != nil {
				return err
			}
			masked, err := h.DeleteSDKKey(project, args[0], args[1])
			if err != nil {
				return err
			}
			return printFMDeleted(ctx, printer, "fm/sdk-key", fmt.Sprintf("Revoked SDK key %s of release stage %s", masked, args[0]),
				map[string]string{"projectKey": project, "releaseStageKey": args[0], "sdkKey": masked})
		},
	}
	addFMYesFlag(c, &yes)
	return c
}
