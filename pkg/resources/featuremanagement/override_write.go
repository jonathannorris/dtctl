package featuremanagement

import (
	"context"
	"fmt"
	"slices"
	"strings"

	sdkfm "github.com/Dynatrace/dtctl/sdk/api/featuremanagement"
)

// OverrideWriteResult is the caller's overrides of a feature after a write.
type OverrideWriteResult struct {
	Overrides *FeatureOverrides
	// Changed is false when the overrides already were what the command
	// asked for, so nothing was sent.
	Changed bool
}

// SetOverride sets the variant the caller is served for a feature in one
// release stage, or with a nil variant removes the override there. The
// feature is a key or name; the stage and the variant are keys.
func (h *Handler) SetOverride(projectKey, featureKeyOrName, stage string, variant *string) (*OverrideWriteResult, error) {
	d, err := h.readFeature(projectKey, featureKeyOrName)
	if err != nil {
		return nil, err
	}
	if variant != nil && variantIndex(d.Variants, *variant) < 0 {
		return nil, fmt.Errorf("feature %s has no variant %q (variants: %s)", d.Key, *variant, variantKeys(d.Variants))
	}
	current, err := h.GetFeatureOverrides(projectKey, d.Key)
	if err != nil {
		return nil, err
	}
	found := false
	var stages []string
	for _, o := range current.Overrides {
		stages = append(stages, o.ReleaseStageKey)
		found = found || o.ReleaseStageKey == stage
	}
	if !found {
		return nil, fmt.Errorf("project %s has no release stage %q (stages: %s)", projectKey, stage, strings.Join(stages, ", "))
	}
	return h.writeOverrides(projectKey, current, func(o MyOverride) *string {
		if o.ReleaseStageKey == stage {
			return variant
		}
		return o.VariantKey
	})
}

// ClearOverrides removes the caller's overrides of a feature in every
// release stage.
func (h *Handler) ClearOverrides(projectKey, featureKeyOrName string) (*OverrideWriteResult, error) {
	current, err := h.GetFeatureOverrides(projectKey, featureKeyOrName)
	if err != nil {
		return nil, err
	}
	return h.writeOverrides(projectKey, current, func(MyOverride) *string { return nil })
}

// writeOverrides sends the overrides variantOf gives each stage, unless they
// are the current ones.
func (h *Handler) writeOverrides(projectKey string, current *FeatureOverrides, variantOf func(MyOverride) *string) (*OverrideWriteResult, error) {
	var body sdkfm.MyOverridesUpdate
	changed := false
	for _, o := range current.Overrides {
		v := variantOf(o)
		if (v == nil) != (o.VariantKey == nil) || (v != nil && *v != *o.VariantKey) {
			changed = true
		}
		if v != nil {
			body.Overrides = append(body.Overrides, sdkfm.MyOverrideUpdate{ReleaseStageKey: o.ReleaseStageKey, VariantKey: v})
		}
	}
	if !changed {
		return &OverrideWriteResult{Overrides: current}, nil
	}
	updated, err := h.sdk.UpdateMyOverrides(context.Background(), projectKey, current.FeatureKey, body)
	if err != nil {
		return nil, err
	}
	overrides := updated.Overrides
	if overrides == nil {
		overrides = []MyOverride{}
	}
	return &OverrideWriteResult{Overrides: &FeatureOverrides{FeatureKey: current.FeatureKey, Overrides: overrides}, Changed: true}, nil
}

// TargetingKeysChange changes the caller's targeting keys: Set replaces
// them, then Add and Remove apply.
type TargetingKeysChange struct {
	Set         *[]string
	Add, Remove []string
}

// UpdateTargetingKeys changes the caller's targeting keys in a project (the
// overrides profile) and returns them as they are afterwards. Keys are
// case-sensitive, as SDKs pass them.
func (h *Handler) UpdateTargetingKeys(projectKey string, c TargetingKeysChange) ([]string, bool, error) {
	profile, err := h.sdk.GetOverridesProfile(context.Background(), projectKey)
	if err != nil {
		return nil, false, err
	}
	before := append([]string{}, profile.TargetingKeys...)
	keys := append([]string{}, before...)
	if c.Set != nil {
		keys = nil
		for _, k := range *c.Set {
			if !slices.Contains(keys, k) {
				keys = append(keys, k)
			}
		}
	}
	for _, k := range c.Add {
		if !slices.Contains(keys, k) {
			keys = append(keys, k)
		}
	}
	for _, k := range c.Remove {
		keys = slices.DeleteFunc(keys, func(s string) bool { return s == k })
	}
	for _, k := range keys {
		if strings.TrimSpace(k) == "" {
			return nil, false, fmt.Errorf("a targeting key must not be empty")
		}
	}
	if keys == nil {
		keys = []string{}
	}
	if slices.Equal(keys, before) {
		return keys, false, nil
	}
	if err := h.sdk.UpdateOverridesProfile(context.Background(), projectKey, sdkfm.OverridesProfile{TargetingKeys: keys}); err != nil {
		return nil, false, err
	}
	after, err := h.sdk.GetOverridesProfile(context.Background(), projectKey)
	if err != nil {
		return nil, false, err
	}
	if after.TargetingKeys == nil {
		after.TargetingKeys = []string{}
	}
	return after.TargetingKeys, true, nil
}

// DeleteAllOverrides deletes every override the caller has in a project.
func (h *Handler) DeleteAllOverrides(projectKey string) error {
	return h.sdk.DeleteProjectMyOverrides(context.Background(), projectKey)
}

// DeleteTargetingKeys deletes the caller's targeting keys in a project.
func (h *Handler) DeleteTargetingKeys(projectKey string) error {
	return h.sdk.DeleteOverridesProfile(context.Background(), projectKey)
}

// TargetingKeyRow is one table row of the caller's targeting keys.
type TargetingKeyRow struct {
	TargetingKey string `table:"TARGETING_KEY"`
}

// TargetingKeyRows turns targeting keys into table rows.
func TargetingKeyRows(keys []string) []TargetingKeyRow {
	rows := make([]TargetingKeyRow, len(keys))
	for i, k := range keys {
		rows[i] = TargetingKeyRow{TargetingKey: k}
	}
	return rows
}
