package cmd

import (
	"context"
	"fmt"
	"reflect"

	"github.com/spf13/cobra"

	"github.com/Dynatrace/dtctl/pkg/output"
	"github.com/Dynatrace/dtctl/pkg/prompt"
)

// fmNoChangeSuggestion tells an agent that a write found nothing to do.
const fmNoChangeSuggestion = "nothing changed: the resource already was what the command asked for"

// printFMWrite reports a write of one resource: a line on stderr in table
// view, then the resource through the printer. When nothing changed the line
// says so and the agent envelope carries a suggestion that does.
func printFMWrite(ctx context.Context, printer output.Printer, verb, resource, message string, changed bool, warnings []string, v any) error {
	stderr := currentStderr(ctx)
	if useFMTableView(ctx) {
		if changed {
			output.FprintSuccess(stderr, "%s", message)
		} else {
			output.FprintInfo(stderr, "%s", message)
		}
	}
	for _, w := range warnings {
		output.FprintWarning(stderr, "%s", w)
	}
	if ap := enrichAgent(printer, verb, resource); ap != nil && !changed {
		ap.Context().Suggestions = append(ap.Context().Suggestions, fmNoChangeSuggestion)
	}
	if rv := reflect.ValueOf(v); rv.Kind() == reflect.Slice {
		return printer.PrintList(v)
	}
	return printer.Print(v)
}

// confirmFMDelete asks before an FM delete unless -y was given. Without a
// terminal to ask on (plain or agent mode) it refuses instead of deleting
// unasked, so a script states its intent with -y.
func confirmFMDelete(ctx context.Context, yes bool, what string) (bool, error) {
	if yes {
		return true, nil
	}
	if plainMode(ctx) || agentMode(ctx) {
		return false, fmt.Errorf("deleting %s cannot be undone; pass -y to confirm", what)
	}
	if !prompt.ConfirmWith(currentStdin(ctx), currentStdout(ctx), fmt.Sprintf("Delete %s? This cannot be undone.", what)) {
		output.FprintInfo(currentStderr(ctx), "Deletion cancelled")
		return false, nil
	}
	return true, nil
}

// addFMYesFlag declares -y on an FM delete and the prompt it skips.
func addFMYesFlag(c *cobra.Command, yes *bool) {
	c.Flags().BoolVarP(yes, "yes", "y", false, "skip the confirmation prompt")
	markPrompts(c, "yes")
}

// changedAny reports whether any of the named flags was given.
func changedAny(cmd *cobra.Command, names ...string) bool {
	for _, n := range names {
		if cmd.Flags().Changed(n) {
			return true
		}
	}
	return false
}

// printFMDeleted reports a delete: a line on stderr in table view, and what
// was deleted under -o json|yaml and in the agent envelope.
func printFMDeleted(ctx context.Context, printer output.Printer, resource, message string, deleted map[string]string) error {
	if useFMTableView(ctx) {
		output.FprintSuccess(currentStderr(ctx), "%s", message)
		return nil
	}
	enrichAgent(printer, "delete", resource)
	return printer.Print(map[string]any{"deleted": deleted})
}
