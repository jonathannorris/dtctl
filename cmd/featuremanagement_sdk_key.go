package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Dynatrace/dtctl/pkg/output"
	"github.com/Dynatrace/dtctl/pkg/resources/featuremanagement"
)

// fmSDKKeyTypes are the values --type accepts on the SDK key commands.
var fmSDKKeyTypes = []string{"server", "web", "mobile"}

// fmSDKKeyNotice is printed on stderr with revealed key values in the table
// view. It goes to stderr so it cannot end up in a piped value.
const fmSDKKeyNotice = "SDK key values are live credentials: keep them out of logs, tickets and chat."

func newGetFMSDKKeysCmd() *cobra.Command {
	var (
		keyType string
		reveal  bool
	)
	c := &cobra.Command{
		Use:     "sdk-keys <release-stage-key>",
		Aliases: []string{"sdk-key"},
		Short:   "Get the SDK keys of a release stage",
		Long: `Get the SDK keys of a release stage of a Feature Management project.

An SDK key is the credential an SDK uses to fetch one release stage's flag
configuration. Key values are masked in every output format, the --agent
envelope included: a masked key keeps its public, log-safe part and hides the
secret, as in dt01.server_us_AbCd123456.********.

--reveal prints the full values, so an existing key can be reused instead of
creating a new one each time. Treat revealed values as live credentials.

The release stage is a key; a name is accepted too when no stage has it as a
key. The project comes from --project, or from the context's default project.

Examples:
  # List the SDK keys of the production stage, values masked
  dtctl get fm sdk-keys production --project checkout

  # The full value of the stage's server keys, to reuse one
  dtctl get fm sdk-keys production --type server --reveal

  # Wide output (adds who created each key)
  dtctl get fm sdk-keys production -o wide
`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmdContext(cmd)
			if keyType != "" && !fmValidSDKKeyType(keyType) {
				return fmt.Errorf("invalid --type %q: must be one of server, web, mobile", keyType)
			}
			cfg, c, printer, err := setup(ctx)
			if err != nil {
				return err
			}
			project, err := resolveFMProject(cmd, cfg)
			if err != nil {
				return err
			}
			keys, err := featuremanagement.NewHandler(c).ListSDKKeys(project, args[0], featuremanagement.SDKKeyListOptions{
				Type:   featuremanagement.SDKKeyType(keyType),
				Reveal: reveal,
			})
			if err != nil {
				return err
			}
			if reveal && len(keys) > 0 && useFMTableView(ctx) {
				output.FprintWarning(currentStderr(ctx), "%s", fmSDKKeyNotice)
			}
			return printer.PrintList(keys)
		},
	}
	c.Flags().StringVar(&keyType, "type", "", "only keys of this type: server|web|mobile")
	c.Flags().BoolVar(&reveal, "reveal", false, "print full key values instead of masking the secret")
	_ = c.RegisterFlagCompletionFunc("type", cobra.FixedCompletions(fmSDKKeyTypes, cobra.ShellCompDirectiveNoFileComp))
	return c
}

func fmValidSDKKeyType(t string) bool {
	for _, v := range fmSDKKeyTypes {
		if t == v {
			return true
		}
	}
	return false
}
