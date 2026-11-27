package cmd

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Dynatrace/dtctl/pkg/output"
	"github.com/Dynatrace/dtctl/pkg/resources/featuremanagement"
)

// fmStageFlag names a release stage on the targeting commands. It is never
// filled from the context: the stage always appears in the command text.
const fmStageFlag = "stage"

// useFMTableView reports whether a Feature Management command prints its
// tabular view (table, wide or csv) rather than the API's read model, for a
// resource whose read model nests too deep for a table. It follows the
// printer: --plain prints table and wide as JSON, so they get the read model.
func useFMTableView(ctx context.Context) bool {
	if agentMode(ctx) {
		return false
	}
	f := outputFormat(ctx)
	return f == "csv" || output.IsTabularFormat(f, plainMode(ctx)) && f != "toon"
}

func newGetFMFeatureTargetingCmd() *cobra.Command {
	var stage string
	c := &cobra.Command{
		Use:     "feature-targeting <feature-key>",
		Aliases: []string{"feature-targetings", "feature-targeting-rule", "feature-targeting-rules"},
		Short:   "Get a feature's targeting rules per release stage",
		Long: `Get a feature's targeting in every release stage, or in one with --stage:
whether targeting is on, and the ordered rules that pick a variant.

Table output lists each stage (on/off and its rule count), then each
stage's rules. Rules are evaluated top-down and the first match wins; a rule
with no conditions matches all requests. The # column is the rule's 0-based
position and ID the prefix of its id; the write commands take either with
--position. Conditions are shown one per line in --condition syntax, with
context keys.

-o json or -o yaml prints the API's targeting unchanged, full rule ids
included.

The feature is a key; a name is accepted too when no feature has it as a key.
The project comes from --project, or from the context's default project.

Examples:
  # Every stage: on/off and its rules
  dtctl get fm feature-targeting new-checkout

  # One stage
  dtctl get fm feature-targeting new-checkout --stage production

  # The API's targeting, unchanged
  dtctl get fm feature-targeting new-checkout -s production -o json
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

			var items []featuremanagement.FeatureTargeting
			if cmd.Flags().Changed(fmStageFlag) {
				if stage == "" {
					return fmt.Errorf("--%s must not be empty", fmStageFlag)
				}
				ft, err := h.GetFeatureTargeting(project, args[0], stage)
				if err != nil {
					return err
				}
				if !useFMTableView(ctx) {
					return printer.Print(ft)
				}
				items = []featuremanagement.FeatureTargeting{*ft}
			} else {
				items, err = h.ListFeatureTargeting(project, args[0])
				if err != nil {
					return err
				}
				if !useFMTableView(ctx) {
					return printer.PrintList(items)
				}
			}
			return printFMTargetingTable(currentStdout(ctx), outputFormat(ctx), printer, items, cmd.Flags().Changed(fmStageFlag))
		},
	}
	c.Flags().StringVarP(&stage, fmStageFlag, "s", "", "release stage key (default: every stage)")
	return c
}

// printFMTargetingTable prints targeting in its tabular view. CSV gets one
// flat row per rule. Table and wide print a stage summary and then each
// stage's rules, with one condition per line; for a single stage the rules
// follow the summary without a stage heading.
func printFMTargetingTable(w io.Writer, format string, printer output.Printer, items []featuremanagement.FeatureTargeting, oneStage bool) error {
	if format == "csv" {
		return printer.PrintList(featuremanagement.TargetingRows(items))
	}
	if err := printer.PrintList(featuremanagement.TargetingStageRows(items)); err != nil {
		return err
	}
	for _, ft := range items {
		rules := featuremanagement.TargetingRuleRows(ft)
		if len(rules) == 0 {
			continue
		}
		fmt.Fprintln(w)
		if !oneStage {
			fmt.Fprintln(w, output.Colorize(output.Bold, ft.ReleaseStageKey))
		}
		if err := printer.PrintList(rules); err != nil {
			return err
		}
	}
	return nil
}

func newDescribeFMFeatureTargetingCmd() *cobra.Command {
	var stage string
	c := &cobra.Command{
		Use:     "feature-targeting <feature-key> --stage <stage>",
		Aliases: []string{"feature-targetings", "feature-targeting-rule", "feature-targeting-rules"},
		Short:   "Show a feature's targeting in one release stage",
		Long: `Show a feature's targeting in one release stage the way the UI does:
numbered rules (0-based, as the write commands take them), "all requests" for
a rule with no conditions, the variant served, the rollout schedule and the
bucketing key. Conditions are shown one per line in --condition syntax.

A rule placed below one that matches all requests is never evaluated; such
rules get a warning on stderr.

The feature is a key; a name is accepted too when no feature has it as a key.
The stage is a key and is required. The project comes from --project, or
from the context's default project.

Examples:
  # Describe the production targeting of a feature
  dtctl describe fm feature-targeting new-checkout --stage production

  # Output as JSON or YAML (the API's targeting, unchanged)
  dtctl describe fm feature-targeting new-checkout -s production -o yaml
`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmdContext(cmd)
			if stage == "" {
				return fmt.Errorf("--%s is required", fmStageFlag)
			}
			cfg, c, printer, err := setup(ctx)
			if err != nil {
				return err
			}
			project, err := resolveFMProject(cmd, cfg)
			if err != nil {
				return err
			}

			ft, err := featuremanagement.NewHandler(c).GetFeatureTargeting(project, args[0], stage)
			if err != nil {
				return err
			}

			if useActionDescribeTextView(ctx) {
				newFMUserNames(c).modificationInfo(&ft.ModificationInfo)
				// The feature's variant order picks the colors, as in describe
				// fm feature; without it (no read access) variants print plain.
				var colors fmVariantColors
				if f, err := featuremanagement.NewHandler(c).GetFeature(project, args[0]); err == nil {
					colors = newFMVariantColors(f.Variants)
				}
				printFMTargetingDescribe(currentStdout(ctx), ft, colors)
				for _, w := range featuremanagement.ShadowedRules(ft.Targeting.Rules) {
					output.FprintWarning(currentStderr(ctx), "%s", w)
				}
				return nil
			}

			enrichAgent(printer, "describe", "fm/feature-targeting")
			return printer.Print(ft)
		},
	}
	c.Flags().StringVarP(&stage, fmStageFlag, "s", "", "release stage key (required)")
	_ = c.MarkFlagRequired(fmStageFlag)
	return c
}

// printFMTargetingDescribe renders one stage's targeting: the stage, then
// each rule as a numbered block, then the default note.
func printFMTargetingDescribe(w io.Writer, ft *featuremanagement.FeatureTargeting, colors fmVariantColors) {
	const kw = 15
	output.FprintDescribeKV(w, "Feature:", kw, "%s", ft.FeatureKey)
	output.FprintDescribeKV(w, "Stage:", kw, "%s", ft.ReleaseStageKey)
	output.FprintDescribeKV(w, "Targeting:", kw, "%s", onOffColored(ft.Enabled, 0))
	output.FprintDescribeKV(w, "Version:", kw, "%d", ft.Version)
	printFMModificationInfo(w, kw, ft.ModificationInfo)

	rules := ft.Targeting.Rules
	if len(rules) == 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "No rules.")
	}
	posWidth := len(fmt.Sprint(len(rules) - 1))
	indent := strings.Repeat(" ", posWidth+2)
	const lw = 13
	line := func(label, value string) {
		fmt.Fprintf(w, "%s%s%s\n", indent, output.Colorize(output.Bold, label), strings.Repeat(" ", max(1, lw-len(label)))+value)
	}
	cont := func(value string) {
		fmt.Fprintf(w, "%s%s%s\n", indent, strings.Repeat(" ", lw), value)
	}
	for i, r := range rules {
		fmt.Fprintln(w)
		head := fmt.Sprintf("%-*d  %s", posWidth, i, featuremanagement.ShortRuleID(r.ID))
		if r.Name != "" {
			head += "  " + r.Name
		}
		fmt.Fprintln(w, head)
		for j, c := range featuremanagement.FormatConditions(r.Conditions) {
			if j == 0 {
				line("Conditions:", c)
			} else {
				cont(c)
			}
		}
		line("Variant:", fmFormatServe(r.Serve, colors))
		if r.Rollout != nil && len(r.Rollout.Steps) > 0 {
			for j, s := range r.Rollout.Steps {
				if j == 0 {
					line("Rollout:", featuremanagement.FormatRolloutStep(s))
				} else {
					cont(featuremanagement.FormatRolloutStep(s))
				}
			}
		}
		if r.BucketingKey != "" {
			line("Bucketing:", r.BucketingKey)
		}
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Default: requests that match no rule get the default defined in code.")
}

func onOffString(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

// onOffColored is onOffString padded to width and colored like the TARGETING
// column of the tables: green on, red off. Padding comes first, so the escape
// codes don't count toward the column.
func onOffColored(b bool, width int) string {
	s := fmt.Sprintf("%-*s", width, onOffString(b))
	if b {
		return output.Colorize(output.BrightGreen, s)
	}
	return output.Colorize(output.BrightRed, s)
}

// printFMFeatureStages renders the per-stage summary at the end of a
// feature's describe view: whether targeting is on, how many rules each stage
// has, and which variants it serves, then the command that shows a stage's
// rules in full.
func printFMFeatureStages(w io.Writer, featureKey string, items []featuremanagement.FeatureTargeting, colors fmVariantColors) {
	fmt.Fprintln(w)
	output.FprintDescribeSection(w, "Release Stages:")
	if len(items) == 0 {
		fmt.Fprintln(w, "  (none)")
		return
	}
	width := 0
	for _, ft := range items {
		width = max(width, len(ft.ReleaseStageKey))
	}
	for _, ft := range items {
		n := len(ft.Targeting.Rules)
		rules := fmt.Sprintf("%d rules", n)
		if n == 1 {
			rules = "1 rule"
		}
		fmt.Fprintf(w, "  %-*s  %s  %-7s  serves %s\n", width, ft.ReleaseStageKey, onOffColored(ft.Enabled, 3), rules, fmStageServes(ft, colors))
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Rules: dtctl describe fm feature-targeting %s --stage <stage>\n", featureKey)
}

// fmFormatServe is featuremanagement.FormatServe with each variant key in its
// color: the one variant a rule serves, or a split as key=percent pairs.
func fmFormatServe(serve []featuremanagement.VariantDistribution, colors fmVariantColors) string {
	if len(serve) == 0 {
		return "-"
	}
	if len(serve) == 1 && serve[0].Percentage == 100 {
		return colors.paint(serve[0].VariantKey, 0)
	}
	parts := make([]string, len(serve))
	for i, d := range serve {
		plain := featuremanagement.FormatServe([]featuremanagement.VariantDistribution{d})
		parts[i] = colors.paint(d.VariantKey, 0) + strings.TrimPrefix(plain, d.VariantKey)
	}
	return strings.Join(parts, ",")
}

// fmStageServes says what a stage hands out: the code default while targeting
// is off or has no rules, else the variants its rules serve in rule order,
// then the code default unless a rule matches every request. A rule with a
// rollout does not, since requests outside its percentage fall through.
func fmStageServes(ft featuremanagement.FeatureTargeting, colors fmVariantColors) string {
	const codeDefault = "the code default"
	if !ft.Enabled || len(ft.Targeting.Rules) == 0 {
		return codeDefault
	}
	var parts []string
	seen := map[string]bool{}
	catchAll := false
	for _, r := range ft.Targeting.Rules {
		if p := fmFormatServe(r.Serve, colors); len(r.Serve) > 0 && !seen[p] {
			seen[p] = true
			parts = append(parts, p)
		}
		if len(r.Conditions) == 0 && r.Rollout == nil {
			catchAll = true
			break
		}
	}
	if !catchAll {
		parts = append(parts, codeDefault)
	}
	return strings.Join(parts, ", ")
}
