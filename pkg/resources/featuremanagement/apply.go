package featuremanagement

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/Dynatrace/dtctl/pkg/diff"
	"github.com/Dynatrace/dtctl/pkg/util/format"
	sdkfm "github.com/Dynatrace/dtctl/sdk/api/featuremanagement"
	"github.com/Dynatrace/dtctl/sdk/httpclient"
)

// ApplyOptions control a typed FM apply.
type ApplyOptions struct {
	// DryRun checks the file and computes the change, and sends nothing.
	DryRun bool
	// Force applies a targeting file that drops or changes an existing
	// rule's id, which re-buckets the rule's users.
	Force bool
}

// Apply actions.
const (
	ApplyCreated   = "created"
	ApplyUpdated   = "updated"
	ApplyUnchanged = "unchanged"
)

// ApplyResult is the outcome of a typed FM apply, or under DryRun what it
// would be.
type ApplyResult struct {
	// Action is ApplyCreated, ApplyUpdated or ApplyUnchanged.
	Action string
	// Key names what was applied: the feature, or the feature and stage.
	Key string
	// Diff is the change in unified form, from what is live to what the
	// file asks for; empty for a create or no change.
	Diff string
	// Feature or Targeting is the resource after the write, or as it would
	// be sent under DryRun.
	Feature   *Feature
	Targeting *FeatureTargeting
	// Warnings are follow-ups, such as rules shadowed by a catch-all rule.
	Warnings []string
}

// readApplyFile parses an apply file into a JSON object, after checking its
// optional kind.
func readApplyFile(data []byte, kind string) (map[string]any, []byte, error) {
	jsonData, err := format.YAMLToJSON(data)
	if err != nil {
		return nil, nil, err
	}
	var doc map[string]any
	if err := json.Unmarshal(jsonData, &doc); err != nil {
		return nil, nil, fmt.Errorf("the file must hold one object: %w", err)
	}
	if k, ok := doc["kind"]; ok {
		if k != kind {
			return nil, nil, fmt.Errorf("the file is a %v, not a %s", k, kind)
		}
		delete(doc, "kind")
	}
	clean, err := json.Marshal(doc)
	return doc, clean, err
}

// fileVersion is the optimistic-locking version a file carries, or
// fallback when it has none.
func fileVersion(doc map[string]any, fallback int) (string, error) {
	v, ok := doc["version"]
	if !ok || v == nil {
		return strconv.Itoa(fallback), nil
	}
	switch n := v.(type) {
	case float64:
		if n != math.Trunc(n) || n < 0 {
			return "", fmt.Errorf("the file's version %v is not a version", v)
		}
		return strconv.FormatInt(int64(n), 10), nil
	case string:
		if _, err := strconv.Atoi(n); err != nil {
			return "", fmt.Errorf("the file's version %q is not a version", n)
		}
		return n, nil
	}
	return "", fmt.Errorf("the file's version %v is not a version", v)
}

// unifiedDiff renders the change from live to file, both request bodies.
func unifiedDiff(live, file any, what string) (string, error) {
	var l, f any
	for _, p := range []struct {
		in  any
		out *any
	}{{live, &l}, {file, &f}} {
		b, err := json.Marshal(p.in)
		if err != nil {
			return "", err
		}
		if err := json.Unmarshal(b, p.out); err != nil {
			return "", err
		}
	}
	res, err := diff.NewDiffer(diff.DiffOptions{Format: diff.DiffFormatUnified, ContextLines: 3}).Compare(l, f, "live "+what, "file "+what)
	if err != nil || !res.HasChanges {
		return "", err
	}
	return res.Patch, nil
}

// checkFlagValues holds every flag to a value for every variant, and
// variant and flag keys to be unique.
func checkFlagValues(variants []Variant, flags []FeatureFlag) error {
	seen := map[string]bool{}
	for _, v := range variants {
		if v.Key == "" || seen[v.Key] {
			return fmt.Errorf("variant keys must be unique and non-empty")
		}
		seen[v.Key] = true
	}
	flagSeen := map[string]bool{}
	for _, f := range flags {
		if f.Key == "" || flagSeen[f.Key] {
			return fmt.Errorf("flag keys must be unique and non-empty")
		}
		flagSeen[f.Key] = true
		var missing []string
		for _, v := range variants {
			if _, ok := f.Variants[v.Key]; !ok {
				missing = append(missing, v.Key)
			}
		}
		if len(missing) > 0 {
			return fmt.Errorf("flag %s has no value for variant(s) %s; every flag needs a value for every variant", f.Key, strings.Join(missing, ", "))
		}
		for k := range f.Variants {
			if !seen[k] {
				return fmt.Errorf("flag %s has a value for %q, which is not a variant of the feature", f.Key, k)
			}
		}
	}
	return nil
}

// ApplyFeature creates or replaces a feature from a file: the API's read
// model, as get fm feature -o yaml exports it. An existing feature is
// replaced with the version the file carries (the live one when it has
// none), so a feature changed since the export fails with a conflict.
func (h *Handler) ApplyFeature(projectKey string, data []byte, opts ApplyOptions) (*ApplyResult, error) {
	doc, clean, err := readApplyFile(data, "fm/feature")
	if err != nil {
		return nil, err
	}
	if p, ok := doc["projectKey"]; ok && p != projectKey {
		return nil, fmt.Errorf("the file's projectKey is %v, but the command targets %s; delete projectKey from the file to use it here", p, projectKey)
	}
	key, _ := doc["key"].(string)
	if key == "" {
		return nil, fmt.Errorf("the file has no key")
	}
	var desired sdkfm.FeatureDetails
	if err := json.Unmarshal(clean, &desired); err != nil {
		return nil, err
	}
	if err := checkFlagValues(desired.Variants, desired.Flags); err != nil {
		return nil, err
	}

	current, err := h.sdk.GetFeature(context.Background(), projectKey, key)
	if errors.Is(err, httpclient.ErrNotFound) {
		var body FeatureCreate
		if err := FeatureFile(projectKey).Decode(data, &body); err != nil {
			return nil, err
		}
		if opts.DryRun {
			f := fromSDKFeature(&desired)
			return &ApplyResult{Action: ApplyCreated, Key: key, Feature: &f}, nil
		}
		res, err := h.CreateFeature(projectKey, body)
		if err != nil {
			return nil, err
		}
		return &ApplyResult{Action: ApplyCreated, Key: key, Feature: res.Feature}, nil
	}
	if err != nil {
		return nil, err
	}

	version, err := fileVersion(doc, current.Version)
	if err != nil {
		return nil, err
	}
	live, body := sdkfm.NewFeatureUpdate(current), sdkfm.NewFeatureUpdate(&desired)
	d, err := unifiedDiff(live, body, "feature "+key)
	if err != nil {
		return nil, err
	}
	if sameJSON(live, body) {
		f := fromSDKFeature(current)
		return &ApplyResult{Action: ApplyUnchanged, Key: key, Feature: &f}, nil
	}
	if opts.DryRun {
		f := fromSDKFeature(&desired)
		return &ApplyResult{Action: ApplyUpdated, Key: key, Diff: d, Feature: &f}, nil
	}
	updated, err := h.sdk.UpdateFeature(context.Background(), projectKey, key, body, version)
	if err != nil {
		return nil, conflictError(err, fmt.Sprintf("feature %q", key))
	}
	f := fromSDKFeature(updated)
	return &ApplyResult{Action: ApplyUpdated, Key: key, Diff: d, Feature: &f}, nil
}

// ApplyFeatureTargeting replaces one stage's targeting of a feature from a
// file: the API's read model, as get fm feature-targeting <f> -s <stage>
// -o yaml exports it (an export of every stage works too; the stage's
// entry is used). It sends the version the file carries (the live one when
// it has none), and refuses a file that drops or changes an existing rule's
// id unless opts.Force.
func (h *Handler) ApplyFeatureTargeting(projectKey, featureKeyOrName, stage string, data []byte, opts ApplyOptions) (*ApplyResult, error) {
	doc, err := targetingDoc(data, stage)
	if err != nil {
		return nil, err
	}
	current, err := h.GetFeatureTargeting(projectKey, featureKeyOrName, stage)
	if err != nil {
		return nil, err
	}
	for member, want := range map[string]string{"projectKey": projectKey, "featureKey": current.FeatureKey, "releaseStageKey": current.ReleaseStageKey} {
		if got, ok := doc[member]; ok && got != want {
			return nil, fmt.Errorf("the file's %s is %v, but the command targets %s; delete %s from the file to use it here", member, got, want, member)
		}
	}
	if _, ok := doc["targeting"]; !ok {
		return nil, fmt.Errorf("the file has no targeting")
	}
	clean, err := json.Marshal(doc)
	if err != nil {
		return nil, err
	}
	var desired sdkfm.FeatureTargetingDetails
	if err := json.Unmarshal(clean, &desired); err != nil {
		return nil, err
	}
	enabled := current.Enabled
	if e, ok := doc["enabled"].(bool); ok {
		enabled = e
	}
	body := FeatureTargetingUpdate{Enabled: enabled, Targeting: desired.Targeting}
	live := FeatureTargetingUpdate{Enabled: current.Enabled, Targeting: current.Targeting}
	what := fmt.Sprintf("feature %s in stage %s", current.FeatureKey, current.ReleaseStageKey)

	if err := h.checkTargeting(projectKey, current.FeatureKey, body.Targeting); err != nil {
		return nil, err
	}
	if err := checkRuleIDs(current.Targeting.Rules, body.Targeting.Rules, opts.Force); err != nil {
		return nil, err
	}
	version, err := fileVersion(doc, current.Version)
	if err != nil {
		return nil, err
	}
	d, err := unifiedDiff(live, body, "targeting")
	if err != nil {
		return nil, err
	}
	key := current.FeatureKey + "/" + current.ReleaseStageKey
	if sameJSON(live, body) {
		return &ApplyResult{Action: ApplyUnchanged, Key: key, Targeting: current, Warnings: ShadowedRules(current.Targeting.Rules)}, nil
	}
	if opts.DryRun {
		next := *current
		next.Enabled, next.Targeting = body.Enabled, body.Targeting
		return &ApplyResult{Action: ApplyUpdated, Key: key, Diff: d, Targeting: &next, Warnings: ShadowedRules(next.Targeting.Rules)}, nil
	}
	updated, err := h.sdk.UpdateFeatureTargeting(context.Background(), projectKey, current.FeatureKey, current.ReleaseStageKey, body, version)
	if err != nil {
		return nil, conflictError(err, "the targeting of "+what)
	}
	if updated == nil || updated.ReleaseStageKey == "" {
		if updated, err = h.GetFeatureTargeting(projectKey, current.FeatureKey, current.ReleaseStageKey); err != nil {
			return nil, err
		}
	}
	return &ApplyResult{Action: ApplyUpdated, Key: key, Diff: d, Targeting: updated, Warnings: ShadowedRules(updated.Targeting.Rules)}, nil
}

// targetingDoc reads a targeting file: one stage's targeting, or a list of
// stages from which the one for stage is taken.
func targetingDoc(data []byte, stage string) (map[string]any, error) {
	jsonData, err := format.YAMLToJSON(data)
	if err != nil {
		return nil, err
	}
	var list []map[string]any
	if json.Unmarshal(jsonData, &list) == nil {
		var stages []string
		for _, item := range list {
			if item["releaseStageKey"] == stage {
				return withoutKind(item, "fm/feature-targeting")
			}
			stages = append(stages, fmt.Sprint(item["releaseStageKey"]))
		}
		return nil, fmt.Errorf("the file has no targeting for stage %s (stages: %s)", stage, strings.Join(stages, ", "))
	}
	var doc map[string]any
	if err := json.Unmarshal(jsonData, &doc); err != nil {
		return nil, fmt.Errorf("the file must hold a targeting object or a list of them: %w", err)
	}
	return withoutKind(doc, "fm/feature-targeting")
}

func withoutKind(doc map[string]any, kind string) (map[string]any, error) {
	if k, ok := doc["kind"]; ok {
		if k != kind {
			return nil, fmt.Errorf("the file is a %v, not a %s", k, kind)
		}
		delete(doc, "kind")
	}
	return doc, nil
}

// checkTargeting runs the targeting write commands' checks on a whole
// targeting: served variants exist and total 100%, conditions name context
// attributes with their value type and an operator it supports, bucketing
// keys are context attributes, and rollout steps are percentages in time
// order.
func (h *Handler) checkTargeting(projectKey, featureKey string, t Targeting) error {
	f, err := h.GetFeature(projectKey, featureKey)
	if err != nil {
		return err
	}
	attrs, err := h.ListContextAttributes(projectKey)
	if err != nil {
		return fmt.Errorf("read the project's context attributes for the condition value types: %w", err)
	}
	types := NewContextKeyTypes(attrs)
	for i, r := range t.Rules {
		rule := fmt.Sprintf("rule %d", i)
		if r.ID != "" {
			rule += " (" + ShortRuleID(r.ID) + ")"
		}
		if len(r.Serve) == 0 {
			return fmt.Errorf("%s serves no variant", rule)
		}
		total := 0.0
		for _, s := range r.Serve {
			if variantIndex(f.Variants, s.VariantKey) < 0 {
				return fmt.Errorf("%s serves %q, which is not a variant of feature %s (variants: %s)", rule, s.VariantKey, f.Key, variantKeys(f.Variants))
			}
			total += s.Percentage
		}
		if math.Abs(total-100) > 1e-9 {
			return fmt.Errorf("%s serves %s%% in total; a rule's variants must total 100%%", rule, formatPercent(total))
		}
		for _, c := range r.Conditions {
			vt, err := types.typeOf(projectKey, c.ContextKey)
			if err != nil {
				return fmt.Errorf("%s: %w", rule, err)
			}
			if c.ValueType != vt {
				return fmt.Errorf("%s: context key %s is a %s attribute, but the condition's valueType is %s", rule, c.ContextKey, vt, c.ValueType)
			}
			if syntax, known := operatorSyntax[c.Operator]; known && !operatorSupported(vt, syntax) {
				return fmt.Errorf("%s: operator %s does not apply to %s context key %s", rule, c.Operator, vt, c.ContextKey)
			}
		}
		if r.BucketingKey != "" {
			if _, err := types.typeOf(projectKey, r.BucketingKey); err != nil {
				return fmt.Errorf("%s: bucketing key: %w", rule, err)
			}
		}
		if r.Rollout != nil {
			for j, s := range r.Rollout.Steps {
				if s.Percentage < 0 || s.Percentage > 100 {
					return fmt.Errorf("%s: rollout step %d is %s%%; a step is a percentage from 0 to 100", rule, j, formatPercent(s.Percentage))
				}
				if j > 0 && s.EffectiveTime.Before(r.Rollout.Steps[j-1].EffectiveTime) {
					return fmt.Errorf("%s: rollout step %d takes effect before the step ahead of it; list steps in time order", rule, j)
				}
			}
		}
	}
	return nil
}

// checkRuleIDs refuses, unless force, a file that drops an existing rule's
// id (the rule is removed, or kept without its id and so re-created, which
// re-buckets its users) or names an id the stage does not have.
func checkRuleIDs(live, file []TargetingRule, force bool) error {
	var liveIDs, fileIDs []string
	for _, r := range live {
		if r.ID != "" {
			liveIDs = append(liveIDs, r.ID)
		}
	}
	for _, r := range file {
		if r.ID != "" {
			if !slices.Contains(liveIDs, r.ID) {
				return fmt.Errorf("the file names rule id %s, which the stage does not have; rule ids are assigned by the service, so drop the id to add the rule as a new one", r.ID)
			}
			fileIDs = append(fileIDs, r.ID)
		}
	}
	var dropped []string
	for _, id := range liveIDs {
		if !slices.Contains(fileIDs, id) {
			dropped = append(dropped, ShortRuleID(id))
		}
	}
	if len(dropped) > 0 && !force {
		return fmt.Errorf("the file drops rule(s) %s: a removed rule, or one kept without its id, re-buckets its users; pass --force to apply it anyway", strings.Join(dropped, ", "))
	}
	return nil
}
