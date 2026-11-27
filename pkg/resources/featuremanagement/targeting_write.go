package featuremanagement

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	sdkfm "github.com/Dynatrace/dtctl/sdk/api/featuremanagement"
	"github.com/Dynatrace/dtctl/sdk/httpclient"
)

// FeatureTargetingUpdate is the body of a targeting write.
type FeatureTargetingUpdate = sdkfm.FeatureTargetingUpdate

// TargetingWriteOptions are the write controls every targeting command
// shares.
type TargetingWriteOptions struct {
	// Version, when set, is sent as the optimistic-locking version in place
	// of the one just read, so the write fails with 409 unless the stage is
	// still at it. A write with Version set is never retried.
	Version string
	// Now is the instant `now` and offsets resolve to. Zero means the time
	// the write starts; it is resolved once, so every step of one command
	// agrees.
	Now time.Time
}

// RuleSpec is what a rule write changes. A nil pointer or empty list means
// the flag was not given.
type RuleSpec struct {
	// Position selects the rule on update and delete, and is where a new
	// rule goes on create (default: last).
	Position string
	// MovePosition moves the rule on update.
	MovePosition string

	Conditions      []string // --condition: add, or replace the condition on the same key
	ConditionAdd    []string // --condition-add
	ConditionRemove []string // --condition-remove
	ConditionClear  []string // --condition-clear: context keys

	Variant      *string
	Rollout      []string
	ClearRollout bool
	BucketingKey *string
	Name         *string
}

// needsKeyTypes reports whether the spec names context keys, whose value
// types (and existence) come from the project's context attributes.
func (s RuleSpec) needsKeyTypes() bool {
	return len(s.Conditions)+len(s.ConditionAdd)+len(s.ConditionRemove) > 0 || (s.BucketingKey != nil && *s.BucketingKey != "")
}

// TargetingWriteResult is the outcome of a targeting write.
type TargetingWriteResult struct {
	// Targeting is the stage's targeting after the write, or as read when
	// nothing changed.
	Targeting *FeatureTargeting
	// Changed is false when the stage already was what the command asked
	// for, so nothing was sent.
	Changed bool
	// Position is the 0-based position of the rule the command wrote, or -1
	// for a whole-stage write (enable, disable, delete --all, a deleted
	// rule).
	Position int
	// RuleID is the id of the rule the command wrote or deleted, when the
	// server returned one.
	RuleID string
	// Removed is the number of rules a delete removed.
	Removed int
	// Warnings are rules the write leaves shadowed by a rule that matches all
	// requests.
	Warnings []string
}

// CreateTargetingRule adds a rule to one stage's targeting. Existing rules
// go back unchanged, ids included; the new one goes without an id, which the
// service assigns.
func (h *Handler) CreateTargetingRule(projectKey, featureKeyOrName, stage string, spec RuleSpec, opts TargetingWriteOptions) (*TargetingWriteResult, error) {
	if spec.Variant == nil {
		return nil, fmt.Errorf("--variant is required")
	}
	return h.writeTargeting(projectKey, featureKeyOrName, stage, opts, spec, func(env *ruleEnv, ft *FeatureTargeting) (int, string, error) {
		rule := TargetingRule{}
		if err := env.apply(&rule, spec); err != nil {
			return 0, "", err
		}
		at := len(ft.Targeting.Rules)
		if spec.Position != "" {
			p, err := ResolveRulePosition(ft.Targeting.Rules, spec.Position, true)
			if err != nil {
				return 0, "", err
			}
			at = p
		}
		rules := append([]TargetingRule{}, ft.Targeting.Rules[:at]...)
		rules = append(rules, rule)
		ft.Targeting.Rules = append(rules, ft.Targeting.Rules[at:]...)
		return at, "", nil
	})
}

// UpdateTargetingRule changes one rule; what spec leaves unset is kept.
func (h *Handler) UpdateTargetingRule(projectKey, featureKeyOrName, stage string, spec RuleSpec, opts TargetingWriteOptions) (*TargetingWriteResult, error) {
	if spec.Position == "" {
		return nil, fmt.Errorf("--position is required: pass a 0-based position or a rule id")
	}
	return h.writeTargeting(projectKey, featureKeyOrName, stage, opts, spec, func(env *ruleEnv, ft *FeatureTargeting) (int, string, error) {
		rules := ft.Targeting.Rules
		i, err := ResolveRulePosition(rules, spec.Position, false)
		if err != nil {
			return 0, "", err
		}
		target := -1
		if spec.MovePosition != "" {
			if target, err = ResolveRulePosition(rules, spec.MovePosition, false); err != nil {
				return 0, "", fmt.Errorf("--move-position: %w", err)
			}
		}
		if err := env.apply(&rules[i], spec); err != nil {
			return 0, "", err
		}
		if target >= 0 && target != i {
			rule := rules[i]
			rest := append(append([]TargetingRule{}, rules[:i]...), rules[i+1:]...)
			moved := append([]TargetingRule{}, rest[:target]...)
			moved = append(moved, rule)
			ft.Targeting.Rules = append(moved, rest[target:]...)
			return target, "", nil
		}
		return i, "", nil
	})
}

// DeleteTargetingRule removes one rule.
func (h *Handler) DeleteTargetingRule(projectKey, featureKeyOrName, stage, position string, opts TargetingWriteOptions) (*TargetingWriteResult, error) {
	if position == "" {
		return nil, fmt.Errorf("pass --position <position-or-id>, or --all")
	}
	return h.writeTargeting(projectKey, featureKeyOrName, stage, opts, RuleSpec{}, func(_ *ruleEnv, ft *FeatureTargeting) (int, string, error) {
		rules := ft.Targeting.Rules
		i, err := ResolveRulePosition(rules, position, false)
		if err != nil {
			return 0, "", err
		}
		id := rules[i].ID
		ft.Targeting.Rules = append(append([]TargetingRule{}, rules[:i]...), rules[i+1:]...)
		return -1, id, nil
	})
}

// ClearTargetingRules removes every rule from a stage and leaves its on/off
// switch as read. The API has no delete for a stage's targeting: a stage
// always has one, possibly with no rules.
func (h *Handler) ClearTargetingRules(projectKey, featureKeyOrName, stage string, opts TargetingWriteOptions) (*TargetingWriteResult, error) {
	return h.writeTargeting(projectKey, featureKeyOrName, stage, opts, RuleSpec{}, func(_ *ruleEnv, ft *FeatureTargeting) (int, string, error) {
		ft.Targeting.Rules = []TargetingRule{}
		return -1, "", nil
	})
}

// SetTargetingEnabled turns targeting on or off in one stage and changes
// nothing else. A stage already in that state is left alone.
func (h *Handler) SetTargetingEnabled(projectKey, featureKeyOrName, stage string, enabled bool, opts TargetingWriteOptions) (*TargetingWriteResult, error) {
	return h.writeTargeting(projectKey, featureKeyOrName, stage, opts, RuleSpec{}, func(_ *ruleEnv, ft *FeatureTargeting) (int, string, error) {
		ft.Enabled = enabled
		return -1, "", nil
	})
}

// ruleEnv is what a rule edit needs from the project: the feature's
// variants and the context keys' value types, each read once per command and
// only when the spec needs it.
type ruleEnv struct {
	projectKey string
	variants   []Variant
	keyTypes   ContextKeyTypes
	now        time.Time
}

// writeTargeting is the read-modify-write every targeting command shares.
// edit changes a copy of the stage's targeting in place and returns the
// position of the rule it wrote (-1 for none) and the id of a rule it
// removed. writeTargeting reads the stage, applies edit to a copy, and sends the whole targeting
// back with the version it read. When the server answers 409 because the
// stage changed in between, it reads and applies edit once more, so an
// unattended rollback survives a concurrent write; a second 409 fails.
func (h *Handler) writeTargeting(projectKey, featureKeyOrName, stage string, opts TargetingWriteOptions, spec RuleSpec, edit func(*ruleEnv, *FeatureTargeting) (int, string, error)) (*TargetingWriteResult, error) {
	env := &ruleEnv{projectKey: projectKey, now: opts.Now}
	if env.now.IsZero() {
		env.now = time.Now()
	}
	env.now = env.now.UTC()

	current, err := h.GetFeatureTargeting(projectKey, featureKeyOrName, stage)
	if err != nil {
		return nil, err
	}
	featureKey := current.FeatureKey
	if featureKey == "" {
		featureKey = featureKeyOrName
	}
	if spec.Variant != nil {
		f, err := h.GetFeature(projectKey, featureKey)
		if err != nil {
			return nil, err
		}
		env.variants = f.Variants
	}
	if spec.needsKeyTypes() {
		attrs, err := h.ListContextAttributes(projectKey)
		if err != nil {
			return nil, fmt.Errorf("read the project's context attributes for the condition value types: %w", err)
		}
		env.keyTypes = NewContextKeyTypes(attrs)
	}

	for attempt := 0; ; attempt++ {
		next, err := copyTargeting(current)
		if err != nil {
			return nil, err
		}
		position, removedID, err := edit(env, next)
		if err != nil {
			return nil, err
		}
		result := &TargetingWriteResult{Targeting: current, Position: position, RuleID: removedID,
			Removed: len(current.Targeting.Rules) - len(next.Targeting.Rules)}
		if result.Removed < 0 {
			result.Removed = 0
		}
		if sameTargeting(current, next, env.now) {
			if position >= 0 && position < len(current.Targeting.Rules) {
				result.RuleID = current.Targeting.Rules[position].ID
			}
			result.Warnings = ShadowedRules(current.Targeting.Rules)
			return result, nil
		}

		version := opts.Version
		if version == "" {
			version = strconv.Itoa(current.Version)
		}
		updated, err := h.sdk.UpdateFeatureTargeting(context.Background(), projectKey, featureKey, current.ReleaseStageKey,
			FeatureTargetingUpdate{Enabled: next.Enabled, Targeting: next.Targeting}, version)
		if err != nil {
			if errors.Is(err, httpclient.ErrConflict) {
				if opts.Version == "" && attempt == 0 {
					if current, err = h.GetFeatureTargeting(projectKey, featureKey, current.ReleaseStageKey); err != nil {
						return nil, err
					}
					continue
				}
				return nil, fmt.Errorf("the targeting of feature %q in stage %q changed since it was read; re-run to retry: %w", featureKey, current.ReleaseStageKey, err)
			}
			return nil, err
		}
		if updated == nil || updated.ReleaseStageKey == "" {
			// The server returned no body: what was sent is the new state.
			next.Version = current.Version + 1
			updated = next
		}
		result.Targeting = updated
		result.Changed = true
		if position >= 0 && position < len(updated.Targeting.Rules) {
			result.RuleID = updated.Targeting.Rules[position].ID
		}
		result.Warnings = ShadowedRules(updated.Targeting.Rules)
		return result, nil
	}
}

// apply changes one rule as spec says, in a fixed order: conditions cleared,
// set, extended and reduced, then variant, rollout, bucketing key and name.
func (env *ruleEnv) apply(rule *TargetingRule, spec RuleSpec) error {
	for _, key := range spec.ConditionClear {
		rule.Conditions = withoutKey(rule.Conditions, key)
	}
	set := map[string]bool{}
	for _, s := range spec.Conditions {
		c, err := env.condition(s)
		if err != nil {
			return err
		}
		if !set[c.ContextKey] {
			// The first --condition on a key replaces what the rule had on
			// it; further ones on the same key add to it.
			rule.Conditions = withoutKey(rule.Conditions, c.ContextKey)
			set[c.ContextKey] = true
		}
		rule.Conditions = append(rule.Conditions, c)
	}
	for _, s := range spec.ConditionAdd {
		if err := env.changeValues(rule, s, true); err != nil {
			return err
		}
	}
	for _, s := range spec.ConditionRemove {
		if err := env.changeValues(rule, s, false); err != nil {
			return err
		}
	}
	if spec.Variant != nil {
		serve, err := ParseServe(*spec.Variant, env.variants)
		if err != nil {
			return err
		}
		rule.Serve = serve
	}
	if spec.ClearRollout {
		rule.Rollout = nil
	}
	if len(spec.Rollout) > 0 {
		r, err := ParseRollout(spec.Rollout, env.now)
		if err != nil {
			return err
		}
		if rule.Rollout != nil {
			// Keep members of the rollout this dtctl doesn't know.
			kept := *rule.Rollout
			kept.Steps = r.Steps
			r = &kept
		}
		rule.Rollout = r
	}
	if spec.BucketingKey != nil {
		if k := *spec.BucketingKey; k != "" {
			if _, err := env.keyTypes.typeOf(env.projectKey, k); err != nil {
				return fmt.Errorf("--bucketing-key: %w", err)
			}
		}
		rule.BucketingKey = *spec.BucketingKey
	}
	if spec.Name != nil {
		rule.Name = *spec.Name
	}
	return nil
}

func (env *ruleEnv) condition(s string) (TargetingCondition, error) {
	e, err := ParseConditionExpr(s)
	if err != nil {
		return TargetingCondition{}, err
	}
	return env.keyTypes.TypedCondition(env.projectKey, e, env.now)
}

// changeValues adds or removes values in the rule's condition with the same
// context key and operator. Adding creates the condition when there is none;
// removing the last value removes it. Values already there (or not there)
// are skipped.
func (env *ruleEnv) changeValues(rule *TargetingRule, s string, add bool) error {
	flag := "--condition-remove"
	if add {
		flag = "--condition-add"
	}
	e, err := ParseConditionExpr(s)
	if err != nil {
		return fmt.Errorf("%s: %w", flag, err)
	}
	if !IsListOperator(e.Operator) {
		return fmt.Errorf("%s %q: %s does not take a list of values; use --condition to replace the condition", flag, s, e.Operator)
	}
	c, err := env.keyTypes.TypedCondition(env.projectKey, e, env.now)
	if err != nil {
		return err
	}
	idx := -1
	for i, existing := range rule.Conditions {
		if existing.ContextKey == c.ContextKey && existing.Operator == c.Operator {
			idx = i
			break
		}
	}
	if idx < 0 {
		if add {
			rule.Conditions = append(rule.Conditions, c)
		}
		return nil
	}
	existing := &rule.Conditions[idx]
	have := map[string]bool{}
	for _, v := range existing.Values {
		have[conditionValueKey(existing.ValueType, v)] = true
	}
	if add {
		for _, v := range c.Values {
			if k := conditionValueKey(c.ValueType, v); !have[k] {
				existing.Values = append(existing.Values, v)
				have[k] = true
			}
		}
		return nil
	}
	drop := map[string]bool{}
	for _, v := range c.Values {
		drop[conditionValueKey(c.ValueType, v)] = true
	}
	kept := existing.Values[:0:0]
	for _, v := range existing.Values {
		if !drop[conditionValueKey(existing.ValueType, v)] {
			kept = append(kept, v)
		}
	}
	if len(kept) == 0 {
		rule.Conditions = append(rule.Conditions[:idx:idx], rule.Conditions[idx+1:]...)
		return nil
	}
	existing.Values = kept
	return nil
}

func withoutKey(cs []TargetingCondition, key string) []TargetingCondition {
	out := cs[:0:0]
	for _, c := range cs {
		if c.ContextKey != key {
			out = append(out, c)
		}
	}
	return out
}

// copyTargeting deep-copies a stage's targeting through its JSON encoding,
// which keeps the members dtctl does not declare.
func copyTargeting(ft *FeatureTargeting) (*FeatureTargeting, error) {
	data, err := json.Marshal(ft)
	if err != nil {
		return nil, err
	}
	var out FeatureTargeting
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	if out.Targeting.Rules == nil {
		out.Targeting.Rules = []TargetingRule{}
	}
	return &out, nil
}

// sameTargeting reports whether a write would leave the stage as it is. Two
// rollouts that each hold one percentage from before now on are the same,
// whatever instant each was written at, so `--rollout now=0` on a rule
// already at 0% changes nothing.
func sameTargeting(a, b *FeatureTargeting, now time.Time) bool {
	if a.Enabled != b.Enabled || len(a.Targeting.Rules) != len(b.Targeting.Rules) {
		return false
	}
	for i := range a.Targeting.Rules {
		ra, rb := a.Targeting.Rules[i], b.Targeting.Rules[i]
		if settledRollout(ra.Rollout, now) && settledRollout(rb.Rollout, now) &&
			ra.Rollout.Steps[0].Percentage == rb.Rollout.Steps[0].Percentage {
			ra.Rollout, rb.Rollout = nil, nil
		}
		ja, errA := json.Marshal(ra)
		jb, errB := json.Marshal(rb)
		if errA != nil || errB != nil || string(ja) != string(jb) {
			return false
		}
	}
	return true
}

// settledRollout is a rollout of one step that took effect at or before now.
func settledRollout(r *Rollout, now time.Time) bool {
	return r != nil && len(r.Steps) == 1 && !r.Steps[0].EffectiveTime.After(now)
}
