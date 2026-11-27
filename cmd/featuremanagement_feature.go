package cmd

import (
	"cmp"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Dynatrace/dtctl/pkg/output"
	"github.com/Dynatrace/dtctl/pkg/resources/featuremanagement"
)

func newGetFMFeaturesCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "features [feature-key]",
		Aliases: []string{"feature"},
		Short:   "Get features of a Feature Management project",
		Long: `Get the features of a Feature Management project.

A feature is a releasable unit of work: its variants, its flags, and its
lifecycle status. A flag is a typed value with one value per variant; it is
what SDKs evaluate. A feature is identified by its key; a name is accepted too
when no feature has it as a key.

The project comes from --project, or from the context's default project
(dtctl config set-context <context> --fm-project <project>).

Examples:
  # List the features of a project
  dtctl get fm features --project checkout

  # Get one feature
  dtctl get fm feature new-checkout

  # Wide output (adds description, owner, last modified, version)
  dtctl get fm features -o wide

  # Variants and flags in full
  dtctl get fm feature new-checkout -o yaml
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
				f, err := h.GetFeature(project, args[0])
				if err != nil {
					return err
				}
				return printer.Print(f)
			}

			features, err := h.ListFeatures(project)
			if err != nil {
				return err
			}
			return printer.PrintList(features)
		},
	}
}

func newDescribeFMFeatureCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "feature <feature-key>",
		Aliases: []string{"features"},
		Short:   "Show details of a Feature Management feature",
		Long: `Show detailed information about a feature of a Feature Management project:
its variants, each flag's value per variant, and whether targeting is on in
each release stage. For a stage's rules, use
'dtctl describe fm feature-targeting <feature> --stage <stage>'.

The argument is the feature's key; a name is accepted too when no feature has
it as a key. The project comes from --project, or from the context's default
project.

Examples:
  # Describe a feature
  dtctl describe fm feature new-checkout --project checkout

  # Output as JSON or YAML
  dtctl describe fm feature new-checkout -o json
  dtctl describe fm feature new-checkout -o yaml
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

			h := featuremanagement.NewHandler(c)
			f, err := h.GetFeature(project, args[0])
			if err != nil {
				return err
			}

			if ctx := cmdContext(cmd); useActionDescribeTextView(ctx) {
				out := currentStdout(ctx)
				users := newFMUserNames(c)
				users.modificationInfo(&f.ModificationInfo)
				f.Owner, f.Maintainers = users.name(f.Owner), users.names(f.Maintainers)
				printFMFeatureDescribe(out, f)
				// The per-stage summary needs the targeting read scope, which
				// the feature itself does not; without it the feature still
				// describes, and stderr says what is missing.
				if stages, err := h.ListFeatureTargeting(project, f.Key); err != nil {
					output.FprintWarning(currentStderr(ctx), "release stage targeting not shown: %v", err)
				} else {
					printFMFeatureStages(out, f.Key, stages, newFMVariantColors(f.Variants))
				}
				return nil
			}

			enrichAgent(printer, "describe", "fm/feature")
			return printer.Print(f)
		},
	}
}

// printFMFeatureDescribe renders the vertical describe view of a feature:
// its fields, then its variants, then each flag's value per variant.
func printFMFeatureDescribe(w io.Writer, f *featuremanagement.Feature) {
	const kw = 15
	output.FprintDescribeKV(w, "Key:", kw, "%s", f.Key)
	output.FprintDescribeKV(w, "Name:", kw, "%s", f.Name)
	output.FprintDescribeKV(w, "Project:", kw, "%s", f.ProjectKey)
	if f.Description != "" {
		output.FprintDescribeKV(w, "Description:", kw, "%s", f.Description)
	}
	output.FprintDescribeKV(w, "Type:", kw, "%s", f.Type)
	output.FprintDescribeKV(w, "Status:", kw, "%s", f.Status)
	if f.Owner != "" {
		output.FprintDescribeKV(w, "Owner:", kw, "%s", f.Owner)
	}
	if len(f.Maintainers) > 0 {
		output.FprintDescribeKV(w, "Maintainers:", kw, "%s", strings.Join(f.Maintainers, ", "))
	}
	output.FprintDescribeKV(w, "Version:", kw, "%d", f.Version)
	printFMModificationInfo(w, kw, f.ModificationInfo)
	if f.ResourceContext != nil && len(f.ResourceContext.Operations) > 0 {
		output.FprintDescribeKV(w, "Operations:", kw, "%s", strings.Join(f.ResourceContext.Operations, ", "))
	}

	variantWidth := 0
	for _, v := range f.Variants {
		variantWidth = max(variantWidth, len(v.Key))
	}
	colors := newFMVariantColors(f.Variants)

	fmt.Fprintln(w)
	output.FprintDescribeSection(w, "Variants:")
	if len(f.Variants) == 0 {
		fmt.Fprintln(w, "  (none)")
	}
	for _, v := range f.Variants {
		// A variant created from flags is named after its key; print the name
		// only when it adds something, as for flags below.
		if v.Name == "" || v.Name == v.Key {
			fmt.Fprintf(w, "  %s\n", colors.paint(v.Key, 0))
			continue
		}
		fmt.Fprintf(w, "  %s  %s\n", colors.paint(v.Key, variantWidth), v.Name)
	}

	fmt.Fprintln(w)
	output.FprintDescribeSection(w, "Flags:")
	if len(f.Flags) == 0 {
		fmt.Fprintln(w, "  (none)")
	}
	for i, fl := range f.Flags {
		if i > 0 {
			fmt.Fprintln(w)
		}
		fmt.Fprintf(w, "  %s (%s)", fl.Key, fl.Type)
		if fl.Name != "" && fl.Name != fl.Key {
			fmt.Fprintf(w, "  %s", fl.Name)
		}
		fmt.Fprintln(w)
		if fl.Description != "" {
			fmt.Fprintf(w, "    %s\n", fl.Description)
		}
		if t, ok := fl.Schema["type"].(string); ok {
			fmt.Fprintf(w, "    Schema: %s\n", t)
		}
		for _, key := range flagVariantOrder(f.Variants, fl) {
			fmt.Fprintf(w, "    %s%s  %s\n", colors.paint(key, 0), ":"+strings.Repeat(" ", max(0, variantWidth-len(key))), flagValue(fl.Variants[key]))
		}
	}
}

// fmVariantColor is one entry of the variant palette: a Dynatrace chart color
// for terminals that render 24-bit color, and the nearest of the 16 ANSI
// colors for the rest.
type fmVariantColor struct {
	r, g, b  uint8
	fallback string
}

func (c fmVariantColor) code() string {
	if output.TrueColorEnabled() {
		return output.RGB(c.r, c.g, c.b)
	}
	return c.fallback
}

// fmVariantPalette colors variants by display order with the Strato
// categorical chart colors Dynatrace apps use for series (dark theme), minus
// the near-white and grey entries that read as plain text. Green comes last,
// so a feature's first variants never read as success or failure.
var fmVariantPalette = []fmVariantColor{
	{0x2e, 0x5b, 0xd6, output.BrightBlue},    // categorical 01
	{0xd3, 0x50, 0x91, output.BrightMagenta}, // categorical 04
	{0xb6, 0x8c, 0x2b, output.BrightYellow},  // categorical 06
	{0x8d, 0x74, 0xcd, output.Magenta},       // categorical 08
	{0x42, 0x85, 0xa9, output.BrightCyan},    // categorical 07
	{0x36, 0x81, 0x60, output.Green},         // categorical 03
}

// fmVariantColors gives each of a feature's variants its own color, so one
// variant reads the same in the variant list, a flag's values and what a
// stage serves.
type fmVariantColors map[string]fmVariantColor

func newFMVariantColors(variants []featuremanagement.Variant) fmVariantColors {
	ordered := slices.Clone(variants)
	slices.SortStableFunc(ordered, func(a, b featuremanagement.Variant) int {
		return cmp.Compare(fmDisplayOrder(a), fmDisplayOrder(b))
	})
	colors := make(fmVariantColors, len(ordered))
	for i, v := range ordered {
		colors[v.Key] = fmVariantPalette[i%len(fmVariantPalette)]
	}
	return colors
}

func fmDisplayOrder(v featuremanagement.Variant) int {
	if v.DisplayOrder == nil {
		return 0
	}
	return *v.DisplayOrder
}

// paint pads key to width and colors it. Padding comes first so the escape
// codes don't count toward the column; a key with no color stays plain.
func (c fmVariantColors) paint(key string, width int) string {
	s := fmt.Sprintf("%-*s", width, key)
	if color, ok := c[key]; ok {
		return output.Colorize(color.code(), s)
	}
	return s
}

// flagVariantOrder lists a flag's variant keys in the feature's variant
// order, then any the feature does not declare, sorted.
func flagVariantOrder(variants []featuremanagement.Variant, fl featuremanagement.FeatureFlag) []string {
	seen := make(map[string]bool, len(variants))
	var keys []string
	for _, v := range variants {
		if _, ok := fl.Variants[v.Key]; ok {
			keys = append(keys, v.Key)
			seen[v.Key] = true
		}
	}
	var rest []string
	for k := range fl.Variants {
		if !seen[k] {
			rest = append(rest, k)
		}
	}
	sort.Strings(rest)
	return append(keys, rest...)
}

// flagValue renders a flag value as JSON, so a string reads quoted and a
// number or boolean does not, which keeps "true" and true apart.
func flagValue(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(b)
}
