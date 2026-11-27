package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Dynatrace/dtctl/pkg/output"
	"github.com/Dynatrace/dtctl/pkg/resources/featuremanagement"
	"github.com/Dynatrace/dtctl/pkg/safety"
)

// fmApplyResult is what a typed FM apply prints outside the table view.
type fmApplyResult struct {
	Action    string                              `json:"action" yaml:"action"`
	DryRun    bool                                `json:"dryRun,omitempty" yaml:"dryRun,omitempty"`
	Key       string                              `json:"key" yaml:"key"`
	Diff      string                              `json:"diff,omitempty" yaml:"diff,omitempty"`
	Warnings  []string                            `json:"warnings,omitempty" yaml:"warnings,omitempty"`
	Feature   *featuremanagement.Feature          `json:"feature,omitempty" yaml:"feature,omitempty"`
	Targeting *featuremanagement.FeatureTargeting `json:"targeting,omitempty" yaml:"targeting,omitempty"`
}

var applyFMCmd = newApplyFMCmd()

func newApplyFMCmd() *cobra.Command {
	c := newFMGroupCmd("Apply Feature Management files")
	c.AddCommand(newApplyFMFeatureCmd())
	c.AddCommand(newApplyFMFeatureTargetingCmd())
	return c
}

func init() {
	addDevelopmentCommand(applyCmd, applyFMCmd, featureManagementDevelopmentFeature)
}

// runFMApply reads the file, sets up the handler and runs apply. A dry run
// is set up like a read: it sends no write, so it needs no safety level.
func runFMApply(cmd *cobra.Command, file string, dryRun, showDiff bool, verb, resource string,
	apply func(project string, h *featuremanagement.Handler, data []byte) (*featuremanagement.ApplyResult, error)) error {
	ctx := cmdContext(cmd)
	data, err := readFileFlag(ctx, "file", file)
	if err != nil {
		return err
	}
	var (
		project string
		h       *featuremanagement.Handler
		printer output.Printer
	)
	if dryRun {
		cfg, c, p, err := setup(ctx)
		if err != nil {
			return err
		}
		if project, err = resolveFMProject(cmd, cfg); err != nil {
			return err
		}
		h, printer = featuremanagement.NewHandler(c), p
	} else {
		if _, project, h, printer, err = setupFMWrite(cmd, safety.OperationUpdate); err != nil {
			return err
		}
	}
	res, err := apply(project, h, data)
	if err != nil {
		return fmt.Errorf("%s: %w", sourceName(file), err)
	}

	stderr := currentStderr(ctx)
	if res.Diff != "" && (showDiff || dryRun) && useFMTableView(ctx) {
		_, _ = fmt.Fprintln(stderr, strings.TrimRight(res.Diff, "\n"))
	}
	action := res.Action
	if dryRun && action != featuremanagement.ApplyUnchanged {
		action = "would be " + action
	}
	msg := fmt.Sprintf("%s %s (%s)", strings.TrimPrefix(resource, "fm/"), res.Key, action)
	if !useFMTableView(ctx) {
		out := fmApplyResult{Action: res.Action, DryRun: dryRun, Key: res.Key, Warnings: res.Warnings,
			Feature: res.Feature, Targeting: res.Targeting}
		if showDiff || dryRun {
			out.Diff = res.Diff
		}
		return printFMWrite(ctx, printer, verb, resource, msg, res.Action != featuremanagement.ApplyUnchanged, nil, out)
	}
	if res.Targeting != nil {
		if res.Action != featuremanagement.ApplyUnchanged {
			output.FprintSuccess(stderr, "%s", msg)
		} else {
			output.FprintInfo(stderr, "%s", msg)
		}
		for _, w := range res.Warnings {
			output.FprintWarning(stderr, "%s", w)
		}
		return printFMTargetingTable(currentStdout(ctx), outputFormat(ctx), printer, []featuremanagement.FeatureTargeting{*res.Targeting}, true)
	}
	return printFMWrite(ctx, printer, verb, resource, msg, res.Action != featuremanagement.ApplyUnchanged, res.Warnings, res.Feature)
}

func newApplyFMFeatureCmd() *cobra.Command {
	var (
		file             string
		dryRun, showDiff bool
	)
	c := &cobra.Command{
		Use:     "feature -f <file>",
		Aliases: []string{"features"},
		Short:   "Create or replace a feature from a file",
		Long: `Create or replace a feature from a file, as one write: the way to make several
changes together, or ones the update flags can't express.

The file is the API's read model, as 'dtctl get fm feature <key> -o yaml'
exports it. A feature with the file's key is replaced with everything in the
file, sent with the version the file carries, so if the feature changed
since the export the write fails with a conflict and nothing is applied.
Without a version the live one is used. A feature that doesn't exist is
created.

Before sending, every flag needs a value for every variant, and variant and
flag keys must be unique. --show-diff prints the change on stderr; --dry-run
prints it and sends nothing.

Examples:
  dtctl get fm feature new-checkout -o yaml > feature.yaml
  # edit feature.yaml, then
  dtctl apply fm feature -f feature.yaml --show-diff
  dtctl apply fm feature -f feature.yaml --dry-run
`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runFMApply(cmd, file, dryRun, showDiff, "apply", "fm/feature",
				func(project string, h *featuremanagement.Handler, data []byte) (*featuremanagement.ApplyResult, error) {
					return h.ApplyFeature(project, data, featuremanagement.ApplyOptions{DryRun: dryRun})
				})
		},
	}
	c.Flags().StringVarP(&file, "file", "f", "", "file containing the feature (YAML or JSON)")
	c.Flags().BoolVar(&dryRun, "dry-run", false, "check the file and print the change without applying it")
	c.Flags().BoolVar(&showDiff, "show-diff", false, "print the change on stderr")
	_ = c.MarkFlagRequired("file")
	markFileFlag(c, "file", fileValuePath)
	return c
}

func newApplyFMFeatureTargetingCmd() *cobra.Command {
	var (
		file, stage             string
		dryRun, showDiff, force bool
	)
	c := &cobra.Command{
		Use:     "feature-targeting <feature-key-or-name> --stage <stage> -f <file>",
		Aliases: []string{"feature-targetings", "targeting"},
		Short:   "Replace a feature's targeting in one release stage from a file",
		Long: `Replace a feature's targeting in one release stage from a file, as one write:
the way to change several rules together, or anything the rule flags don't
cover, such as reordering.

The file is the API's read model, as
'dtctl get fm feature-targeting <feature> -s <stage> -o yaml' exports it; an
export of every stage works too, and the entry for --stage is used. It is
sent with the version the file carries, so if the stage changed since the
export the write fails with a conflict and nothing is applied. Without a
version the live one is used.

Before sending, dtctl runs the targeting commands' checks on every rule:
served variants exist and total 100%, conditions name context attributes
with their value type, and rollout steps are in time order. New rules have
no id. A file that drops an existing rule's id, by removing the rule or
keeping it without its id, re-buckets that rule's users, so it is refused
unless --force is given. --show-diff prints the change on stderr; --dry-run
prints it and sends nothing.

Examples:
  dtctl get fm feature-targeting new-checkout -s staging -o yaml > staging.yaml
  # edit staging.yaml, then
  dtctl apply fm feature-targeting new-checkout -s staging -f staging.yaml --show-diff
`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := requireFMStage(cmd, stage)
			if err != nil {
				return err
			}
			return runFMApply(cmd, file, dryRun, showDiff, "apply", "fm/feature-targeting",
				func(project string, h *featuremanagement.Handler, data []byte) (*featuremanagement.ApplyResult, error) {
					return h.ApplyFeatureTargeting(project, args[0], s, data, featuremanagement.ApplyOptions{DryRun: dryRun, Force: force})
				})
		},
	}
	c.Flags().StringVarP(&stage, fmStageFlag, "s", "", fmTargetingStageHelp)
	c.Flags().StringVarP(&file, "file", "f", "", "file containing the stage's targeting (YAML or JSON)")
	c.Flags().BoolVar(&dryRun, "dry-run", false, "check the file and print the change without applying it")
	c.Flags().BoolVar(&showDiff, "show-diff", false, "print the change on stderr")
	c.Flags().BoolVar(&force, "force", false, "apply a file that drops an existing rule's id, re-bucketing its users")
	_ = c.MarkFlagRequired("file")
	markFileFlag(c, "file", fileValuePath)
	return c
}
