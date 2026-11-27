package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Dynatrace/dtctl/pkg/client"
	"github.com/Dynatrace/dtctl/pkg/config"
	"github.com/Dynatrace/dtctl/pkg/output"
	"github.com/Dynatrace/dtctl/pkg/prompt"
	"github.com/Dynatrace/dtctl/pkg/resources/featuremanagement"
	"github.com/Dynatrace/dtctl/pkg/safety"
)

// fmTargetingStageHelp is the --stage help on every targeting write. The
// stage is never filled from a context default, so it always appears in the
// command text approval policy matches.
const fmTargetingStageHelp = "release stage key (required; never taken from the context)"

const fmVersionHelp = "fail with 409 unless the current optimistic-locking version matches"

// fmConditionHelp is the condition syntax, shared by the rule commands' help.
const fmConditionHelp = `Conditions (--condition '<context-key> <operator> <values>'):
  Values are comma-separated. The value type comes from the project's context
  attribute with that context key; targetingKey is always STRING. '<key> = a,b'
  matches any of the values. All of a rule's conditions must match, and a rule
  with none matches all requests.

  STRING      =  !=  contains  !contains  starts-with  ends-with  empty  !empty
  NUMBER      =  !=  >  >=  <  <=
  SEM_VER     =  !=  >  >=  <  <=  ^ (same major)  ~ (same minor)
  BOOLEAN     =  !=
  DATE_TIME   before  after  between (two values)
  any type    exists  !exists

  A DATE_TIME value, like a --rollout time, is a timestamp with a zone
  (2026-12-01T09:00Z, 2026-12-01T10:00+01:00), Unix seconds, now, or an
  offset from now (+30m, +2h, +1d).`

// fmTargetingWrite is the result of a targeting write under -o json|yaml and
// in the --agent envelope: what changed, and the stage's targeting after it.
type fmTargetingWrite struct {
	Action          string                              `json:"action" yaml:"action"`
	Changed         bool                                `json:"changed" yaml:"changed"`
	FeatureKey      string                              `json:"featureKey" yaml:"featureKey"`
	ReleaseStageKey string                              `json:"releaseStageKey" yaml:"releaseStageKey"`
	Position        *int                                `json:"position,omitempty" yaml:"position,omitempty"`
	RuleID          string                              `json:"ruleId,omitempty" yaml:"ruleId,omitempty"`
	Removed         int                                 `json:"removed,omitempty" yaml:"removed,omitempty"`
	Targeting       *featuremanagement.FeatureTargeting `json:"targeting" yaml:"targeting"`
}

// setupFMWrite loads the config, applies the safety level to an FM write,
// and returns the project, the handler and a printer. FM configuration is
// shared, so every write but the caller's own overrides is checked with
// OwnershipShared (design decision 12): readwrite-mine refuses it.
func setupFMWrite(cmd *cobra.Command, op safety.Operation) (*config.Config, string, *featuremanagement.Handler, output.Printer, error) {
	ctx := cmdContext(cmd)
	cfg, err := loadConfig(ctx)
	if err != nil {
		return nil, "", nil, nil, err
	}
	if err := checkSafety(ctx, cfg, op, safety.OwnershipShared); err != nil {
		return nil, "", nil, nil, err
	}
	project, err := resolveFMProject(cmd, cfg)
	if err != nil {
		return nil, "", nil, nil, err
	}
	var c *client.Client
	if c, err = newClientFromConfig(ctx, cfg); err != nil {
		return nil, "", nil, nil, err
	}
	return cfg, project, featuremanagement.NewHandler(c), newPrinterCtx(ctx), nil
}

// requireFMStage returns --stage, which every targeting write requires.
func requireFMStage(cmd *cobra.Command, stage string) (string, error) {
	stage = strings.TrimSpace(stage)
	if !cmd.Flags().Changed(fmStageFlag) || stage == "" {
		return "", fmt.Errorf("--%s is required: the release stage to change (it is never taken from the context)", fmStageFlag)
	}
	return stage, nil
}

// printFMTargetingWrite reports a targeting write: a line on stderr and the
// stage's rules on stdout in table view, or the write's result otherwise.
// Warnings about shadowed rules go to stderr either way.
func printFMTargetingWrite(ctx context.Context, printer output.Printer, action, message string, res *featuremanagement.TargetingWriteResult) error {
	stderr := currentStderr(ctx)
	if useFMTableView(ctx) {
		if res.Changed {
			output.FprintSuccess(stderr, "%s", message)
		} else {
			output.FprintInfo(stderr, "%s", message)
		}
	}
	for _, w := range res.Warnings {
		output.FprintWarning(stderr, "%s", w)
	}
	ft := res.Targeting
	if useFMTableView(ctx) {
		return printFMTargetingTable(currentStdout(ctx), outputFormat(ctx), printer, []featuremanagement.FeatureTargeting{*ft}, true)
	}
	out := fmTargetingWrite{
		Action: action, Changed: res.Changed, FeatureKey: ft.FeatureKey, ReleaseStageKey: ft.ReleaseStageKey,
		RuleID: res.RuleID, Removed: res.Removed, Targeting: ft,
	}
	if res.Position >= 0 {
		p := res.Position
		out.Position = &p
	}
	if ap := enrichAgent(printer, action, "fm/feature-targeting"); ap != nil && !res.Changed {
		ap.Context().Suggestions = append(ap.Context().Suggestions, "nothing changed: the stage already was what the command asked for")
	}
	return printer.Print(out)
}

// ruleRef names a rule in messages: its position and short id.
func ruleRef(position int, id string) string {
	if id == "" {
		return fmt.Sprintf("rule %d", position)
	}
	return fmt.Sprintf("rule %d (%s)", position, featuremanagement.ShortRuleID(id))
}

// fmRuleFlags binds the rule-content flags create and update share.
type fmRuleFlags struct {
	conditions   []string
	variant      string
	rollout      []string
	bucketingKey string
	name         string
}

func (f *fmRuleFlags) spec(cmd *cobra.Command) featuremanagement.RuleSpec {
	s := featuremanagement.RuleSpec{Conditions: f.conditions, Rollout: f.rollout}
	if cmd.Flags().Changed("variant") {
		s.Variant = &f.variant
	}
	if cmd.Flags().Changed("bucketing-key") {
		s.BucketingKey = &f.bucketingKey
	}
	if cmd.Flags().Changed("name") {
		s.Name = &f.name
	}
	return s
}

func newCreateFMFeatureTargetingCmd() *cobra.Command {
	var (
		stage, position, version string
		rule                     fmRuleFlags
	)
	c := &cobra.Command{
		Use:     "feature-targeting <feature-key-or-name> --stage <stage> --variant <variant>",
		Aliases: []string{"feature-targeting-rule"},
		Short:   "Add a targeting rule to one release stage of a feature",
		Long: `Add a rule to one release stage's targeting of a feature.

This is a read-modify-write: dtctl reads the stage's targeting, adds the rule,
and sends every rule back with the version it read. Existing rules go back
unchanged, ids included, so their users stay in the same rollout buckets. If
the stage changes in between, the write is retried once on a fresh read.

The rule goes last unless --position says where: a 0-based position, or a
rule id to take that rule's place and move it down. Rules are evaluated
top-down and the first match wins.

` + fmConditionHelp + `

--variant serves one variant to every request the rule matches, or splits
them, like baseline=50,variation-a=50 (the split must total 100). --rollout
adds a percentage rollout, one --rollout per step, as <time>=<percent>, with
,linear to rise linearly from the previous step.

The feature is a key; a name is accepted too. The project comes from
--project, or from the context's default project.

Examples:
  # Target a list of users
  dtctl create fm feature-targeting new-checkout --stage production --variant variation-a \
    --condition 'targetingKey = user-1,user-2' --position 0

  # Roll out to accounts, 10% now and 100% in a week, bucketed by account
  dtctl create fm feature-targeting new-checkout --stage production --variant variation-a \
    --condition 'account.id = acct-1,acct-2' --bucketing-key account.id \
    --rollout now=10 --rollout +7d=100,linear

  # Typed conditions, all of which must match, and a 50/50 split
  dtctl create fm feature-targeting new-checkout --stage production --variant baseline=50,variation-a=50 \
    --condition 'plan.tier = enterprise,pro' --condition 'app.version >= 2.3.0'
`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := requireFMStage(cmd, stage)
			if err != nil {
				return err
			}
			if !cmd.Flags().Changed("variant") {
				return fmt.Errorf("--variant is required: the variant to serve, or a split like baseline=50,variation-a=50")
			}
			_, project, h, printer, err := setupFMWrite(cmd, safety.OperationUpdate)
			if err != nil {
				return err
			}
			spec := rule.spec(cmd)
			spec.Position = position
			res, err := h.CreateTargetingRule(project, args[0], s, spec, featuremanagement.TargetingWriteOptions{Version: version})
			if err != nil {
				return err
			}
			ft := res.Targeting
			msg := fmt.Sprintf("Created %s in stage %s of feature %s", ruleRef(res.Position, res.RuleID), ft.ReleaseStageKey, ft.FeatureKey)
			return printFMTargetingWrite(cmdContext(cmd), printer, "create", msg, res)
		},
	}
	c.Flags().SortFlags = false
	c.Flags().StringVarP(&stage, fmStageFlag, "s", "", fmTargetingStageHelp)
	c.Flags().StringArrayVar(&rule.conditions, "condition", nil, "condition as '<context-key> <operator> <values>' (repeatable)")
	c.Flags().StringVar(&rule.variant, "variant", "", "variant to serve, or a split like baseline=50,variation-a=50 (required)")
	c.Flags().StringArrayVar(&rule.rollout, "rollout", nil, "rollout step as <time>=<percent>[,linear] (repeatable; default: no rollout)")
	c.Flags().StringVar(&rule.bucketingKey, "bucketing-key", "", "context key that splits users into rollout buckets (default: targetingKey); doesn't change which requests match")
	c.Flags().StringVar(&position, "position", "", "where to insert: a 0-based position, or a rule id to take that rule's place (default: last)")
	c.Flags().StringVar(&rule.name, "name", "", "optional display name; never used to address the rule")
	c.Flags().StringVar(&version, "version", "", fmVersionHelp)
	return c
}

func newUpdateFMFeatureTargetingCmd() *cobra.Command {
	var (
		stage, position, movePosition, version   string
		conditionAdd, conditionRemove, condClear []string
		clearRollout                             bool
		rule                                     fmRuleFlags
	)
	c := &cobra.Command{
		Use:     "feature-targeting <feature-key-or-name> --position <position-or-id> --stage <stage>",
		Aliases: []string{"feature-targeting-rule"},
		Short:   "Change one targeting rule in one release stage of a feature",
		Long: `Change one rule in one release stage's targeting of a feature. The parts of
the rule you pass no flag for are kept.

--position selects the rule: a 0-based position, or its id (the full id or a
unique prefix, as 'dtctl get fm feature-targeting' shows). Scripts and agents
should pass the id, since a reorder can shift positions between commands.

This is a read-modify-write that sends every rule back with the version it
read, ids unchanged. If the stage changes in between, the write is retried
once on a fresh read. A change that leaves the rule as it was sends nothing
and exits 0, so a rollback ('--rollout now=0' on a rule already at 0%) is
safe to repeat.

` + fmConditionHelp + `

--condition replaces the rule's condition on the same context key, or adds
one. --condition-add and --condition-remove add or remove values in the
condition with that key and operator without retyping the list (any operator
that takes a list, not between): values already there, or not there, are
skipped, and removing the last value removes the condition.
--condition-clear <key> drops the conditions on a context key.

Examples:
  # Add a user to a list and remove another
  dtctl update fm feature-targeting new-checkout --position 0 --stage production \
    --condition-add 'targetingKey = user-3' --condition-remove 'targetingKey = user-1'

  # Change the rollout: 25% now, 100% tomorrow, rising linearly
  dtctl update fm feature-targeting new-checkout --position a93d07e1 --stage production \
    --rollout now=25 --rollout +1d=100,linear

  # Roll back
  dtctl update fm feature-targeting new-checkout --position a93d07e1 --stage production --rollout now=0

  # Move a rule to the top
  dtctl update fm feature-targeting new-checkout --position 2 --stage production --move-position 0
`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := requireFMStage(cmd, stage)
			if err != nil {
				return err
			}
			if !cmd.Flags().Changed("position") {
				return fmt.Errorf("--position is required: the rule to change, by 0-based position or id")
			}
			if clearRollout && len(rule.rollout) > 0 {
				return fmt.Errorf("--clear-rollout and --rollout cannot be combined")
			}
			spec := rule.spec(cmd)
			spec.Position = position
			spec.MovePosition = movePosition
			spec.ConditionAdd = conditionAdd
			spec.ConditionRemove = conditionRemove
			spec.ConditionClear = condClear
			spec.ClearRollout = clearRollout
			if !fmRuleSpecChanges(cmd) {
				return fmt.Errorf("nothing to update: pass at least one of --condition, --condition-add, --condition-remove, --condition-clear, --variant, --rollout, --clear-rollout, --bucketing-key, --move-position or --name")
			}
			_, project, h, printer, err := setupFMWrite(cmd, safety.OperationUpdate)
			if err != nil {
				return err
			}
			res, err := h.UpdateTargetingRule(project, args[0], s, spec, featuremanagement.TargetingWriteOptions{Version: version})
			if err != nil {
				return err
			}
			ft := res.Targeting
			ref := ruleRef(res.Position, res.RuleID)
			msg := fmt.Sprintf("Updated %s in stage %s of feature %s", ref, ft.ReleaseStageKey, ft.FeatureKey)
			if !res.Changed {
				msg = fmt.Sprintf("Nothing to change: %s in stage %s of feature %s already is as requested", ref, ft.ReleaseStageKey, ft.FeatureKey)
			}
			return printFMTargetingWrite(cmdContext(cmd), printer, "update", msg, res)
		},
	}
	c.Flags().SortFlags = false
	c.Flags().StringVarP(&stage, fmStageFlag, "s", "", fmTargetingStageHelp)
	c.Flags().StringVar(&position, "position", "", "rule to change, by 0-based position or id (required)")
	c.Flags().StringArrayVar(&rule.conditions, "condition", nil, "add a condition, or replace the one on the same context key (repeatable)")
	c.Flags().StringArrayVar(&conditionAdd, "condition-add", nil, "add values to the condition with this key and operator; creates it if needed (repeatable)")
	c.Flags().StringArrayVar(&conditionRemove, "condition-remove", nil, "remove values from the condition with this key and operator; the last value removes the condition (repeatable)")
	c.Flags().StringArrayVar(&condClear, "condition-clear", nil, "remove the conditions on this context key (repeatable)")
	c.Flags().StringVar(&rule.variant, "variant", "", "new variant to serve, or a split")
	c.Flags().StringArrayVar(&rule.rollout, "rollout", nil, "replace the rollout steps, each <time>=<percent>[,linear] (repeatable)")
	c.Flags().BoolVar(&clearRollout, "clear-rollout", false, "remove the rollout, so the rule serves 100% of matches")
	c.Flags().StringVar(&rule.bucketingKey, "bucketing-key", "", "new context key for rollout buckets; doesn't change which requests match")
	c.Flags().StringVar(&movePosition, "move-position", "", "move the rule to this 0-based position, or to the place of the rule with this id")
	c.Flags().StringVar(&rule.name, "name", "", "new display name")
	c.Flags().StringVar(&version, "version", "", fmVersionHelp)
	return c
}

// fmRuleSpecChanges reports whether an update was given anything to change.
func fmRuleSpecChanges(cmd *cobra.Command) bool {
	for _, name := range []string{"condition", "condition-add", "condition-remove", "condition-clear", "variant", "rollout", "clear-rollout", "bucketing-key", "move-position", "name"} {
		if cmd.Flags().Changed(name) {
			return true
		}
	}
	return false
}

func newDeleteFMFeatureTargetingCmd() *cobra.Command {
	var (
		stage, position, version string
		all, yes                 bool
	)
	c := &cobra.Command{
		Use:     "feature-targeting <feature-key-or-name> --stage <stage> (--position <position-or-id> | --all)",
		Aliases: []string{"feature-targeting-rule"},
		Short:   "Remove targeting rules from one release stage of a feature",
		Long: `Remove one rule from one release stage's targeting of a feature, or every
rule with --all.

The stage's targeting itself always exists: --all leaves it with no rules,
and doesn't change whether targeting is on. Requests in a stage with no rules
get the default defined in code. --all asks for confirmation unless -y is
given; without a terminal to ask on, it needs -y.

This is a read-modify-write that sends the remaining rules back with the
version it read, ids unchanged.

Examples:
  # Remove one rule
  dtctl delete fm feature-targeting new-checkout --position 2 --stage production

  # Remove every rule in a stage
  dtctl delete fm feature-targeting new-checkout --stage staging --all
`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmdContext(cmd)
			s, err := requireFMStage(cmd, stage)
			if err != nil {
				return err
			}
			switch {
			case all && cmd.Flags().Changed("position"):
				return fmt.Errorf("--position and --all cannot be combined")
			case !all && !cmd.Flags().Changed("position"):
				return fmt.Errorf("pass --position <position-or-id>, or --all")
			}
			if all && !yes {
				if plainMode(ctx) || agentMode(ctx) {
					return fmt.Errorf("--all removes every rule in stage %s; pass -y to confirm", s)
				}
				if !prompt.ConfirmWith(currentStdin(ctx), currentStdout(ctx), fmt.Sprintf("Remove every targeting rule of feature %s in stage %s?", args[0], s)) {
					output.FprintInfo(currentStderr(ctx), "Deletion cancelled")
					return nil
				}
			}
			_, project, h, printer, err := setupFMWrite(cmd, safety.OperationDelete)
			if err != nil {
				return err
			}
			opts := featuremanagement.TargetingWriteOptions{Version: version}
			if all {
				res, err := h.ClearTargetingRules(project, args[0], s, opts)
				if err != nil {
					return err
				}
				ft := res.Targeting
				msg := fmt.Sprintf("Removed %d rule(s) from stage %s of feature %s; targeting stays %s", res.Removed, ft.ReleaseStageKey, ft.FeatureKey, onOffWord(ft.Enabled))
				if !res.Changed {
					msg = fmt.Sprintf("Nothing to delete: stage %s of feature %s has no rules", ft.ReleaseStageKey, ft.FeatureKey)
				}
				return printFMTargetingWrite(ctx, printer, "delete", msg, res)
			}
			res, err := h.DeleteTargetingRule(project, args[0], s, position, opts)
			if err != nil {
				return err
			}
			ft := res.Targeting
			msg := fmt.Sprintf("Deleted rule %s (%s) from stage %s of feature %s", position, featuremanagement.ShortRuleID(res.RuleID), ft.ReleaseStageKey, ft.FeatureKey)
			return printFMTargetingWrite(ctx, printer, "delete", msg, res)
		},
	}
	c.Flags().SortFlags = false
	c.Flags().StringVarP(&stage, fmStageFlag, "s", "", fmTargetingStageHelp)
	c.Flags().StringVar(&position, "position", "", "rule to remove, by 0-based position or id (required unless --all)")
	c.Flags().BoolVar(&all, "all", false, "remove every rule in the stage; asks for confirmation unless -y")
	c.Flags().BoolVarP(&yes, "yes", "y", false, "skip the confirmation prompt")
	c.Flags().StringVar(&version, "version", "", fmVersionHelp)
	markPrompts(c, "yes")
	return c
}

func onOffWord(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

// newFMFeatureToggleCmd builds `enable fm feature` and `disable fm feature`,
// which flip one stage's targeting switch and change nothing else.
func newFMFeatureToggleCmd(enable bool) *cobra.Command {
	var stage, version string
	verb, state := "disable", "off"
	if enable {
		verb, state = "enable", "on"
	}
	c := &cobra.Command{
		Use:     "feature <feature-key-or-name> --stage <stage>",
		Aliases: []string{"features"},
		Short:   fmt.Sprintf("Turn a feature's targeting %s in one release stage", state),
		Long: fmt.Sprintf(`Turn a feature's targeting %[1]s in one release stage: the stage's on/off switch.
The rules are left as they are. A stage already %[1]s is left alone, and the
command exits 0.

A stage with no rules can be turned on; it then serves the default defined in
code. This is a read-modify-write that sends the stage's rules back
unchanged, ids included, with the version it read.

Examples:
  dtctl %[2]s fm feature new-checkout --stage production
`, state, verb),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := requireFMStage(cmd, stage)
			if err != nil {
				return err
			}
			_, project, h, printer, err := setupFMWrite(cmd, safety.OperationUpdate)
			if err != nil {
				return err
			}
			res, err := h.SetTargetingEnabled(project, args[0], s, enable, featuremanagement.TargetingWriteOptions{Version: version})
			if err != nil {
				return err
			}
			ft := res.Targeting
			msg := fmt.Sprintf("Turned targeting %s for feature %s in stage %s", state, ft.FeatureKey, ft.ReleaseStageKey)
			if !res.Changed {
				msg = fmt.Sprintf("Nothing to change: targeting of feature %s in stage %s is already %s", ft.FeatureKey, ft.ReleaseStageKey, state)
			}
			return printFMTargetingWrite(cmdContext(cmd), printer, verb, msg, res)
		},
	}
	c.Flags().SortFlags = false
	c.Flags().StringVarP(&stage, fmStageFlag, "s", "", fmTargetingStageHelp)
	c.Flags().StringVar(&version, "version", "", fmVersionHelp)
	return c
}
